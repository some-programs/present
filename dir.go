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
		render := func(w io.Writer) error { return renderDoc(w, serveRenderer, root, name, r.URL.Path) }
		if r.URL.Query().Has("notes") {
			if !*notesFlag || path.Ext(name) != ".slide" {
				http.NotFound(w, r)
				return
			}
			render = func(w io.Writer) error { return renderNotes(w, serveRenderer, root.fsys, name) }
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

var (
	// dirListTemplate holds the front page template, for serve.
	dirListTemplate *template.Template

	// serveRenderer renders the pages that serve serves.
	serveRenderer *renderer
)

// renderDoc reads the present file docFile in root, served at the URL path
// urlPath, and writes its page, rendered by pr, to w.
func renderDoc(w io.Writer, pr *renderer, root contentRoot, docFile, urlPath string) error {
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

	var sourceFile string
	var sources []slideSource
	if *showDoc && path.Ext(docFile) == ".slide" {
		sourceFile = sourceFileName(urlPath)
		if sources, err = slideSources(doc, src, root.fsys, docFile, urlPath); err != nil {
			return err
		}
	}
	return pr.page(w, doc, docFile, sourceFile, sources)
}

// renderNotes writes the presenter notes window of the slide presentation
// docFile in fsys, rendered by pr, to w. The presentation's own window
// shows itself in it.
func renderNotes(w io.Writer, pr *renderer, fsys fs.FS, docFile string) error {
	doc, err := parseFile(fsys, docFile, 0)
	if err != nil {
		return err
	}
	return pr.notes(w, doc)
}

// parseDirList parses the template of directory listings in fsys, which
// links to static resources through staticURL.
func parseDirList(fsys fs.FS, staticURL func(path string) string) (*template.Template, error) {
	return template.New("dir.tmpl").
		Funcs(template.FuncMap{
			"static": staticURL,
			"doc":    func() bool { return *showDoc },
		}).
		ParseFS(fsys, "templates/dir.tmpl")
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
