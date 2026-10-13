// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"regexp"
	"strings"
)

// isBlank reports whether line is blank: empty, or only spaces and tabs.
func isBlank(line string) bool {
	return strings.Trim(line, " \t") == ""
}

// isHeading matches the heading of a section: #s, then a space or nothing.
var isHeading = regexp.MustCompile(`^\#+( |$)`)

// IsNotesBlockStart reports whether line starts a block of notes, which
// runs up to a line for which IsNotesBlockEnd reports true.
func IsNotesBlockStart(line string) bool {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), ":::")
	return ok && strings.TrimSpace(rest) == "notes"
}

// IsNotesBlockEnd reports whether line ends a block of notes.
func IsNotesBlockEnd(line string) bool {
	return strings.TrimSpace(line) == ":::"
}

// notesBlockEnd returns the index of the line that ends the block of notes
// that lines[start] starts, or -1 if none does. A ":::" line in a fenced
// code block in the block of notes does not end it.
func notesBlockEnd(lines []string, start int) int {
	fence := ""
	for i := start + 1; i < len(lines); i++ {
		switch {
		case fence != "":
			if closesFence(lines[i], fence) {
				fence = ""
			}
		case IsNotesBlockEnd(lines[i]):
			return i
		default:
			if f, ok := codeFence(lines[i]); ok {
				fence = f
			}
		}
	}
	return -1
}

// codeFence returns the fence that line opens a fenced code block with, if
// it does: at least three backticks or tildes, after at most three spaces,
// and with no backtick after backticks, as in CommonMark.
func codeFence(line string) (fence string, ok bool) {
	rest := strings.TrimLeft(line, " ")
	if len(line)-len(rest) > 3 || rest == "" || (rest[0] != '`' && rest[0] != '~') {
		return "", false
	}
	info := strings.TrimLeft(rest, rest[:1])
	fence = rest[:len(rest)-len(info)]
	if len(fence) < 3 || (fence[0] == '`' && strings.Contains(info, "`")) {
		return "", false
	}
	return fence, true
}

// closesFence reports whether line closes a fenced code block opened by
// fence: it has at least as many of the same character, after at most
// three spaces, and nothing else but spaces.
func closesFence(line, fence string) bool {
	rest := strings.TrimLeft(line, " ")
	if len(line)-len(rest) > 3 || !strings.HasPrefix(rest, fence) {
		return false
	}
	return strings.TrimSpace(strings.TrimLeft(rest, fence[:1])) == ""
}

// splitHeading splits the text of a heading after its #s into its title
// and its HTML anchor ID, which it may end in as {#id}. Sections can end in
// {#id} to set the HTML anchor for the section, which is nicer than the
// default #TOC_1_2-style anchor.
func splitHeading(text string) (title, id string) {
	title = strings.TrimSpace(text)
	if rest, ok := strings.CutSuffix(title, "}"); ok {
		if before, after, ok := strings.CutLast(rest, "{#"); ok {
			title, id = strings.TrimSpace(before), after
		}
	}
	return title, id
}

func isSpeakerNote(s string) bool {
	return strings.HasPrefix(s, ": ") || s == ":"
}

func trimSpeakerNote(s string) string {
	if s == ":" {
		return ""
	}
	return strings.TrimPrefix(s, ": ")
}
