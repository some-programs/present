package main

import (
	"bytes"
	_ "embed"
	"flag"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
)

var showDoc = flag.Bool("doc", false, "serve the present documentation (README.md) at /_doc and link to it from the Talks page")

//go:embed README.md
var readme []byte

// repoURL is the base that relative links in README.md, such as to the
// examples directory, are resolved against, since the files they point
// to are not served by present.
const repoURL = "https://github.com/some-programs/present/blob/main/"

// initDoc registers the handler that serves README.md as HTML, if -doc is
// set.
func initDoc(fsys fs.FS, staticURL func(path string) string) error {
	if !*showDoc {
		return nil
	}
	body, err := renderReadme(readme)
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
	return nil
}

// renderReadme renders the Markdown of README.md as HTML, with relative
// links resolved against repoURL.
func renderReadme(src []byte) (template.HTML, error) {
	doc := parser.New(parser.WithExtensions(extension.TableParser)).Parse(src)
	base, err := url.Parse(repoURL)
	if err != nil {
		return "", err
	}
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if l, ok := n.(*ast.Link); ok && entering {
			if u, err := url.Parse(l.Destination.Value(src)); err == nil && !u.IsAbs() && u.Path != "" {
				l.Destination = text.NewSingleLineValueFromString(base.ResolveReference(u).String(), text.IdentityDecoder)
			}
		}
		return ast.WalkContinue, nil
	})
	var b bytes.Buffer
	r := html.New(html.WithExtensions(extension.NewTableHTMLRenderer()))
	if err := r.Render(&b, src, doc); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}
