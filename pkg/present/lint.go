// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/yuin/goldmark/v2/ast"
)

// A Problem is a problem that Lint finds in a present file.
type Problem struct {
	File     string   `json:"file"`
	Line     int      `json:"line"` // from 1
	Rule     string   `json:"rule"` // such as "missing-file"
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

// String returns the problem as file:line: message (rule).
func (p Problem) String() string {
	return fmt.Sprintf("%s:%d: %s (%s)", p.File, p.Line, p.Message, p.Rule)
}

// A Severity tells how bad a Problem is.
type Severity string

const (
	// An Error keeps the file from presenting, or makes it present
	// something else than what is written.
	Error Severity = "error"
	// A Warning is likely a mistake, or makes the file harder to read.
	Warning Severity = "warning"
)

// Lint checks name, a present file with the contents src, and returns the
// problems it finds, by line. It reads the files that commands use, from
// the directory of name, and reports all problems, but for one that keeps
// the file from being read, such as a section nested too deep, which ends
// the checks.
//
// The errors are:
//
//   - syntax: the file cannot be read, as a slide presentation or article.
//   - unknown-command: a line starts with a period, but is not a command.
//   - command: a command has the wrong arguments.
//   - missing-file: a command uses a file that does not exist.
//   - code-address: the address of a .code or .play command matches no
//     lines of its file.
//   - duplicate-id: two headings have the same {#id}.
//   - parse: anything else that keeps the file from presenting.
//
// The warnings are:
//
//   - not-formatted: present fmt would change the file.
//   - heading-space: a line starts with #, but is not a heading, since no
//     space follows the #s.
//   - note-space: a line starts with : and a letter, but is not a note,
//     since no space follows the colon.
//   - notes-block: a line starts with :::, but does not start or end a
//     block of notes.
//   - header-date: the subtitle is a date, but not in a form that present
//     reads as one, such as 2026-10-09 or October 9, 2026.
//   - highlight: the HL word of a .code or .play command marks no line of
//     its file.
//   - play-language: .play would run a file as Go, but it is not one.
//   - empty-link: a link has no URL.
func Lint(name string, src []byte) []Problem {
	l := &linter{name: name}
	l.lint(src)
	slices.SortStableFunc(l.problems, func(a, b Problem) int { return cmp.Compare(a.Line, b.Line) })
	return l.problems
}

type linter struct {
	name     string
	problems []Problem
}

func (l *linter) report(line int, rule string, severity Severity, format string, args ...any) {
	l.problems = append(l.problems, Problem{
		File:     l.name,
		Line:     line,
		Rule:     rule,
		Severity: severity,
		Message:  fmt.Sprintf(format, args...),
	})
}

// reportErr reports err, an error from the parser, which starts with the
// name of the file and a line number, at that line, or at line otherwise.
func (l *linter) reportErr(line int, rule string, severity Severity, err error) {
	msg := err.Error()
	if rest, ok := strings.CutPrefix(msg, l.name+":"); ok {
		if num, rest, ok := strings.Cut(rest, ": "); ok {
			if n, err := strconv.Atoi(num); err == nil {
				line, msg = n, rest
			}
		}
	}
	l.report(line, rule, severity, "%s", msg)
}

func (l *linter) lint(src []byte) {
	f, err := parseFile(bytes.NewReader(src), l.name, 0)
	if err != nil {
		l.reportErr(1, "syntax", Error, err)
		return
	}
	l.format(src)

	ids := make(map[string]int)
	for i, line := range f.lines {
		n := i + 1
		switch kind := f.kinds[i]; {
		case kind == CommandLine:
			l.command(n, line)
		case kind == HeadingLine:
			level := len(line) - len(strings.TrimLeft(line, "#"))
			if _, id := splitHeading(line[level:]); id != "" {
				if first, ok := ids[id]; ok {
					l.report(n, "duplicate-id", Error, "the heading on line %d has the ID %q too", first, id)
				} else {
					ids[id] = n
				}
			}
		case kind == HeaderLine:
			l.header(n, line)
		case kind == TextLine && !f.verbatim[i]:
			l.text(n, line)
		}
	}
	l.markdown(f.sections)

	// Anything else that keeps the file from presenting, unless it was
	// reported already.
	if _, err := Parse(bytes.NewReader(src), l.name, 0); err != nil {
		before := len(l.problems)
		l.reportErr(1, "parse", Error, err)
		p := l.problems[before]
		if slices.ContainsFunc(l.problems[:before], func(q Problem) bool { return q.Line == p.Line && q.Severity == Error }) {
			l.problems = l.problems[:before]
		}
	}
}

// format reports where present fmt would change the file first.
func (l *linter) format(src []byte) {
	formatted, err := Format(l.name, src)
	if err != nil {
		l.reportErr(1, "not-formatted", Warning, err)
		return
	}
	if bytes.Equal(formatted, src) {
		return
	}
	a := strings.Split(string(src), "\n")
	b := strings.Split(string(formatted), "\n")
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	l.report(min(n+1, len(a)), "not-formatted", Warning, "the file is not formatted; run present fmt")
}

// dateLayouts are the layouts of dates that present does not read as
// one, but people write.
var dateLayouts = []string{
	"2006-01-02",
	"January 2, 2006",
	"January 2 2006",
	"Jan 2, 2006",
	"Jan 2 2006",
	"2 January 2006",
	"Monday, January 2, 2006",
	"Mon, Jan 2, 2006",
	"Monday, 2 January 2006",
	"Mon, 2 Jan 2006",
}

// header checks a line of the header after the title.
func (l *linter) header(n int, line string) {
	line = strings.TrimRight(line, " \t")
	if strings.HasPrefix(line, "Tags:") || strings.HasPrefix(line, "Summary:") {
		return
	}
	if _, ok := parseTime(line); ok {
		return
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, line); err == nil {
			l.report(n, "header-date", Warning, "%q is the subtitle, not the date, which present reads only as 2 Jan 2006 or 15:04 2 Jan 2006: write %s", line, t.Format("2 Jan 2006"))
			return
		}
	}
}

var (
	headingSpaceRE = regexp.MustCompile(`^#+[^#\s]`)
	noteSpaceRE    = regexp.MustCompile(`^:\pL`)
)

// text checks a line of Markdown text, outside code blocks.
func (l *linter) text(n int, line string) {
	switch {
	case headingSpaceRE.MatchString(line):
		hashes := line[:len(line)-len(strings.TrimLeft(line, "#"))]
		l.report(n, "heading-space", Warning, "the line is text, not a heading; a heading needs a space after %s", hashes)
	case strings.HasPrefix(strings.TrimSpace(line), ":::"):
		l.report(n, "notes-block", Warning, "the line is text: a block of notes starts with a \"::: notes\" line, and ends with a \":::\" line")
	case noteSpaceRE.MatchString(line):
		l.report(n, "note-space", Warning, "the line is text, not a note; a note needs a space after the colon")
	}
}

// markdown checks the Markdown of b, the sections.
func (l *linter) markdown(b *sections) {
	ast.Walk(b.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if n, ok := n.(*ast.Link); ok && n.Destination.Value(b.src) == "" {
			l.report(b.line(textPos(n))+1, "empty-link", Warning, "the link has no URL")
		}
		return ast.WalkContinue, nil
	})
}

// textPos returns the offset of the first text in n, or 0 if it has none.
func textPos(n ast.Node) int {
	pos := 0
	ast.Walk(n, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := n.(*ast.Text); ok && entering && t.Pos() >= 0 {
			pos = t.Pos()
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return pos
}

// command checks the command on line n.
func (l *linter) command(n int, line string) {
	args := strings.Fields(line)
	cmd := args[0]
	if cmd == ".background" {
		if len(args) != 2 {
			l.report(n, "command", Error, ".background expects exactly one argument")
			return
		}
		l.file(n, args[1])
		return
	}
	parse := parsers[cmd]
	if parse == nil {
		msg := fmt.Sprintf("unknown command %q", cmd)
		if cmd == "." || !strings.ContainsAny(cmd[1:2], "abcdefghijklmnopqrstuvwxyz") {
			msg += `; a backslash before the period, \., makes the line text`
		}
		l.report(n, "unknown-command", Error, "%s", msg)
		return
	}
	switch cmd {
	case ".code", ".play":
		l.code(n, line)
		return
	case ".html":
		if len(args) != 2 {
			l.report(n, "command", Error, ".html expects exactly one argument")
			return
		}
		l.file(n, args[1])
		return
	}
	ctx := &Context{ReadFile: os.ReadFile}
	e, err := parse(ctx, l.name, n, line)
	if err != nil {
		l.reportErr(n, "command", Error, err)
		return
	}
	switch e := e.(type) {
	case Image:
		l.file(n, cmp.Or(e.Source, e.URL))
	case Video:
		l.file(n, e.URL)
	case Iframe:
		l.file(n, e.URL)
	}
}

// file reports a missing file at ref, a file name or URL in a command on
// line n, if it is a file, relative to the presentation, that does not
// exist. It reports whether the file exists, or ref is not a file.
func (l *linter) file(n int, ref string) bool {
	u, err := url.Parse(ref)
	if err != nil || u.Scheme != "" || u.Host != "" || path.IsAbs(u.Path) {
		return true
	}
	_, err = os.Stat(filepath.Join(filepath.Dir(l.name), filepath.FromSlash(u.Path)))
	if errors.Is(err, fs.ErrNotExist) {
		l.report(n, "missing-file", Error, "%s does not exist", u.Path)
		return false
	}
	return true
}

// playExts are the extensions of the files that .play runs other than as
// Go: by the interpreters of package socket, or as a txtar archive.
var playExts = []string{".go", ".txtar", ".py", ".js", ".ts"}

// code checks the .code or .play command on line n.
func (l *linter) code(n int, line string) {
	cmd := strings.TrimSpace(line)
	highlight := ""
	if hl := highlightRE.FindStringSubmatchIndex(cmd); len(hl) == 4 {
		if hl[2] < 0 {
			l.report(n, "command", Error, "invalid highlight syntax")
			return
		}
		highlight = cmd[hl[2]:hl[3]]
		cmd = cmd[:hl[2]-2]
	}
	args := codeRE.FindStringSubmatch(cmd)
	if len(args) != 5 {
		l.report(n, "command", Error, "syntax error for .code/.play invocation; want %s [-numbers] [-edit] file [address] [HLword]", strings.Fields(cmd)[0])
		return
	}
	file := args[3]
	if !l.file(n, file) {
		return
	}
	ctx := &Context{ReadFile: os.ReadFile}
	if _, err := parseCode(ctx, l.name, n, line); err != nil {
		l.reportErr(n, "code-address", Error, err)
		return
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(l.name), file))
	if err != nil {
		return // reported by parseCode
	}

	// Lines that a word highlights may not be among those shown, as
	// when the commands of a slide show parts of a file with the same
	// word. Then the word marks lines elsewhere.
	if highlight != "" && !slices.ContainsFunc(strings.Split(string(data), "\n"), func(line string) bool {
		m := hlCommentRE.FindStringSubmatch(strings.TrimSuffix(line, "\r"))
		return m != nil && m[2] == highlight
	}) {
		l.report(n, "highlight", Warning, "HL%s marks no line: no line of %s ends in // HL%s", highlight, file, highlight)
	}

	if args[1] == "play" && !slices.Contains(playExts, filepath.Ext(file)) && !bytes.HasPrefix(data, []byte("#!")) {
		l.report(n, "play-language", Warning, "%s is run as Go: .play runs %s files as their language, and others as Go unless they start with #!", file, strings.Join(playExts, ", "))
	}
}
