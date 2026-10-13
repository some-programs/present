package main

import (
	"bytes"
	_ "embed"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/some-programs/present/internal/readme"
)

var showDoc = serveFlags.Bool("doc", false, "serve the present documentation (README.md) at /_doc, the example presentations at /_examples and the fonts at /download/fonts.tar, and list them on the Talks page")

//go:embed README.md
var readmeSrc []byte

// initDoc registers the handlers that serve README.md as HTML and the
// fonts archive, if -doc is set. dirHandler serves the examples.
func initDoc(fsys fs.FS, staticURL func(path string) string) error {
	if !*showDoc {
		return nil
	}
	body, err := readme.Render(readmeSrc, examplesPath)
	if err != nil {
		return err
	}
	tmpl, err := template.New("doc.tmpl").
		Funcs(template.FuncMap{"static": staticURL}).
		ParseFS(fsys, "templates/doc.tmpl")
	if err != nil {
		return err
	}
	var page bytes.Buffer
	if err := tmpl.Execute(&page, body); err != nil {
		return err
	}
	http.HandleFunc("/_doc", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page.Bytes())
	})
	http.Handle("/download/fonts.tar", fontsTarHandler(fsys))
	return nil
}
