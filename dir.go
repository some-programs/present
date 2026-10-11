// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
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
// *contentPath, or at examplesDir for the examples with -doc.
func dirHandler(w http.ResponseWriter, r *http.Request) {
	name := contentFile(r.URL.Path)
	if isDoc(name) {
		render := func(w io.Writer) error { return renderDoc(w, name, r.URL.Path) }
		if r.URL.Query().Has("notes") {
			if !present.NotesEnabled || filepath.Ext(name) != ".slide" {
				http.NotFound(w, r)
				return
			}
			render = func(w io.Writer) error { return renderNotes(w, name) }
		}
		var buf bytes.Buffer
		if err := render(&buf); err != nil {
			serveError(w, r, name, err)
			return
		}
		w.Write(buf.Bytes())
		return
	}
	if isDir, err := dirList(w, name, r.URL.Path); err != nil {
		serveError(w, r, name, err)
		return
	} else if isDir {
		return
	}
	// Have browsers check for a newer file on every load, such as an
	// image that has been replaced while the slides are open.
	w.Header().Set("Cache-Control", "no-cache")
	if filepath.Ext(name) == ".svg" && serveSVG(w, r, name) {
		return
	}
	prefix, dir := contentRoot(r.URL.Path)
	http.StripPrefix(prefix, http.FileServer(http.Dir(dir))).ServeHTTP(w, r)
}

// serveError logs err and replies with an error for the request for name.
// If name itself does not exist, the reply is a plain 404. Otherwise the
// error text is sent so that authors see mistakes such as an unknown
// command, with the content directories removed from any paths in it.
func serveError(w http.ResponseWriter, r *http.Request, name string, err error) {
	addr, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		addr = r.RemoteAddr
	}
	log.Printf("request from %s: %s", addr, err)

	var pathErr *fs.PathError
	if errors.As(err, &pathErr) && pathErr.Path == name && errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	http.Error(w, trimContentDirs(err.Error()), http.StatusInternalServerError)
}

func isDoc(path string) bool {
	_, ok := contentTemplate[filepath.Ext(path)]
	return ok
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
			"importMap": func(paths ...string) (template.HTML, error) {
				return importMap(paths, staticURL)
			},
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
// executed with: what present.Doc.Render passes them, and with -doc the
// source of each slide.
type pageData struct {
	*present.Doc
	Template       *template.Template
	PlayEnabled    bool
	NotesEnabled   bool
	PreloadEnabled bool

	SourceFile string        // the presentation file, as sourceFileName names it
	Sources    []slideSource // the source of each slide, with -doc
}

// renderDoc reads the present file docFile, served at the URL path
// urlPath, gets its template representation, and executes the template,
// sending output to w.
func renderDoc(w io.Writer, docFile, urlPath string) error {
	// Read the input and build the doc structure.
	src, err := os.ReadFile(docFile)
	if err != nil {
		return err
	}
	doc, err := present.Parse(bytes.NewReader(src), docFile, 0)
	if err != nil {
		return err
	}
	renderDiagrams(doc, docFile)

	// Find which template should be executed.
	tmpl := contentTemplate[filepath.Ext(docFile)]

	data := pageData{
		Doc:            doc,
		Template:       tmpl,
		PlayEnabled:    present.PlayEnabled,
		NotesEnabled:   present.NotesEnabled,
		PreloadEnabled: present.PreloadEnabled,
	}
	if *showDoc && filepath.Ext(docFile) == ".slide" {
		data.SourceFile = sourceFileName(urlPath)
		data.Sources, err = slideSources(doc, src, docFile, urlPath)
		if err != nil {
			return err
		}
	}

	// Execute the template.
	return tmpl.ExecuteTemplate(w, "root", data)
}

// renderNotes renders the presenter notes window of the slide
// presentation docFile, with the notes of every slide. The presentation's
// own window shows itself in it.
func renderNotes(w io.Writer, docFile string) error {
	doc, err := parse(docFile, 0)
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

// importMap returns an import map script element that has browsers load
// the JavaScript modules at paths from staticURL, also when one module
// imports another.
func importMap(paths []string, staticURL func(string) string) (template.HTML, error) {
	imports := make(map[string]string)
	for _, p := range paths {
		imports[p] = staticURL(p)
	}
	b, err := json.Marshal(map[string]any{"imports": imports})
	if err != nil {
		return "", err
	}
	return template.HTML(`<script type="importmap">` + string(b) + `</script>`), nil
}

func parse(name string, mode present.ParseMode) (*present.Doc, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return present.Parse(f, name, mode)
}

// dirList scans the given path, served at the URL path urlPath, and writes
// a directory listing to w.
// It parses the first part of each .slide file it encounters to display the
// presentation title in the listing.
// If the given path is not a directory, it returns (isDir == false, err == nil)
// and writes nothing to w.
func dirList(w io.Writer, name, urlPath string) (isDir bool, err error) {
	fi, err := os.Stat(name)
	if err != nil {
		return false, err
	}
	if !fi.IsDir() {
		return false, nil
	}
	// The entries are sorted by name, and so is each list built from them.
	entries, err := os.ReadDir(name)
	if err != nil {
		return false, err
	}
	strippedPath := strings.Trim(path.Clean("/"+urlPath), "/")
	d := &dirListData{Path: strippedPath}
	for _, de := range entries {
		e := dirEntry{
			Name: de.Name(),
			Path: filepath.ToSlash(filepath.Join(strippedPath, de.Name())),
		}
		if de.IsDir() && showDir(e.Name) {
			d.Dirs = append(d.Dirs, e)
			continue
		}
		if isDoc(e.Name) {
			fn := filepath.ToSlash(filepath.Join(name, de.Name()))
			if p, err := parse(fn, present.TitlesOnly); err != nil {
				log.Printf("parse(%q, present.TitlesOnly): %v", fn, err)
			} else {
				e.Title = p.Title
			}
			switch filepath.Ext(e.Path) {
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
