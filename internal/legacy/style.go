// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package legacy

// The inline markup is converted by a copy of golang.org/x/tools/cmd/present2md,
// which is itself a copy of golang.org/x/tools/present/style.go adjusted to
// produce Markdown instead of HTML.

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

/*
	Fonts are demarcated by an initial and final char bracketing a
	space-delimited word, plus possibly some terminal punctuation.
	The chars are
		_ for italic
		* for bold
		` (back quote) for fixed width.
	Inner appearances of the char become spaces. For instance,
		_this_is_italic_!
	becomes
		<i>this is italic</i>!
*/

// styleLine converts s, a line of text with legacy inline markup, to a line
// of Markdown that renders the same, wherever it is in a block of text.
func styleLine(s string) string {
	return escapeLineStart(font(s, false))
}

// font returns s with font indicators and inline links turned into
// Markdown, and the rest escaped. In a link's text, inLink is true and
// brackets are escaped.
func font(s string, inLink bool) string {
	// Brackets only make a link if a "](" follows them.
	brackets := inLink || strings.Contains(s, "](")
	esc := func(s string) string { return escapeText(s, brackets) }
	if !strings.ContainsAny(s, "[`_*") {
		return esc(s)
	}
	words := split(s)
	var b strings.Builder
Word:
	for w, word := range words {
		words[w] = esc(word) // for all the continue Word
		if len(word) < 2 {
			continue Word
		}
		if link, _ := parseInlineLink(word); link != "" {
			words[w] = link
			continue Word
		}
		const marker = "_*`"
		// Initial punctuation is OK but must be peeled off.
		first := strings.IndexAny(word, marker)
		if first == -1 {
			continue Word
		}
		// Opening marker must be at the beginning of the token or else preceded by punctuation.
		if first != 0 {
			r, _ := utf8.DecodeLastRuneInString(word[:first])
			if !isPunct(r) {
				continue Word
			}
		}
		open, word := esc(word[:first]), word[first:]
		char := word[0] // ASCII is OK.
		close := ""
		switch char {
		default:
			continue Word
		case '_':
			open += "*"
			close = "*"
		case '*':
			open += "**"
			close = "**"
		case '`':
			open += "`"
			close = "`"
		}
		// Closing marker must be at the end of the token or else followed by punctuation.
		last := strings.LastIndex(word, word[:1])
		if last == 0 {
			continue Word
		}
		if last+1 != len(word) {
			r, _ := utf8.DecodeRuneInString(word[last+1:])
			if !isPunct(r) {
				continue Word
			}
		}
		head, tail := word[:last+1], word[last+1:]
		b.Reset()
		var wid int
		for i := 1; i < len(head)-1; i += wid {
			var r rune
			r, wid = utf8.DecodeRuneInString(head[i:])
			if r != rune(char) {
				// Ordinary character.
				b.WriteRune(r)
				continue
			}
			if head[i+1] != char {
				// Inner char becomes space.
				b.WriteRune(' ')
				continue
			}
			// Doubled char becomes real char.
			// Not worth worrying about "_x__".
			b.WriteByte(char)
			wid++ // Consumed two chars, both ASCII.
		}
		text := b.String()
		if close == "`" {
			for strings.Contains(text, close) {
				open += "`"
				close += "`"
			}
		} else {
			text = esc(text)
		}
		words[w] = open + text + close + esc(tail)
	}
	return strings.Join(words, "")
}

// isPunct reports whether r is punctuation for the font markers. The legacy
// markup applies them to HTML-escaped text, so '<' and '>', which become
// "&lt;" and "&gt;", count as punctuation too.
func isPunct(r rune) bool {
	return unicode.IsPunct(r) || r == '<' || r == '>'
}

// split is like strings.Fields but also returns the runs of spaces
// and treats inline links as distinct words.
func split(s string) []string {
	var (
		words = make([]string, 0, 10)
		start = 0
	)

	// appendWord appends the string s[start:end] to the words slice.
	// If the word contains the beginning of a link, the non-link portion
	// of the word and the entire link are appended as separate words,
	// and the start index is advanced to the end of the link.
	appendWord := func(end int) {
		if j := strings.Index(s[start:end], "[["); j > -1 {
			if _, l := parseInlineLink(s[start+j:]); l > 0 {
				// Append portion before link, if any.
				if j > 0 {
					words = append(words, s[start:start+j])
				}
				// Append link itself.
				words = append(words, s[start+j:start+j+l])
				// Advance start index to end of link.
				start = start + j + l
				return
			}
		}
		// No link; just add the word.
		words = append(words, s[start:end])
		start = end
	}

	wasSpace := false
	for i, r := range s {
		isSpace := unicode.IsSpace(r)
		if i > start && isSpace != wasSpace {
			appendWord(i)
		}
		wasSpace = isSpace
	}
	for start < len(s) {
		appendWord(len(s))
	}
	return words
}

// parseInlineLink parses an inline link at the start of s, and returns
// a rendered Markdown link and the total length of the raw inline link.
// If no inline link is present, it returns all zeroes.
func parseInlineLink(s string) (link string, length int) {
	if !strings.HasPrefix(s, "[[") {
		return
	}
	end := strings.Index(s, "]]")
	if end == -1 {
		return
	}
	urlEnd := strings.Index(s, "]")
	rawURL := s[2:urlEnd]
	const badURLChars = `<>"{}|\^[] ` + "`" // per RFC2396 section 2.4.3
	if strings.ContainsAny(rawURL, badURLChars) {
		return
	}
	if urlEnd == end {
		simpleURL := ""
		url, err := url.Parse(rawURL)
		if err == nil {
			// If the URL is http://foo.com, drop the http://
			// In other words, render [[http://golang.org]] as:
			//   <a href="http://golang.org">golang.org</a>
			if after, ok := strings.CutPrefix(rawURL, url.Scheme+"://"); ok {
				simpleURL = after
			} else if after, ok := strings.CutPrefix(rawURL, url.Scheme+":"); ok {
				simpleURL = after
			}
		}
		return renderLink(rawURL, simpleURL), end + 2
	}
	if s[urlEnd:urlEnd+2] != "][" {
		return
	}
	text := s[urlEnd+2 : end]
	return renderLink(rawURL, text), end + 2
}

func renderLink(href, text string) string {
	text = font(text, true)
	if text == "" {
		text = escapeText(href, true)
	}
	// Parentheses in a link destination must be balanced, unless it is
	// in angle brackets.
	if strings.Count(href, "(") != strings.Count(href, ")") {
		href = "<" + href + ">"
	}
	return "[" + text + "](" + href + ")"
}

// entity matches an HTML entity or character reference, which Markdown
// replaces with its character.
var entity = regexp.MustCompile(`^&(#[0-9]{1,7}|#[xX][0-9a-fA-F]{1,6}|[A-Za-z][A-Za-z0-9]{1,31});`)

// escapeText escapes the characters of plain text that Markdown would take
// for inline markup, and with brackets, also '[' and ']'.
func escapeText(s string, brackets bool) string {
	var b strings.Builder
	for i, r := range s {
		esc := false
		switch r {
		case '\\', '`', '*':
			esc = true
		case '_':
			// An underscore inside a word cannot start or end emphasis.
			before, _ := utf8.DecodeLastRuneInString(s[:i])
			after, _ := utf8.DecodeRuneInString(s[i+1:])
			esc = !isAlnum(before) || !isAlnum(after)
		case '<':
			// Only "<" followed by something can start HTML or an autolink.
			esc = i+1 < len(s) && s[i+1] != ' '
		case '&':
			esc = entity.MatchString(s[i:])
		case '[', ']':
			esc = brackets
		}
		if esc {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isAlnum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

var (
	orderedListItem = regexp.MustCompile(`^[0-9]{1,9}[.)]([ \t]|$)`)
	setextUnderline = regexp.MustCompile(`^(=+|-+)[ \t]*$`)
	linkDefinition  = regexp.MustCompile(`^\[[^\]]*\]:`)
	tableDelimiter  = regexp.MustCompile(`^\|?[ \t]*:?-+:?[ \t]*(\|[ \t]*:?-+:?[ \t]*)*\|?[ \t]*$`)
)

// escapeLineStart escapes the start of s, a line of Markdown text, if
// present or Markdown would take it for the start of something other than
// text: a command, comment or speaker note of present, or a Markdown block
// such as a list item, heading or quote.
func escapeLineStart(s string) string {
	switch {
	case strings.HasPrefix(s, "."), strings.HasPrefix(s, "//"):
		return `\` + s
	case strings.HasPrefix(strings.TrimSpace(s), ":::"):
		i := strings.Index(s, ":")
		return s[:i] + `\` + s[i:]
	}
	text := strings.TrimLeft(s, " \t")
	indent := s[:len(s)-len(text)]
	switch {
	case text == "":
	case setextUnderline.MatchString(text),
		text[0] == '>', text[0] == '#', strings.HasPrefix(text, "~~~"),
		linkDefinition.MatchString(text),
		(text[0] == '-' || text[0] == '+') && (len(text) == 1 || text[1] == ' ' || text[1] == '\t'):
		return indent + `\` + text
	case orderedListItem.MatchString(text):
		i := strings.IndexAny(text, ".)")
		return indent + text[:i] + `\` + text[i:]
	case strings.Contains(text, "|") && tableDelimiter.MatchString(text):
		return indent + strings.ReplaceAll(text, "|", `\|`)
	}
	return s
}
