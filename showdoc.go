package main

import (
	"bytes"
	_ "embed"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
)

var showDoc = serveFlags.Bool("doc", false, "serve the present documentation (README.md) at /_doc, the example presentations at /_examples and the fonts at /download/fonts.tar, and list them on the Talks page")

//go:embed README.md
var readme []byte

// repoURL is the base that relative links in README.md are resolved
// against, since the files they point to are not served by present,
// except for the examples.
const repoURL = "https://github.com/some-programs/present/blob/main/"

// rawURL is the base that relative image sources in README.md are resolved
// against, since a page on repoURL is HTML, not the image itself.
const rawURL = "https://raw.githubusercontent.com/some-programs/present/main/"

// initDoc registers the handlers that serve README.md as HTML and the
// fonts archive, and writes out the examples for dirHandler to serve, if
// -doc is set.
func initDoc(fsys fs.FS, staticURL func(path string) string) error {
	if !*showDoc {
		return nil
	}
	if err := initExamples(); err != nil {
		return err
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
	http.Handle("/download/fonts.tar", fontsTarHandler(fsys))
	return nil
}

// renderReadme renders the Markdown of README.md as HTML, with relative
// links to the examples pointing at examplesPath, other relative links
// resolved against repoURL, and relative image sources against rawURL.
func renderReadme(src []byte) (template.HTML, error) {
	doc := parser.New(parser.WithExtensions(extension.TableParser)).Parse(src)
	base, err := url.Parse(repoURL)
	if err != nil {
		return "", err
	}
	raw, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	// relative parses dest and reports whether it is a relative path.
	relative := func(dest text.SingleLineValue) (*url.URL, bool) {
		u, err := url.Parse(dest.Value(src))
		if err != nil || u.IsAbs() || u.Path == "" || strings.HasPrefix(u.Path, "/") {
			return nil, false
		}
		return u, true
	}
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Link:
			if u, ok := relative(n.Destination); ok {
				dest := base.ResolveReference(u).String()
				if u.Path == "examples" || strings.HasPrefix(u.Path, "examples/") {
					dest = examplesPath + strings.TrimPrefix(u.Path, "examples")
				}
				n.Destination = text.NewSingleLineValueFromString(dest, text.IdentityDecoder)
			}
		case *ast.Image:
			if u, ok := relative(n.Destination); ok {
				dest := raw.ResolveReference(u).String()
				n.Destination = text.NewSingleLineValueFromString(dest, text.IdentityDecoder)
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
