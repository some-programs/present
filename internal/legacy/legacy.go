// Package legacy converts presentations in the legacy present markup, which
// golang.org/x/tools/present also reads, to the Markdown markup that
// present reads.
//
// The conversion keeps how a presentation renders, as far as Markdown can:
//
//   - The title line gets a "# " prefix, and sections, "* Title", become
//     "## Title", with one more '#' for each level of subsection.
//   - Inline markup, *bold*, _italic_, `code` and [[url][text]] links,
//     becomes its Markdown equivalent, also in .caption and .link labels,
//     and other text is escaped where Markdown would take it for markup.
//   - Each line of a paragraph in a slide presentation, which the legacy
//     markup shows on a line of its own, ends in a backslash, Markdown's
//     line break, but for the last. In an article, where the lines of a
//     paragraph are filled, they are left as they are.
//   - Indented preformatted text becomes an indented code block, or a
//     fenced code block right after a list or another code block, which an
//     indented one would join.
//   - Comments, "# comment", become "// comment", above the block they were
//     in, or for comments above the title, below it.
//   - OldURL lines, which present no longer reads, are dropped from the
//     header. The rest of the header and the authors are kept as they are.
//
// Commands are kept as they are.
package legacy

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// IsLegacy reports whether data is in the legacy markup, whose first
// non-empty line, the title, does not begin with "# " as in the Markdown
// markup.
func IsLegacy(data []byte) bool {
	for line := range bytes.Lines(data) {
		line = bytes.TrimRight(line, "\r\n")
		if len(line) > 0 {
			return !bytes.HasPrefix(line, []byte("# "))
		}
	}
	return true
}

// Convert converts data, the presentation file name in the legacy markup,
// to the Markdown markup. A name ending in .article is an article, and any
// other name a slide presentation.
func Convert(name string, data []byte) ([]byte, error) {
	if !IsLegacy(data) {
		return nil, fmt.Errorf("%s: already in the Markdown markup", name)
	}
	var text []string
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSuffix(line, "\n")
		text = append(text, strings.TrimSuffix(line, "\r"))
	}
	c := &converter{
		name:    name,
		article: filepath.Ext(name) == ".article",
		lines:   &lines{text: text},
	}
	if err := c.header(); err != nil {
		return nil, err
	}
	if err := c.sections("*"); err != nil {
		return nil, err
	}
	if len(c.lines.comments) > 0 {
		c.out.WriteByte('\n')
		c.writeComments(len(text))
	}
	return c.out.Bytes(), nil
}

// lines reads the lines of a presentation, and keeps the comments it skips
// until they are written.
type lines struct {
	text     []string
	line     int // index of the next line
	recorded int // the comments before this index are in comments
	comments []comment
}

// A comment is a comment line, in the Markdown markup.
type comment struct {
	line int // index
	text string
}

// next returns the next line that is not a comment, and ok false at the
// end.
func (l *lines) next() (text string, ok bool) {
	for {
		current := l.line
		l.line++
		if current >= len(l.text) {
			return "", false
		}
		text = l.text[current]
		if !strings.HasPrefix(text, "#") {
			return text, true
		}
		if current >= l.recorded {
			l.comments = append(l.comments, comment{current, "//" + text[1:]})
			l.recorded = current + 1
		}
	}
}

// back steps back to the line that next returned last.
func (l *lines) back() {
	l.line--
}

// nextNonEmpty is like next, but skips empty lines.
func (l *lines) nextNonEmpty() (text string, ok bool) {
	for {
		text, ok = l.next()
		if !ok || len(text) > 0 {
			return
		}
	}
}

// last returns the index of the line that next returned last.
func (l *lines) last() int {
	return l.line - 1
}

// A kind is a kind of block of Markdown.
type kind int

const (
	none    kind = iota // nothing yet in the section
	text                // a paragraph
	list                // a list
	pre                 // a code block
	command             // a command or speaker note, on a line of its own
)

type converter struct {
	name    string
	article bool
	lines   *lines
	out     bytes.Buffer
	prev    kind   // the last block written in the current section
	prevEnd int    // the index of the last line of prev
	bullet  string // the bullet of the last list
}

// header writes the title, the rest of the header and the authors, which
// both markups show as they are, up to the first section.
func (c *converter) header() error {
	title, ok := c.lines.nextNonEmpty()
	if !ok {
		return fmt.Errorf("%s: unexpected EOF; expected title", c.name)
	}
	c.writeLine("# " + title)
	// The comments above the title go below it, as present requires the
	// title to be the first line.
	c.writeComments(c.lines.line)
	inHeader := true
	for {
		line, ok := c.lines.next()
		if !ok {
			return fmt.Errorf("%s: unexpected EOF; expected a section", c.name)
		}
		if strings.HasPrefix(line, "*") {
			c.lines.back()
			break
		}
		c.writeComments(c.lines.last())
		switch {
		case line == "":
			inHeader = false
		case inHeader && strings.HasPrefix(line, "OldURL:"):
			continue
		case isNote(line):
			line = noteLine(line)
		}
		c.writeLine(line)
	}
	trimmed := bytes.TrimRight(c.out.Bytes(), "\n")
	c.out.Truncate(len(trimmed) + 1)
	return nil
}

var isHeading = regexp.MustCompile(`^\*+( |$)`)

// lesserHeading reports whether text is a heading of a lesser or equal
// level than that denoted by prefix.
func lesserHeading(text, prefix string) bool {
	return isHeading.MatchString(text) && !strings.HasPrefix(text, prefix+"*")
}

// sections writes the sections, and their subsections, of the level that
// prefix denotes.
func (c *converter) sections(prefix string) error {
	n := 0
	for {
		line, ok := c.lines.nextNonEmpty()
		if !ok {
			break
		}
		if line != prefix && !strings.HasPrefix(line, prefix+" ") {
			c.lines.back()
			break
		}
		n++
		c.begin(none, c.lines.last(), c.lines.last())
		heading := strings.Repeat("#", len(prefix)+1)
		if title := strings.TrimSpace(line[len(prefix):]); title != "" {
			heading += " " + title
		}
		c.writeLine(heading)

		line, ok = c.lines.nextNonEmpty()
		for ok && !lesserHeading(line, prefix) {
			r, _ := utf8.DecodeRuneInString(line)
			switch {
			case unicode.IsSpace(r):
				c.pre(line)
			case strings.HasPrefix(line, "- "):
				c.list(line)
			case isNote(line):
				c.begin(command, c.lines.last(), c.lines.last())
				c.writeLine(noteLine(line))
			case strings.HasPrefix(line, prefix+"* ") || line == prefix+"*":
				c.lines.back()
				if err := c.sections(prefix + "*"); err != nil {
					return err
				}
			case strings.HasPrefix(line, prefix+"*"):
				return fmt.Errorf("%s:%d: badly nested section inside %s: %s", c.name, c.lines.line, prefix, line)
			case strings.HasPrefix(line, "."):
				c.begin(command, c.lines.last(), c.lines.last())
				c.writeLine(convertCommand(line))
			default:
				c.text(line)
			}
			line, ok = c.lines.nextNonEmpty()
		}
		if isHeading.MatchString(line) {
			c.lines.back()
		}
	}
	if n == 0 {
		return fmt.Errorf("%s:%d: unexpected line: %s", c.name, c.lines.line+1, c.lines.text[c.lines.line])
	}
	return nil
}

// pre writes the preformatted text that starts with first, an indented
// line.
func (c *converter) pre(first string) {
	start := c.lines.last()
	i := strings.IndexFunc(first, func(r rune) bool { return !unicode.IsSpace(r) })
	if i < 0 {
		return
	}
	indent := first[:i]
	var block []string
	end := start
	line, ok := first, true
	for ok && (strings.HasPrefix(line, indent) || line == "") {
		if line != "" {
			line = line[i:]
			end = c.lines.last()
		}
		block = append(block, line)
		line, ok = c.lines.next()
	}
	c.lines.back()
	for block[len(block)-1] == "" {
		block = block[:len(block)-1]
	}

	// In an indented code block, no line starts with something that
	// present takes for a command, comment or speaker note. But one right
	// after a list or another code block would join it, so it is fenced
	// instead, indented by a space to keep that property.
	fenced := c.prev == list || c.prev == pre
	c.begin(pre, start, end)
	if !fenced {
		for _, line := range block {
			if line != "" {
				line = "\t" + line
			}
			c.writeLine(line)
		}
		return
	}
	n := 2
	for _, line := range block {
		line = strings.TrimLeft(line, " \t")
		n = max(n, len(line)-len(strings.TrimLeft(line, "`")))
	}
	fence := " " + strings.Repeat("`", n+1)
	c.writeLine(fence)
	for _, line := range block {
		if line != "" {
			// As in an indented code block, a leading tab is four spaces.
			text := strings.TrimLeft(line, "\t")
			line = " " + strings.Repeat("    ", len(line)-len(text)) + text
		}
		c.writeLine(line)
	}
	c.writeLine(fence)
}

// list writes the list that starts with first, a line starting with "- ".
func (c *converter) list(first string) {
	start := c.lines.last()
	var items [][]string
	end := start
	line, ok := first, true
	for {
		if rest, found := strings.CutPrefix(line, "- "); found {
			items = append(items, []string{strings.TrimSpace(rest)})
		} else if len(items) > 0 && strings.HasPrefix(line, " ") {
			if line = strings.TrimSpace(line); line != "" {
				items[len(items)-1] = append(items[len(items)-1], line)
			}
		} else {
			break
		}
		end = c.lines.last()
		if line, ok = c.lines.next(); !ok {
			break
		}
	}
	c.lines.back()

	// A list right after another list would join it, unless their bullets
	// differ.
	bullet := "-"
	if c.prev == list && c.bullet == "-" {
		bullet = "+"
	}
	c.bullet = bullet
	c.begin(list, start, end)
	for _, item := range items {
		for i, line := range item {
			prefix := "  "
			if i == 0 {
				prefix = bullet + " "
			}
			c.writeLine(strings.TrimRight(prefix+styleLine(line), " "))
		}
	}
}

// text writes the paragraph that starts with first.
func (c *converter) text(first string) {
	start := c.lines.last()
	var block []string
	end := start
	line, ok := first, true
	for ok && strings.TrimSpace(line) != "" {
		if line[0] == '.' || isNote(line) {
			c.lines.back()
			break
		}
		if strings.HasPrefix(line, `\.`) { // Backslash escapes initial period.
			line = line[1:]
		}
		block = append(block, line)
		end = c.lines.last()
		line, ok = c.lines.next()
	}

	c.begin(text, start, end)
	for i, line := range block {
		line = styleLine(strings.TrimRightFunc(line, unicode.IsSpace))
		if !c.article && i < len(block)-1 {
			line += `\`
		}
		c.writeLine(line)
	}
}

// begin begins a block of kind k, whose lines are from the index start to
// end: it writes the blank line that separates it from the block before,
// and the comments before end.
func (c *converter) begin(k kind, start, end int) {
	// Consecutive commands and notes are kept together, unless there was a
	// blank line between them.
	together := k == command && c.prev == command
	for i := c.prevEnd + 1; together && i < start; i++ {
		together = strings.HasPrefix(c.lines.text[i], "#")
	}
	if !together {
		c.out.WriteByte('\n')
	}
	c.writeComments(end)
	c.prev, c.prevEnd = k, end
}

// writeComments writes the comments before the index end.
func (c *converter) writeComments(end int) {
	n := 0
	for _, cm := range c.lines.comments {
		if cm.line >= end {
			break
		}
		c.writeLine(cm.text)
		n++
	}
	c.lines.comments = c.lines.comments[n:]
}

func (c *converter) writeLine(s string) {
	c.out.WriteString(s)
	c.out.WriteByte('\n')
}

// convertCommand converts the inline markup of the text of .caption and the
// label of .link, which present renders as Markdown.
func convertCommand(line string) string {
	args := strings.Fields(line)
	switch {
	case args[0] == ".caption":
		if text := strings.TrimSpace(strings.TrimPrefix(line, ".caption")); text != "" {
			return ".caption " + styleLine(text)
		}
	case args[0] == ".link" && len(args) > 2:
		return ".link " + args[1] + " " + styleLine(strings.Join(args[2:], " "))
	}
	return line
}

func isNote(s string) bool {
	return strings.HasPrefix(s, ": ") || s == ":"
}

// noteLine converts the speaker note s, which the legacy markup shows as it
// is and present renders as Markdown.
func noteLine(s string) string {
	if s == ":" {
		return s
	}
	text := strings.TrimPrefix(s, ": ")
	return ": " + escapeLineStart(escapeText(text, strings.Contains(text, "](")))
}
