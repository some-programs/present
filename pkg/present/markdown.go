// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"html/template"
	"slices"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
)

func renderMarkdown(input []byte) (template.HTML, error) {
	doc := parser.New(parser.WithExtensions(extension.TableParser)).Parse(input)
	fixupMarkdown(doc)
	var b strings.Builder
	r := html.New(html.WithUnsafe(), html.WithExtensions(extension.NewTableHTMLRenderer()))
	if err := r.Render(&b, input, doc); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}

// renderInlineMarkdown renders s, a single line such as a caption or a link
// label, as Markdown without the enclosing paragraph.
func renderInlineMarkdown(s string) (template.HTML, error) {
	html, err := renderMarkdown([]byte(s))
	if err != nil {
		return "", err
	}
	inner, ok := strings.CutPrefix(string(html), "<p>")
	if inner, ok2 := strings.CutSuffix(inner, "</p>\n"); ok && ok2 {
		return template.HTML(inner), nil
	}
	return html, nil
}

func fixupMarkdown(n ast.Node) {
	ast.Walk(n, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch n := n.(type) {
			case *ast.Link:
				n.SetAttribute("target", text.NewMultiLineValueFromString("_blank", text.IdentityDecoder))
				// https://developers.google.com/web/tools/lighthouse/audits/noopener
				n.SetAttribute("rel", text.NewMultiLineValueFromString("noopener", text.IdentityDecoder))
			}
		}
		return ast.WalkContinue, nil
	})
}

// markdownLines marks the lines whose spaces matter, as in a code block,
// as verbatim, and those that end in a hard line break, from root, the
// syntax tree of Markdown, whose lines lineOf returns the indexes of.
func markdownLines(root ast.Node, lineOf func(offset int) int, verbatim, hardBreak []bool) {
	// keepLineEnds marks the lines that v runs over the end of, where
	// spaces are part of it, as in a code span.
	keepLineEnds := func(v text.Value) {
		idx := v.Indices()
		if v.IsOwned() || len(idx) == 0 {
			return
		}
		last := idx[len(idx)-1]
		for i := lineOf(idx[0].Start); i < lineOf(max(last.Start, last.Stop-1)); i++ {
			verbatim[i] = true
		}
	}
	ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var segs []text.Segment
		switch n := n.(type) {
		case *ast.CodeBlock:
			segs = n.Value.Segments()
		case *ast.HTMLBlock:
			segs = n.Value.Segments()
		case *ast.Text:
			if n.HardLineBreak() && n.Pos() >= 0 {
				hardBreak[lineOf(n.Pos())] = true
			}
		case *ast.CodeSpan:
			keepLineEnds(n.Value)
		case *ast.RawHTML:
			keepLineEnds(n.Value)
		case *ast.Link:
			keepLineEnds(n.Title)
		case *ast.Image:
			keepLineEnds(n.Title)
		}
		if len(segs) > 0 {
			for i := lineOf(segs[0].Start); i <= lineOf(segs[len(segs)-1].Start); i++ {
				verbatim[i] = true
			}
		}
		return ast.WalkContinue, nil
	})
}

// renderNodes renders nodes, blocks of Markdown in src, as HTML. It moves
// them out of their syntax tree.
func renderNodes(nodes []ast.Node, src []byte) (template.HTML, error) {
	doc := ast.NewDocument()
	for _, n := range nodes {
		n.Parent().RemoveChild(n)
		doc.AppendChild(n)
	}
	fixupMarkdown(doc)
	var b strings.Builder
	r := html.New(html.WithUnsafe(), html.WithExtensions(extension.NewTableHTMLRenderer()))
	if err := r.Render(&b, src, doc); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}

// parseMarkdown parses lines, a block of Markdown text, as the parser does.
// It returns the document, its source, and a function that returns the
// index in lines of the line at an offset in the source.
func parseMarkdown(lines []string) (doc ast.Node, src []byte, lineOf func(offset int) int) {
	starts := make([]int, len(lines))
	var b strings.Builder
	for i, line := range lines {
		starts[i] = b.Len()
		// As the parser does, leading tabs become four spaces each.
		short := strings.TrimLeft(line, "\t")
		b.WriteString(strings.Repeat("    ", len(line)-len(short)))
		b.WriteString(short)
		b.WriteByte('\n')
	}
	src = []byte(b.String())
	doc = parser.New(parser.WithExtensions(extension.TableParser)).Parse(src)
	lineOf = func(offset int) int {
		i, found := slices.BinarySearch(starts, offset)
		if !found {
			i--
		}
		return max(i, 0)
	}
	return doc, src, lineOf
}

// noteSpaces reports, for each of lines, notes in Markdown, whether its
// spaces matter, as in a code block, and whether it ends in a hard line
// break.
func noteSpaces(lines []string) (verbatim, hardBreak []bool) {
	verbatim = make([]bool, len(lines))
	hardBreak = make([]bool, len(lines))
	doc, _, lineOf := parseMarkdown(lines)
	markdownLines(doc, lineOf, verbatim, hardBreak)
	return verbatim, hardBreak
}
