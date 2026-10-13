// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"bytes"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// Format formats src, the contents of the present file name, and returns
// the result. It changes only spaces and blank lines, and never how the
// file presents:
//
//   - Lines end in a newline, not a carriage return and a newline, and the
//     file in a single one.
//   - Lines have no trailing spaces, but for those of code blocks and HTML.
//     A line break made by trailing spaces in Markdown becomes a
//     backslash, which can be seen.
//   - Blank lines in a row become one, but for those in code blocks, and
//     the file starts with the title.
//   - Headings, commands, notes and Markdown text are set apart by a blank
//     line, but for comments, which stay with what they are next to.
//     Commands in a row stay together, as do notes in a row.
//   - The title and headings have a single space after their #s, and before
//     an {#id}, and a block of notes is set off by "::: notes" and ":::".
//
// Format returns an error if src is not a valid present file, or has a
// carriage return that does not end a line. It does not run commands, so
// it reads no files.
func Format(name string, src []byte) ([]byte, error) {
	f, err := parseFile(bytes.NewReader(src), name, 0)
	if err != nil {
		return nil, err
	}
	// A carriage return ends a line in Markdown, but not for the parser.
	for i, line := range f.lines {
		if strings.Contains(line, "\r") {
			return nil, fmt.Errorf("%s:%d: carriage return in the middle of a line", name, i+1)
		}
	}
	// Should trailing spaces in Markdown matter in a way that
	// markdownLines does not know of, they are kept, and should a blank
	// line that sets text apart become part of it, as in a code block
	// left open in a list, no blank lines are added.
	var formatted [][]byte
	for _, o := range []formatOptions{{}, {keepSpaces: true}, {keepSpaces: true, keepBlankLines: true}} {
		formatted = append(formatted, formatText(f, o))
	}
	doc, err := f.build(nil)
	if err != nil {
		return nil, err
	}
	// Formatting must not change how the file presents. Should it change
	// even so, that is a bug here, and the file is better left as it is.
	for _, out := range formatted {
		f2, err := parseFile(bytes.NewReader(out), name, 0)
		var doc2 *Doc
		if err == nil {
			doc2, err = f2.build(nil)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: formatting would make the file invalid, which is a bug in present: %v", name, err)
		}
		if samePresentation(doc, doc2) {
			return out, nil
		}
	}
	return nil, fmt.Errorf("%s: formatting would change how the file presents, which is a bug in present; the file is left as it is", name)
}

// formatOptions tell formatText what to leave as it is.
type formatOptions struct {
	keepSpaces     bool // the trailing spaces of Markdown text
	keepBlankLines bool // where blank lines are, but for blank lines in a row
}

// formatText formats f, as Format does, but for what o keeps.
func formatText(f *file, o formatOptions) []byte {
	text, kinds := f.lines, f.kinds
	verbatim := slices.Clone(f.verbatim)   // lines whose spaces matter
	hardBreak := slices.Clone(f.hardBreak) // lines that end in a hard line break
	markdown := make([]bool, len(text))    // lines of Markdown text
	notesStart := make(map[int]bool)
	notesEnd := make(map[int]bool)
	inNotes := make([]bool, len(text))
	for _, b := range f.notes {
		notesStart[b[0]], notesEnd[b[1]] = true, true
		for i := b[0]; i <= b[1]; i++ {
			inNotes[i] = true
		}
		md := text[b[0]+1 : b[1]]
		v, h := noteSpaces(md)
		for j := range md {
			i := b[0] + 1 + j
			markdown[i], verbatim[i], hardBreak[i] = true, v[j], h[j]
		}
	}
	for i, kind := range kinds {
		switch {
		case kind == TextLine:
			markdown[i] = true
		case kind == NoteLine && !inNotes[i]:
			v, _ := noteSpaces([]string{trimSpeakerNote(text[i])})
			markdown[i], verbatim[i] = true, v[0]
		}
	}
	firstHeading := slices.Index(kinds, HeadingLine)

	var out []string
	blanks := 0
	prev := -1 // the last line written that is not blank
	for i, line := range text {
		kind := kinds[i]
		if isBlank(line) && !verbatim[i] {
			blanks++
			continue
		}
		if prev >= 0 {
			n := min(blanks, 1)
			pk := kinds[prev]
			switch {
			case o.keepBlankLines:
			case pk == HeadingLine:
				n = 1
			case kind == HeadingLine && pk != CommentLine:
				n = 1
			case prev < firstHeading:
				// Blank lines end the header and the authors.
			case pk == CommentLine || kind == CommentLine:
			case pk != kind:
				n = 1
			}
			for range n {
				out = append(out, "")
			}
		}
		blanks = 0
		prev = i

		switch {
		case verbatim[i]:
		case notesStart[i]:
			line = "::: notes"
		case notesEnd[i]:
			line = ":::"
		case kind == TitleLine:
			if title := strings.TrimSpace(strings.TrimPrefix(line, "#")); title != "" {
				line = "# " + title
			}
		case kind == HeadingLine:
			line = formatHeading(line)
		case markdown[i] && o.keepSpaces:
		default:
			trimmed := strings.TrimRight(line, " \t")
			switch {
			case isHeading.MatchString(trimmed) != isHeading.MatchString(line),
				isSpeakerNote(trimmed) != isSpeakerNote(line):
				// Without its trailing spaces and tabs, the line would
				// be a heading or a note, so it is left as it is.
			case !hardBreak[i] || trimmed == line:
				line = trimmed
			case !strings.HasSuffix(trimmed, `\`):
				line = trimmed + `\`
				// A backslash before the trailing spaces that make the
				// line break is not one itself, so that line is left as
				// it is.
			}
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n") + "\n")
}

// formatHeading formats the heading line, with a single space after its #s
// and before its {#id}, if any.
func formatHeading(line string) string {
	level := len(line) - len(strings.TrimLeft(line, "#"))
	title, id := splitHeading(line[level:])
	s := line[:level]
	if title != "" {
		s += " " + title
	}
	if id != "" {
		s += " {#" + id + "}"
	}
	return s
}

// samePresentation reports whether the documents a and b, from build,
// present the same. It changes them.
func samePresentation(a, b *Doc) bool {
	normalizeDoc(a)
	normalizeDoc(b)
	return reflect.DeepEqual(a, b)
}

// normalizeDoc leaves what in doc does not change how it presents out of
// it: the lines of sections, and how notes are written.
func normalizeDoc(doc *Doc) {
	doc.TitleNotes = renderNotes(doc.TitleNotes)
	for i := range doc.Sections {
		normalizeSection(&doc.Sections[i])
	}
}

func normalizeSection(s *Section) {
	s.Line = 0
	s.Notes = renderNotes(s.Notes)
	for i, e := range s.Elem {
		if ss, ok := e.(Section); ok {
			normalizeSection(&ss)
			s.Elem[i] = ss
		}
	}
}

// renderNotes renders each of notes as Markdown.
func renderNotes(notes []string) []string {
	var out []string
	for _, note := range notes {
		html, err := renderMarkdown([]byte(note))
		if err != nil {
			out = append(out, err.Error())
			continue
		}
		out = append(out, string(html))
	}
	return out
}
