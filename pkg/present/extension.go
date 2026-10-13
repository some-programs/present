// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"
	"github.com/yuin/goldmark/v2/util"
)

// sectionsParser returns a parser of the Markdown of sections.
func sectionsParser() parser.Parser {
	return parser.New(
		parser.WithExtensions(extension.TableParser),
		parser.WithBlockParsers(
			util.PrioritizedValue[parser.BlockParser]{Value: notesBlockParser{}, Priority: 10},
			util.PrioritizedValue[parser.BlockParser]{Value: noteParser{}, Priority: 11},
			util.PrioritizedValue[parser.BlockParser]{Value: commandParser{}, Priority: 10},
		),
	)
}

// The nodes of the syntax tree of sections for present's own blocks.
var (
	kindCommand    = ast.NewNodeKind("PresentCommand")
	kindNote       = ast.NewNodeKind("PresentNote")
	kindNotesBlock = ast.NewNodeKind("PresentNotesBlock")
)

// A commandNode is a command, such as ".image file".
type commandNode struct{ ast.BaseBlock }

func (n *commandNode) Kind() ast.NodeKind          { return kindCommand }
func (n *commandNode) Dump(_ []byte) *ast.NodeDump { return ast.NewNodeDump(n, nil) }

// A noteNode is a line of notes, ": note".
type noteNode struct{ ast.BaseBlock }

func (n *noteNode) Kind() ast.NodeKind          { return kindNote }
func (n *noteNode) Dump(_ []byte) *ast.NodeDump { return ast.NewNodeDump(n, nil) }

// A notesBlockNode is a block of notes, from a "::: notes" line to a ":::"
// line.
type notesBlockNode struct {
	ast.BaseBlock
	end    int    // the offset of its last line
	closed bool   // whether a ":::" line ends it
	fence  string // the fence of a code block open in it
}

func (n *notesBlockNode) Kind() ast.NodeKind          { return kindNotesBlock }
func (n *notesBlockNode) Dump(_ []byte) *ast.NodeDump { return ast.NewNodeDump(n, nil) }

// peekLine returns the current line of reader, without its line ending.
func peekLine(reader text.Reader) (string, text.Segment) {
	line, seg := reader.PeekLine()
	return strings.TrimRight(string(line), "\n"), seg
}

// topLevel reports whether a block that starts at the start of a line in
// parent is at the top level of a section, outside other blocks.
func topLevel(parent ast.Node, pc parser.Context) bool {
	return parent.Kind() == ast.KindDocument && pc.BlockIndent() == 0
}

// commandParser parses commands.
type commandParser struct{}

func (commandParser) Trigger() []byte { return []byte{'.'} }

func (commandParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	if !topLevel(parent, pc) {
		return nil, parser.NoChildren
	}
	n := &commandNode{}
	n.Init(n)
	reader.AdvanceToEOL()
	return n, parser.NoChildren
}

func (commandParser) Continue(ast.Node, text.Reader, parser.Context) parser.State {
	return parser.Close
}

func (commandParser) Close(ast.Node, text.Reader, parser.Context) {}
func (commandParser) CanInterruptParagraph() bool                 { return true }
func (commandParser) CanAcceptIndentedLine() bool                 { return false }

// noteParser parses lines of notes.
type noteParser struct{}

func (noteParser) Trigger() []byte { return []byte{':'} }

func (noteParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	if line, _ := peekLine(reader); !topLevel(parent, pc) || !isSpeakerNote(line) {
		return nil, parser.NoChildren
	}
	n := &noteNode{}
	n.Init(n)
	reader.AdvanceToEOL()
	return n, parser.NoChildren
}

func (noteParser) Continue(ast.Node, text.Reader, parser.Context) parser.State { return parser.Close }
func (noteParser) Close(ast.Node, text.Reader, parser.Context)                 {}
func (noteParser) CanInterruptParagraph() bool                                 { return true }
func (noteParser) CanAcceptIndentedLine() bool                                 { return false }

// notesBlockParser parses blocks of notes.
type notesBlockParser struct{}

func (notesBlockParser) Trigger() []byte { return []byte{':'} }

func (notesBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, seg := peekLine(reader)
	if parent.Kind() != ast.KindDocument || !IsNotesBlockStart(line) {
		return nil, parser.NoChildren
	}
	n := &notesBlockNode{end: seg.Start}
	n.Init(n)
	reader.AdvanceToEOL()
	return n, parser.NoChildren
}

func (notesBlockParser) Continue(node ast.Node, reader text.Reader, _ parser.Context) parser.State {
	n := node.(*notesBlockNode)
	line, seg := peekLine(reader)
	n.end = seg.Start
	reader.AdvanceToEOL()
	switch {
	case n.fence != "":
		if closesFence(line, n.fence) {
			n.fence = ""
		}
	case IsNotesBlockEnd(line):
		n.closed = true
		return parser.Close
	default:
		if fence, ok := codeFence(line); ok {
			n.fence = fence
		}
	}
	return parser.Continue | parser.NoChildren
}

func (notesBlockParser) Close(ast.Node, text.Reader, parser.Context) {}
func (notesBlockParser) CanInterruptParagraph() bool                 { return true }
func (notesBlockParser) CanAcceptIndentedLine() bool                 { return false }
