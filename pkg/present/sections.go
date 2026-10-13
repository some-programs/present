// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/text"
)

// The sections of a present file are Markdown, in which commands, notes
// and blocks of notes are blocks of their own, at the start of a line and
// outside other blocks, such as lists and code blocks. Comments are taken
// out of the Markdown, so that they do not end a paragraph or a list, but
// for those in code blocks, HTML blocks and blocks of notes.

// sections are the sections of a present file, as Markdown.
type sections struct {
	src    []byte   // the lines of the sections, but for comments, with leading tabs as spaces
	lines  []int    // the index in the file of each line of src
	starts []int    // the offset in src of each line
	root   ast.Node // the syntax tree of src
}

// newSections parses the lines of a present file with the indexes idx as
// Markdown.
func newSections(lines []string, idx []int) *sections {
	b := &sections{lines: idx}
	var buf bytes.Buffer
	for _, i := range idx {
		b.starts = append(b.starts, buf.Len())
		// Leading tabs become four spaces each, which render better in
		// code blocks. CommonMark defines that a tab is four spaces
		// for the structure of the text, so it does not change that.
		line := lines[i]
		short := strings.TrimLeft(line, "\t")
		buf.WriteString(strings.Repeat("    ", len(line)-len(short)))
		buf.WriteString(short)
		buf.WriteByte('\n')
	}
	b.src = buf.Bytes()
	b.root = sectionsParser().Parse(b.src)
	return b
}

// line returns the index in the file of the line at offset in b.src.
func (b *sections) line(offset int) int {
	i, found := slices.BinarySearch(b.starts, offset)
	if !found {
		i--
	}
	return b.lines[max(i, 0)]
}

// blockLines marks the lines of the file in code blocks, HTML blocks and
// blocks of notes in b, whose spaces and comments are kept.
func (b *sections) blockLines(n int) []bool {
	marked := make([]bool, n)
	ast.Walk(b.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var segs []text.Segment
		switch n := n.(type) {
		case *ast.CodeBlock:
			segs = n.Value.Segments()
		case *ast.HTMLBlock:
			segs = n.Value.Segments()
		case *notesBlockNode:
			for i := b.line(n.Pos()); i <= b.line(n.end); i++ {
				marked[i] = true
			}
		}
		if len(segs) > 0 {
			for i := b.line(segs[0].Start); i <= b.line(segs[len(segs)-1].Start); i++ {
				marked[i] = true
			}
		}
		return ast.WalkContinue, nil
	})
	return marked
}

// parseSections parses the sections of f, from the line with the index start
// to the end.
func (f *file) parseSections(start int) error {
	var idx []int
	for i := start; i < len(f.lines); i++ {
		idx = append(idx, i)
	}
	// Comments are taken out, but for those in blocks, which the Markdown
	// with comments tells.
	blocks := newSections(f.lines, idx).blockLines(len(f.lines))
	idx = idx[:0]
	for i := start; i < len(f.lines); i++ {
		if strings.HasPrefix(f.lines[i], "//") && !blocks[i] {
			f.kinds[i] = CommentLine
			continue
		}
		idx = append(idx, i)
	}
	b := newSections(f.lines, idx)
	f.sections = b
	markdownLines(b.root, b.line, f.verbatim, f.hardBreak)

	for _, i := range idx {
		if !isBlank(f.lines[i]) || f.verbatim[i] {
			f.kinds[i] = TextLine
		}
	}
	level := 1 // of the open section, or the title
	for n := b.root.FirstChild(); n != nil; n = n.NextSibling() {
		line := b.line(n.Pos())
		// A section starts with a heading of #s, not with one that is
		// underlined, though its text may start with #s.
		h, ok := n.(*ast.Heading)
		if ok && isHeading.MatchString(f.lines[line]) && len(f.lines[line])-len(strings.TrimLeft(f.lines[line], "#")) == h.Level {
			f.kinds[line] = HeadingLine
			switch {
			case h.Level == 1:
				return fmt.Errorf("%s:%d: unexpected heading %q: only the title begins with \"# \", and sections with \"## \"", f.name, line+1, f.lines[line])
			case h.Level > level+1:
				return fmt.Errorf("%s:%d: badly nested section inside %s: %s", f.name, line+1, strings.Repeat("#", level), f.lines[line])
			}
			level = h.Level
		} else if level == 1 {
			return fmt.Errorf("%s:%d: unexpected line: %s", f.name, line+1, f.lines[line])
		}
		switch n := n.(type) {
		case *commandNode:
			f.kinds[line] = CommandLine
		case *noteNode:
			f.kinds[line] = NoteLine
		case *notesBlockNode:
			end := b.line(n.end)
			if !n.closed {
				return fmt.Errorf("%s:%d: notes block is not closed by a line with :::", f.name, line+1)
			}
			f.notes = append(f.notes, [2]int{line, end})
			for i := line; i <= end; i++ {
				f.kinds[i] = NoteLine
			}
		case *ast.CodeBlock:
			if n.CodeBlockKind != ast.CodeBlockKindFenced {
				break
			}
			// An open fenced code block runs to the end of the file,
			// and would hide the sections after it.
			fence, _ := codeFence(f.lines[line])
			last := line
			if segs := n.Value.Segments(); len(segs) > 0 {
				last = b.line(segs[len(segs)-1].Start)
			}
			if j := slices.Index(b.lines, last) + 1; j >= len(b.lines) || !closesFence(f.lines[b.lines[j]], fence) {
				return fmt.Errorf("%s:%d: fenced code block is not closed by a line with %s", f.name, line+1, fence)
			}
		case *ast.HTMLBlock:
			// So would an HTML block that ends only at a closing tag
			// or marker.
			if end := htmlBlockEnd(n, f.lines[line]); end != "" {
				segs := n.Value.Segments()
				last := f.lines[b.line(segs[len(segs)-1].Start)]
				if !strings.Contains(strings.ToLower(last), end) {
					return fmt.Errorf("%s:%d: HTML block is not closed by a line with %s", f.name, line+1, end)
				}
			}
		}
	}
	return nil
}

// htmlBlockEnd returns what ends the HTML block n, whose first line is
// line, if a blank line does not: a closing tag such as </pre>, or the end
// of a comment, -->, or similar.
func htmlBlockEnd(n *ast.HTMLBlock, line string) string {
	switch n.HTMLBlockKind {
	case ast.HTMLBlockKind1:
		lower := strings.ToLower(line)
		for _, tag := range []string{"script", "pre", "style", "textarea"} {
			if strings.Contains(lower, "<"+tag) {
				return "</" + tag + ">"
			}
		}
	case ast.HTMLBlockKind2:
		return "-->"
	case ast.HTMLBlockKind3:
		return "?>"
	case ast.HTMLBlockKind4:
		return ">"
	case ast.HTMLBlockKind5:
		return "]]>"
	}
	return ""
}

// A sectionBuilder builds a section, from its elements and subsections.
type sectionBuilder struct {
	s     Section
	elems []any // an Elem, or a *sectionBuilder for a subsection
	subs  int   // the number of subsections
}

func (sb *sectionBuilder) section() Section {
	s := sb.s
	for _, e := range sb.elems {
		switch e := e.(type) {
		case *sectionBuilder:
			s.Elem = append(s.Elem, e.section())
		case Elem:
			s.Elem = append(s.Elem, e)
		}
	}
	return s
}

// build adds the sections to f.doc, and returns it. It runs the commands
// with ctx, or with ctx nil, leaves them as command elements. It takes
// the syntax tree of the sections apart, so it can be called only once.
func (f *file) build(ctx *Context) (*Doc, error) {
	b := f.sections
	var top []*sectionBuilder
	var stack []*sectionBuilder // the open sections, innermost last
	var markdown []ast.Node     // Markdown not yet added to a section
	add := func(e Elem) {
		cur := stack[len(stack)-1]
		cur.elems = append(cur.elems, e)
	}
	flush := func() error {
		if len(markdown) == 0 {
			return nil
		}
		html, err := renderNodes(markdown, b.src)
		if err != nil {
			return err
		}
		add(HTML{HTML: html})
		markdown = nil
		return nil
	}

	var nodes []ast.Node
	for n := b.root.FirstChild(); n != nil; n = n.NextSibling() {
		nodes = append(nodes, n)
	}
	for _, n := range nodes {
		line := b.line(n.Pos())
		text, num := f.lines[line], line+1
		switch n := n.(type) {
		case *ast.Heading:
			if f.kinds[line] != HeadingLine {
				markdown = append(markdown, n)
				continue
			}
			if err := flush(); err != nil {
				return nil, err
			}
			// parseSections checked the levels of headings.
			stack = stack[:n.Level-2]
			title, id := splitHeading(text[n.Level:])
			sb := &sectionBuilder{s: Section{Title: title, ID: id, Line: num}}
			if len(stack) == 0 {
				top = append(top, sb)
				sb.s.Number = []int{len(top)}
			} else {
				parent := stack[len(stack)-1]
				parent.subs++
				sb.s.Number = append(slices.Clone(parent.s.Number), parent.subs)
				parent.elems = append(parent.elems, sb)
			}
			stack = append(stack, sb)
		case *noteNode:
			if err := flush(); err != nil {
				return nil, err
			}
			cur := stack[len(stack)-1]
			cur.s.Notes = append(cur.s.Notes, trimSpeakerNote(text))
		case *notesBlockNode:
			if err := flush(); err != nil {
				return nil, err
			}
			cur := stack[len(stack)-1]
			cur.s.Notes = append(cur.s.Notes, strings.Join(f.lines[line+1:b.line(n.end)], "\n"))
		case *commandNode:
			if err := flush(); err != nil {
				return nil, err
			}
			if ctx == nil {
				add(command{Cmd: strings.TrimSpace(text)})
				continue
			}
			args := strings.Fields(text)
			if args[0] == ".background" {
				if len(args) != 2 {
					return nil, fmt.Errorf("%s:%d: .background expects exactly one argument", f.name, num)
				}
				cur := stack[len(stack)-1]
				cur.s.Background = args[1]
				cur.s.Classes = append(cur.s.Classes, "background")
				cur.s.Styles = append(cur.s.Styles, "background-image: url('"+args[1]+"')")
				continue
			}
			parse := parsers[args[0]]
			if parse == nil {
				return nil, fmt.Errorf("%s:%d: unknown command %q", f.name, num, text)
			}
			e, err := parse(ctx, f.name, num, text)
			if err != nil {
				return nil, err
			}
			add(e)
		default:
			markdown = append(markdown, n)
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	for _, sb := range top {
		f.doc.Sections = append(f.doc.Sections, sb.section())
	}
	return f.doc, nil
}

// A command is a command, such as ".image file", that is not run.
type command struct {
	Cmd string
}

func (command) TemplateName() string { return "command" }
