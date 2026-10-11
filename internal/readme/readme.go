// Package readme renders README.md as HTML, for present serve -doc and for
// the GitHub Pages site that internal/readmegen writes.
package readme

import (
	"bytes"
	"html/template"
	"net/url"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
)

// RepoURL is the base that relative links in README.md are resolved
// against, since the files they point to are not served with it, except
// for the examples.
const RepoURL = "https://github.com/some-programs/present/blob/main/"

// RawURL is the base that relative image sources in README.md are resolved
// against, since a page on RepoURL is HTML, not the image itself.
const RawURL = "https://raw.githubusercontent.com/some-programs/present/main/"

// Render renders the Markdown src of README.md as HTML, with relative links
// to the examples directory pointing at examplesURL instead, other relative
// links resolved against RepoURL, and relative image sources against
// RawURL.
func Render(src []byte, examplesURL string) (template.HTML, error) {
	doc := parser.New(parser.WithExtensions(extension.TableParser)).Parse(src)
	base, err := url.Parse(RepoURL)
	if err != nil {
		return "", err
	}
	raw, err := url.Parse(RawURL)
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
					dest = examplesURL + strings.TrimPrefix(u.Path, "examples")
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
