// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

// A Context specifies the supporting context for parsing a presentation.
type Context struct {
	// ReadFile reads the file named by filename and returns the contents.
	ReadFile func(filename string) ([]byte, error)
}

// ParseMode represents flags for the Parse function.
type ParseMode int

const (
	// If set, parse only the title and subtitle.
	TitlesOnly ParseMode = 1
)

// Parse parses a document from r.
func (ctx *Context) Parse(r io.Reader, name string, mode ParseMode) (*Doc, error) {
	f, err := parseFile(r, name, mode)
	if err != nil {
		return nil, err
	}
	if mode&TitlesOnly != 0 {
		return f.doc, nil
	}
	return f.build(ctx)
}

// Parse parses a document from r. Parse reads assets used by the presentation
// from the file system using os.ReadFile.
func Parse(r io.Reader, name string, mode ParseMode) (*Doc, error) {
	ctx := Context{ReadFile: os.ReadFile}
	return ctx.Parse(r, name, mode)
}

// A file is a present file, parsed, but with its commands not run.
type file struct {
	name  string
	lines []string   // the lines of the file, as read
	kinds []LineKind // the kind of each line
	doc   *Doc       // the header and the authors, until build adds the sections

	// notes are the blocks of notes, by the indexes of the lines that
	// start and end them.
	notes [][2]int

	// sections are the sections, or nil with TitlesOnly.
	sections *sections

	// verbatim marks the lines of the sections whose spaces matter, as in
	// code, and hardBreak those that end in a hard line break.
	verbatim, hardBreak []bool
}

// parseFile parses the present file in r, called name, without running
// its commands.
func parseFile(r io.Reader, name string, mode ParseMode) (*file, error) {
	text, err := readText(r)
	if err != nil {
		return nil, err
	}
	f := &file{
		name:      name,
		lines:     text,
		kinds:     make([]LineKind, len(text)),
		doc:       new(Doc),
		verbatim:  make([]bool, len(text)),
		hardBreak: make([]bool, len(text)),
	}

	// The title line must begin with "# ".
	// Files without it use the legacy present markup, which is not supported.
	for i, line := range text {
		if isBlank(line) {
			continue
		}
		if !strings.HasPrefix(line, "# ") {
			return nil, fmt.Errorf("%s:%d: title must begin with \"# \" (legacy present markup is not supported; convert it with present convert-legacy)", name, i+1)
		}
		break
	}

	// The header and the authors run up to the first section, after the
	// blank line that ends the header. Each block of notes in them becomes
	// a single note, on the line that starts the block, whose other lines
	// become comments, so that lines keep their numbers.
	start := len(text)
	header := slices.Clone(text)
	title, inHeader := false, true
	for i := 0; i < len(text); i++ {
		if IsNotesBlockStart(text[i]) {
			end := notesBlockEnd(text, i)
			if end < 0 {
				return nil, fmt.Errorf("%s:%d: notes block is not closed by a line with :::", name, i+1)
			}
			f.notes = append(f.notes, [2]int{i, end})
			header[i] = ": " + strings.Join(text[i+1:end], "\n")
			for j := i + 1; j <= end; j++ {
				header[j] = "//"
				f.kinds[j] = NoteLine
			}
			i = end
			continue
		}
		switch {
		case !title:
			title = !isBlank(text[i])
		case inHeader:
			inHeader = !isBlank(text[i])
		case strings.HasPrefix(text[i], "##"):
			start = i
		}
		if start < len(text) {
			break
		}
	}
	header = header[:start]
	for _, line := range header {
		if isSpeakerNote(line) {
			f.doc.TitleNotes = append(f.doc.TitleNotes, trimSpeakerNote(line))
		}
	}
	lines := &lineReader{text: header, kinds: f.kinds[:start], comment: "//"}
	if err := parseHeader(f.doc, name, lines); err != nil {
		return nil, err
	}
	if mode&TitlesOnly != 0 {
		return f, nil
	}
	if f.doc.Authors, err = parseAuthors(name, lines); err != nil {
		return nil, err
	}
	if start == len(text) {
		return nil, fmt.Errorf("%s:%d: unexpected end of file; expected a section, which begins with \"## \"", name, len(text))
	}
	if err := f.parseSections(start); err != nil {
		return nil, err
	}
	return f, nil
}

// readText returns the lines of r, without their line endings.
func readText(r io.Reader) ([]string, error) {
	var lines []string
	s := bufio.NewScanner(r)
	for s.Scan() {
		lines = append(lines, s.Text())
	}
	return lines, s.Err()
}
