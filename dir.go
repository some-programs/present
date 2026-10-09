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
	"os"
	"path/filepath"
	"strings"

	"github.com/some-programs/present/pkg/present"
)

func init() {
	http.HandleFunc("/", dirHandler)
}

// dirHandler serves a directory listing for the requested path, rooted at *contentPath.
func dirHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/favicon.ico" {
		http.NotFound(w, r)
		return
	}
	name := filepath.Join(*contentPath, r.URL.Path)
	if isDoc(name) {
		var buf bytes.Buffer
		if err := renderDoc(&buf, name); err != nil {
			serveError(w, r, name, err)
			return
		}
		w.Write(buf.Bytes())
		return
	}
	if isDir, err := dirList(w, name); err != nil {
		serveError(w, r, name, err)
		return
	} else if isDir {
		return
	}
	// Have browsers check for a newer file on every load, such as an
	// image that has been replaced while the slides are open.
	w.Header().Set("Cache-Control", "no-cache")
	http.FileServer(http.Dir(*contentPath)).ServeHTTP(w, r)
}

// serveError logs err and replies with an error for the request for name.
// If name itself does not exist, the reply is a plain 404. Otherwise the
// error text is sent so that authors see mistakes such as an unknown
// command, with the content directory removed from any paths in it.
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
	root := filepath.Clean(*contentPath) + string(filepath.Separator)
	http.Error(w, strings.ReplaceAll(err.Error(), root, ""), http.StatusInternalServerError)
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

func initTemplates(fsys fs.FS) error {
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
			"playable": playable,
			"thankYou": func() bool { return *thankYou },
			"hot":      func() bool { return *hot },
		})
		if _, err := tmpl.ParseFS(fsys, actionTmpl, contentTmpl); err != nil {
			return err
		}
		contentTemplate[ext] = tmpl
	}

	var err error
	dirListTemplate, err = template.ParseFS(fsys, "templates/dir.tmpl")
	return err
}

// renderDoc reads the present file, gets its template representation,
// and executes the template, sending output to w.
func renderDoc(w io.Writer, docFile string) error {
	// Read the input and build the doc structure.
	doc, err := parse(docFile, 0)
	if err != nil {
		return err
	}
	renderDiagrams(doc, docFile)

	// Find which template should be executed.
	tmpl := contentTemplate[filepath.Ext(docFile)]

	// Execute the template.
	return doc.Render(w, tmpl)
}

func parse(name string, mode present.ParseMode) (*present.Doc, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return present.Parse(f, name, mode)
}

// dirList scans the given path and writes a directory listing to w.
// It parses the first part of each .slide file it encounters to display the
// presentation title in the listing.
// If the given path is not a directory, it returns (isDir == false, err == nil)
// and writes nothing to w.
func dirList(w io.Writer, name string) (isDir bool, err error) {
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
	strippedPath := strings.TrimPrefix(name, filepath.Clean(*contentPath))
	strippedPath = strings.TrimPrefix(strippedPath, "/")
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
	if d.Path == "." {
		d.Path = ""
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
