package main

import (
	"bytes"
	_ "embed"
	"html/template"
	"io/fs"

	"github.com/some-programs/present/internal/readme"
)

//go:embed README.md
var readmeSrc []byte

// readmePage returns README.md as an HTML page, which links to static
// files through staticURL and to the examples at examplesPath.
func readmePage(fsys fs.FS, staticURL func(path string) string) ([]byte, error) {
	body, err := readme.Render(readmeSrc, examplesPath)
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New("doc.tmpl").
		Funcs(template.FuncMap{"static": staticURL}).
		ParseFS(fsys, "templates/doc.tmpl")
	if err != nil {
		return nil, err
	}
	var page bytes.Buffer
	if err := tmpl.Execute(&page, body); err != nil {
		return nil, err
	}
	return page.Bytes(), nil
}
