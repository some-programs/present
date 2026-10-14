// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/some-programs/present/pkg/present"
)

func init() {
	http.HandleFunc("/", dirHandler)
}

// dirHandler serves a directory listing for the requested path, rooted at
// *contentPath, or at the examples with -doc.
func dirHandler(w http.ResponseWriter, r *http.Request) {
	root, name := contentFile(r.URL.Path)
	if isDoc(name) {
		render := func(w io.Writer) error { return renderDoc(w, root, name, r.URL.Path) }
		if r.URL.Query().Has("notes") {
			if !present.NotesEnabled || path.Ext(name) != ".slide" {
				http.NotFound(w, r)
				return
			}
			render = func(w io.Writer) error { return renderNotes(w, root.fsys, name) }
		}
		var buf bytes.Buffer
		if err := render(&buf); err != nil {
			serveError(w, r, name, err)
			return
		}
		w.Write(buf.Bytes())
		return
	}
	if isDir, err := dirList(w, root.fsys, name, r.URL.Path); err != nil {
		serveError(w, r, name, err)
		return
	} else if isDir {
		return
	}
	// Have browsers check for a newer file on every load, such as an
	// image that has been replaced while the slides are open.
	w.Header().Set("Cache-Control", "no-cache")
	if path.Ext(name) == ".svg" && serveSVG(w, r, root.fsys, name) {
		return
	}
	http.StripPrefix(root.prefix, http.FileServerFS(root.fsys)).ServeHTTP(w, r)
}

// serveError logs err and replies with an error for the request for name.
// If name itself does not exist, the reply is a plain 404. Otherwise the
// error text is sent so that authors see mistakes such as an unknown
// command.
func serveError(w http.ResponseWriter, r *http.Request, name string, err error) {
	addr, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		addr = r.RemoteAddr
	}
	log.Printf("request from %s: %s", addr, err)

	if pathErr, ok := errors.AsType[*fs.PathError](err); ok && pathErr.Path == name && errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// isDoc reports whether the file name is a slide presentation or an
// article.
func isDoc(name string) bool {
	switch filepath.Ext(name) {
	case ".slide", ".article":
		return true
	}
	return false
}

var (
	// dirListTemplate holds the front page template.
	dirListTemplate *template.Template

	// contentTemplate maps the presentable file extensions to the
	// template to be executed.
	contentTemplate map[string]*template.Template
)

// initTemplates parses the templates in fsys. Templates link to static
// resources through staticURL, which maps a path such as
// "/static/styles.css" to the URL to serve it at.
func initTemplates(fsys fs.FS, staticURL func(path string) string) error {
	// Locate the template file.
	actionTmpl := "templates/action.tmpl"

	contentTemplate = make(map[string]*template.Template)

	for ext, contentTmpl := range map[string]string{
		".slide":   "slides.tmpl",
		".article": "article.tmpl",
	} {
		contentTmpl = "templates/" + contentTmpl

		// Read and parse the input.
		tmpl := present.Template()
		tmpl = tmpl.Funcs(template.FuncMap{
			"playable":     playable,
			"sectionNotes": sectionNotes,
			"thankYou":     func() bool { return *thankYou },
			"hot":          func() bool { return *hot },
			"static":       staticURL,
		})
		if _, err := tmpl.ParseFS(fsys, actionTmpl, contentTmpl); err != nil {
			return err
		}
		contentTemplate[ext] = tmpl
	}

	var err error
	dirListTemplate, err = template.New("dir.tmpl").
		Funcs(template.FuncMap{
			"static": staticURL,
			"doc":    func() bool { return *showDoc },
		}).
		ParseFS(fsys, "templates/dir.tmpl")
	return err
}

// pageData is what the templates of presentations and articles are
// executed with: what present.Doc.Render passes them, and with -doc, or
// export -source, the source of each slide.
type pageData struct {
	*present.Doc
	Template       *template.Template
	PlayEnabled    bool
	NotesEnabled   bool
	PreloadEnabled bool
	InlineFonts    bool // fonts.css holds the fonts, so they are not preloaded

	SourceFile string        // the presentation file, as sourceFileName names it
	Sources    []slideSource // the source of each slide, with -doc or export -source
}

// renderDoc reads the present file docFile in root, served at the URL path
// urlPath, gets its template representation, and executes the template,
// sending output to w.
func renderDoc(w io.Writer, root contentRoot, docFile, urlPath string) error {
	// Read the input and build the doc structure.
	src, err := fs.ReadFile(root.fsys, docFile)
	if err != nil {
		return err
	}
	doc, err := parse(root.fsys, bytes.NewReader(src), docFile, 0)
	if err != nil {
		return err
	}
	renderDiagrams(doc, root, docFile)

	data := newPageData(doc, docFile)
	if *showDoc && path.Ext(docFile) == ".slide" {
		data.SourceFile = sourceFileName(urlPath)
		data.Sources, err = slideSources(doc, src, root.fsys, docFile, urlPath)
		if err != nil {
			return err
		}
	}

	// Execute the template.
	return data.Template.ExecuteTemplate(w, "root", data)
}

// newPageData returns the data to execute the template for doc, read from
// docFile, with.
func newPageData(doc *present.Doc, docFile string) pageData {
	return pageData{
		Doc:            doc,
		Template:       contentTemplate[path.Ext(docFile)],
		PlayEnabled:    present.PlayEnabled,
		NotesEnabled:   present.NotesEnabled,
		PreloadEnabled: present.PreloadEnabled,
	}
}

// renderNotes renders the presenter notes window of the slide
// presentation docFile in fsys, with the notes of every slide. The
// presentation's own window shows itself in it.
func renderNotes(w io.Writer, fsys fs.FS, docFile string) error {
	doc, err := parseFile(fsys, docFile, 0)
	if err != nil {
		return err
	}
	return contentTemplate[".slide"].ExecuteTemplate(w, "notes", doc)
}

// sectionNotes returns the notes of the slide s, including those of its
// subsections.
func sectionNotes(s present.Section) []string {
	notes := slices.Clone(s.Notes)
	for _, e := range s.Elem {
		if sub, ok := e.(present.Section); ok {
			notes = append(notes, sectionNotes(sub)...)
		}
	}
	return notes
}

// dirList scans the given path in fsys, served at the URL path urlPath,
// and writes a directory listing to w.
// It parses the first part of each .slide file it encounters to display the
// presentation title in the listing.
// If the given path is not a directory, it returns (isDir == false, err == nil)
// and writes nothing to w.
func dirList(w io.Writer, fsys fs.FS, name, urlPath string) (isDir bool, err error) {
	fi, err := fs.Stat(fsys, name)
	if err != nil {
		return false, err
	}
	if !fi.IsDir() {
		return false, nil
	}
	// The entries are sorted by name, and so is each list built from them.
	entries, err := fs.ReadDir(fsys, name)
	if err != nil {
		return false, err
	}
	strippedPath := strings.Trim(path.Clean("/"+urlPath), "/")
	d := &dirListData{Path: strippedPath}
	for _, de := range entries {
		e := dirEntry{
			Name: de.Name(),
			Path: path.Join(strippedPath, de.Name()),
		}
		if de.IsDir() && showDir(e.Name) {
			d.Dirs = append(d.Dirs, e)
			continue
		}
		if isDoc(e.Name) {
			fn := path.Join(name, de.Name())
			if p, err := parseFile(fsys, fn, present.TitlesOnly); err != nil {
				log.Printf("parseFile(%q, present.TitlesOnly): %v", fn, err)
			} else {
				e.Title = p.Title
			}
			switch path.Ext(e.Path) {
			case ".article":
				d.Articles = append(d.Articles, e)
			case ".slide":
				d.Slides = append(d.Slides, e)
			}
		} else if showFile(e.Name) {
			d.Other = append(d.Other, e)
		}
	}
	return true, dirListTemplate.Execute(w, d)
}

// showFile reports whether the given file should be displayed in the list.
func showFile(n string) bool {
	switch filepath.Ext(n) {
	case ".pdf":
	case ".html":
	case ".go":
	default:
		return isDoc(n)
	}
	return true
}

// showDir reports whether the given directory should be displayed in the list.
func showDir(n string) bool {
	return !strings.HasPrefix(n, ".") && !strings.HasPrefix(n, "_")
}

type dirListData struct {
	Path                          string
	Dirs, Slides, Articles, Other []dirEntry
}

type dirEntry struct {
	Name, Path, Title string
}
