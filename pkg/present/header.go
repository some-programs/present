// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"
)

// A lineReader reads the lines of the header and the authors of a
// present file.
type lineReader struct {
	line    int // 0 indexed, so has 1-indexed number of last line returned
	text    []string
	kinds   []LineKind // the kind of each line, as the parser finds it
	comment string
}

// mark sets the kind of the last line returned.
func (l *lineReader) mark(kind LineKind) {
	l.kinds[l.line-1] = kind
}

func (l *lineReader) next() (text string, ok bool) {
	for {
		current := l.line
		l.line++
		if current >= len(l.text) {
			return "", false
		}
		text = l.text[current]
		// Lines starting with l.comment are comments.
		if l.comment == "" || !strings.HasPrefix(text, l.comment) {
			ok = true
			break
		}
		// The lines of a block of notes are comments once it is
		// folded, but are notes to ScanLines.
		if l.kinds[current] == BlankLine {
			l.kinds[current] = CommentLine
		}
	}
	return
}

// nextNonEmpty returns the next line that is not blank.
func (l *lineReader) nextNonEmpty() (text string, ok bool) {
	for {
		text, ok = l.next()
		if !ok {
			return
		}
		if !isBlank(text) {
			break
		}
	}
	return
}

func parseHeader(doc *Doc, name string, lines *lineReader) error {
	var ok bool
	// First non-empty line starts header.
	doc.Title, ok = lines.nextNonEmpty()
	if !ok {
		return fmt.Errorf("%s:%d: unexpected end of file; expected the title", name, lines.line)
	}
	lines.mark(TitleLine)
	doc.Title = strings.TrimSpace(strings.TrimPrefix(doc.Title, "#"))

	for {
		text, ok := lines.next()
		if !ok {
			return fmt.Errorf("%s:%d: unexpected end of file; expected a blank line after the header", name, lines.line-1)
		}
		if isBlank(text) {
			break
		}
		if isSpeakerNote(text) {
			lines.mark(NoteLine)
			continue
		}
		lines.mark(HeaderLine)
		// Trailing spaces, which cannot be seen, would keep the line
		// from being a date.
		text = strings.TrimRight(text, " \t")
		if tags, ok := strings.CutPrefix(text, "Tags:"); ok {
			for tag := range strings.SplitSeq(tags, ",") {
				doc.Tags = append(doc.Tags, strings.TrimSpace(tag))
			}
		} else if summary, ok := strings.CutPrefix(text, "Summary:"); ok {
			doc.Summary = strings.TrimSpace(summary)
		} else if t, ok := parseTime(text); ok {
			doc.Time = t
		} else if doc.Subtitle == "" {
			doc.Subtitle = text
		} else {
			return fmt.Errorf("%s:%d: unexpected header line %q: the header has a subtitle already, and this is not a date (2 Jan 2006 or 15:04 2 Jan 2006), Tags: or Summary:", name, lines.line, text)
		}
	}
	return nil
}

// parseAuthors parses the author blocks in lines, which end with them.
func parseAuthors(name string, lines *lineReader) (authors []Author, err error) {
	// This grammar demarcates authors with blanks.
	var a *Author
	for {
		text, ok := lines.next()
		if !ok {
			break
		}

		if isSpeakerNote(text) {
			lines.mark(NoteLine)
			continue
		}

		// If we encounter a blank we're done with this author.
		if isBlank(text) {
			if a != nil {
				authors = append(authors, *a)
				a = nil
			}
			continue
		}
		lines.mark(AuthorLine)
		text = strings.TrimRight(text, " \t")
		if a == nil {
			a = new(Author)
		}

		// Parse the line. Those that
		// - begin with @ are twitter names,
		// - contain slashes are links, or
		// - contain an @ symbol are an email address.
		// The rest is just text.
		var el Elem
		switch {
		case strings.HasPrefix(text, "@"):
			el = parseAuthorURL(name, "http://twitter.com/"+text[1:])
		case strings.Contains(text, ":"):
			el = parseAuthorURL(name, text)
		case strings.Contains(text, "@"):
			el = parseAuthorURL(name, "mailto:"+text)
		}
		if l, ok := el.(Link); ok {
			l.Label = text
			el = l
		}
		if el == nil {
			el = Text{Lines: []string{text}}
		}
		a.Elem = append(a.Elem, el)
	}
	if a != nil {
		authors = append(authors, *a)
	}
	return authors, nil
}

func parseAuthorURL(name, text string) Elem {
	u, err := url.Parse(text)
	if err != nil {
		log.Printf("parsing %s author block: invalid URL %q: %v", name, text, err)
		return nil
	}
	return Link{URL: u}
}

func parseTime(text string) (t time.Time, ok bool) {
	t, err := time.Parse("15:04 2 Jan 2006", text)
	if err == nil {
		return t, true
	}
	t, err = time.Parse("2 Jan 2006", text)
	if err == nil {
		// at 11am UTC it is the same date everywhere
		t = t.Add(time.Hour * 11)
		return t, true
	}
	return time.Time{}, false
}
