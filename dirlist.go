// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"html/template"
	"io"
	"io/fs"
	"log"
	"path"
	"path/filepath"
	"strings"

	"github.com/some-programs/present/pkg/present"
)

// parseDirList parses the template of directory listings in fsys, which
// links to static resources through staticURL.
func parseDirList(fsys fs.FS, staticURL func(path string) string) (*template.Template, error) {
	return template.New("dir.tmpl").
		Funcs(template.FuncMap{"static": staticURL}).
		ParseFS(fsys, "templates/dir.tmpl")
}

// dirList writes a listing of the directory name in fsys, served at the
// URL path urlPath, to w. It parses the first part of each presentation in
// it to show its title in the listing.
func (s *server) dirList(w io.Writer, fsys fs.FS, name, urlPath string) error {
	// The entries are sorted by name, and so is each list built from them.
	entries, err := fs.ReadDir(fsys, name)
	if err != nil {
		return err
	}
	strippedPath := strings.Trim(path.Clean("/"+urlPath), "/")
	d := &dirListData{Path: strippedPath, Doc: s.doc}
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
	return s.dirListTemplate.Execute(w, d)
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

type dirListData struct {
	Path                          string
	Doc                           bool // list the documentation and the examples, with -doc
	Dirs, Slides, Articles, Other []dirEntry
}

type dirEntry struct {
	Name, Path, Title string
}
