// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"io"
)

// A LineKind is the kind of a line of a present file.
type LineKind int

const (
	BlankLine   LineKind = iota // an empty line, or one of only spaces and tabs
	TitleLine                   // the title, "# Title"
	HeaderLine                  // a line of the header after the title: the subtitle, date, tags or summary
	AuthorLine                  // a line of an author block
	HeadingLine                 // the heading of a section or subsection, "## Heading"
	CommandLine                 // a command, such as ".image file"
	NoteLine                    // presenter notes, ": note", or a line of a block of notes
	CommentLine                 // a comment, "// comment"
	TextLine                    // Markdown text, including each line of a code block
)

// A Line is a line of a present file.
type Line struct {
	Text string // the line, without its line ending
	Kind LineKind
}

// ScanLines reads a present file from r, as Parse does, and returns its
// lines. Unlike Parse, it does not run commands: it reads no files, and a
// command that is unknown or invalid is not an error.
func ScanLines(r io.Reader, name string) ([]Line, error) {
	f, err := parseFile(r, name, 0)
	if err != nil {
		return nil, err
	}
	out := make([]Line, len(f.lines))
	for i, text := range f.lines {
		out[i] = Line{Text: text, Kind: f.kinds[i]}
	}
	return out, nil
}
