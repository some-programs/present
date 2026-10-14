// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"html/template"
	"io"
	"io/fs"
	"path"
	"slices"

	"github.com/some-programs/present/pkg/present"
)

// pageSettings are the settings of the pages of presentations and articles,
// which serve takes from its flags, and export sets.
type pageSettings struct {
	Play        bool // code of .play can be run
	Notes       bool // the presenter notes window can be opened
	Preload     bool // media are preloaded
	InlineFonts bool // fonts.css holds the fonts, so they are not preloaded
	ThankYou    bool // end slides with a "Thank you" slide
	Hot         bool // reload the page when its files change
}

// A renderer renders the pages of presentations and articles, and the
// presenter notes window, with one set of page settings. Serve and export
// each have their own.
type renderer struct {
	settings  pageSettings
	templates map[string]*template.Template // by extension: .slide, .article
}

// newRenderer parses the templates in fsys for pages with the settings s.
// Templates link to static resources through staticURL, which maps a path
// such as "/static/styles.css" to the URL to serve it at.
func newRenderer(fsys fs.FS, staticURL func(path string) string, s pageSettings) (*renderer, error) {
	r := &renderer{settings: s, templates: make(map[string]*template.Template)}
	for ext, name := range map[string]string{
		".slide":   "slides.tmpl",
		".article": "article.tmpl",
	} {
		tmpl := present.Template().Funcs(template.FuncMap{
			"playable":     func(c present.Code) bool { return s.Play && c.Play },
			"sectionNotes": sectionNotes,
			"static":       staticURL,
		})
		if _, err := tmpl.ParseFS(fsys, "templates/action.tmpl", "templates/"+name); err != nil {
			return nil, err
		}
		r.templates[ext] = tmpl
	}
	return r, nil
}

// page writes the page of doc, read from docFile, a slide presentation or
// an article by its extension. With sources, the slides have a source
// panel, which names the presentation file sourceFile.
func (r *renderer) page(w io.Writer, doc *present.Doc, docFile, sourceFile string, sources []slideSource) error {
	tmpl := r.templates[path.Ext(docFile)]
	return tmpl.ExecuteTemplate(w, "root", pageData{
		Doc:          doc,
		Template:     tmpl,
		pageSettings: r.settings,
		SourceFile:   sourceFile,
		Sources:      sources,
	})
}

// notes writes the presenter notes window of the slide presentation doc,
// with the notes of every slide.
func (r *renderer) notes(w io.Writer, doc *present.Doc) error {
	return r.templates[".slide"].ExecuteTemplate(w, "notes", doc)
}

// pageData is what the templates of presentations and articles are
// executed with: what present.Doc.Render passes them, the settings of the
// page, and with -doc, or export -source, the source of each slide.
type pageData struct {
	*present.Doc
	Template *template.Template
	pageSettings

	SourceFile string        // the presentation file, as sourceFileName names it
	Sources    []slideSource // the source of each slide, with -doc or export -source
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
