// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"bufio"
	"bytes"
	"fmt"
	"html/template"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// PlayEnabled specifies whether runnable playground snippets should be
// displayed in the present user interface.
var PlayEnabled = false

// TODO(adg): replace the PlayEnabled flag with something less spaghetti-like.
// Instead this will probably be determined by a template execution Context
// value that contains various global metadata required when rendering
// templates.

// NotesEnabled specifies whether presenter notes should be displayed in the
// present user interface.
var NotesEnabled = false

// PreloadEnabled specifies whether media (images and videos) should be preloaded.
var PreloadEnabled = true

func init() {
	Register("code", parseCode)
	Register("play", parseCode)
}

type Code struct {
	Cmd      string // original command from present source
	Text     template.HTML
	Play     bool   // runnable code
	Edit     bool   // editable code
	FileName string // file name
	Ext      string // file extension
	Raw      []byte // content of the file
}

func (c Code) TemplateName() string { return "code" }

// The input line is a .code or .play entry with a file name and an optional HLfoo marker on the end.
// Anything between the file and HL (if any) is an address expression, which we treat as a string here.
// We pick off the HL first, for easy parsing.
var (
	highlightRE = regexp.MustCompile(`\s+HL([a-zA-Z0-9_]+)?$`)
	hlCommentRE = regexp.MustCompile(`(.+) // HL(.*)$`)
	codeRE      = regexp.MustCompile(`\.(code|play)\s+((?:(?:-edit|-numbers)\s+)*)([^\s]+)(?:\s+(.*))?$`)
)

// parseCode parses a code present directive. Its syntax:
//
//	.code [-numbers] [-edit] <filename> [address] [highlight]
//
// The directive may also be ".play" if the snippet is executable.
func parseCode(ctx *Context, sourceFile string, sourceLine int, cmd string) (Elem, error) {
	cmd = strings.TrimSpace(cmd)
	origCmd := cmd

	// Pull off the HL, if any, from the end of the input line.
	highlight := ""
	if hl := highlightRE.FindStringSubmatchIndex(cmd); len(hl) == 4 {
		if hl[2] < 0 || hl[3] < 0 {
			return nil, fmt.Errorf("%s:%d invalid highlight syntax", sourceFile, sourceLine)
		}
		highlight = cmd[hl[2]:hl[3]]
		cmd = cmd[:hl[2]-2]
	}

	// Parse the remaining command line.
	// Arguments:
	// args[0]: whole match
	// args[1]:  .code/.play
	// args[2]: flags ("-edit -numbers")
	// args[3]: file name
	// args[4]: optional address
	args := codeRE.FindStringSubmatch(cmd)
	if len(args) != 5 {
		return nil, fmt.Errorf("%s:%d: syntax error for .code/.play invocation", sourceFile, sourceLine)
	}
	command, flags, file, addr := args[1], args[2], args[3], strings.TrimSpace(args[4])
	play := command == "play" && PlayEnabled

	// Read in code file and (optionally) match address.
	filename := filepath.Join(filepath.Dir(sourceFile), file)
	textBytes, err := ctx.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("%s:%d: %v", sourceFile, sourceLine, err)
	}
	lo, hi, err := addrToByteRange(addr, 0, textBytes)
	if err != nil {
		return nil, fmt.Errorf("%s:%d: %v", sourceFile, sourceLine, err)
	}
	if lo > hi {
		// The search in addrToByteRange can wrap around so we might
		// end up with the range ending before its starting point
		hi, lo = lo, hi
	}

	// Acme pattern matches can stop mid-line,
	// so run to end of line in both directions if not at line start/end.
	for lo > 0 && textBytes[lo-1] != '\n' {
		lo--
	}
	if hi > 0 {
		for hi < len(textBytes) && textBytes[hi-1] != '\n' {
			hi++
		}
	}

	lines := codeLines(textBytes, lo, hi)

	data := &codeTemplateData{
		Lines:   formatLines(lines, highlight),
		Edit:    strings.Contains(flags, "-edit"),
		Numbers: strings.Contains(flags, "-numbers"),
	}

	// Include before and after in a hidden span for playground code.
	if play {
		data.Prefix = textBytes[:lo]
		data.Suffix = textBytes[hi:]
	}

	var buf bytes.Buffer
	if err := codeTemplate.Execute(&buf, data); err != nil {
		return nil, err
	}
	return Code{
		Cmd:      origCmd,
		Text:     template.HTML(buf.String()),
		Play:     play,
		Edit:     data.Edit,
		FileName: filepath.Base(filename),
		Ext:      filepath.Ext(filename),
		Raw:      rawCode(lines),
	}, nil
}

// formatLines returns a new slice of codeLine with the given lines
// replacing tabs with spaces and adding highlighting where needed.
func formatLines(lines []codeLine, highlight string) []codeLine {
	formatted := make([]codeLine, len(lines))
	for i, line := range lines {
		// Replace tabs with spaces, which work better in HTML.
		line.L = strings.ReplaceAll(line.L, "\t", "    ")

		// Highlight lines that end with "// HL[highlight]"
		// and strip the magic comment.
		if m := hlCommentRE.FindStringSubmatch(line.L); m != nil {
			line.L = m[1]
			line.HL = m[2] == highlight
		}

		formatted[i] = line
	}
	return formatted
}

// rawCode returns the code represented by the given codeLines without any kind
// of formatting.
func rawCode(lines []codeLine) []byte {
	b := new(bytes.Buffer)
	for _, line := range lines {
		b.WriteString(line.L)
		b.WriteByte('\n')
	}
	return b.Bytes()
}

type codeTemplateData struct {
	Lines          []codeLine
	Prefix, Suffix []byte
	Edit, Numbers  bool
}

var leadingSpaceRE = regexp.MustCompile(`^[ \t]*`)

var codeTemplate = template.Must(template.New("code").Funcs(template.FuncMap{
	"trimSpace":    strings.TrimSpace,
	"leadingSpace": leadingSpaceRE.FindString,
}).Parse(codeTemplateHTML))

// codeTemplateHTML puts no text between the pre elements: the playground
// script reads the program back from the DOM, and stray newlines there
// would end up in it.
const codeTemplateHTML = `
{{- with .Prefix}}<pre style="display: none"><span>{{printf "%s" .}}</span></pre>{{end -}}

<pre{{if .Edit}} contenteditable="true" spellcheck="false"{{end}}{{if .Numbers}} class="numbers"{{end}}>
	{{- range .Lines}}<span num="{{.N}}">
	{{- if .HL}}{{leadingSpace .L}}<b>{{trimSpace .L}}</b>
	{{- else}}{{.L}}{{end -}}
</span>
{{end}}</pre>
{{- with .Suffix}}<pre style="display: none"><span>{{printf "%s" .}}</span></pre>{{end -}}
`

// codeLine represents a line of code extracted from a source file.
type codeLine struct {
	L  string // The line of code.
	N  int    // The line number from the source file.
	HL bool   // Whether the line should be highlighted.
}

// codeLines takes a source file and returns the lines that
// span the byte range specified by start and end.
// It discards lines that end in "OMIT".
func codeLines(src []byte, start, end int) (lines []codeLine) {
	startLine := bytes.Count(src[:start], []byte("\n")) + 1
	s := bufio.NewScanner(bytes.NewReader(src[start:end]))
	for n := startLine; s.Scan(); n++ {
		l := s.Text()
		if strings.HasSuffix(l, "OMIT") {
			continue
		}
		lines = append(lines, codeLine{L: l, N: n})
	}
	// Trim leading and trailing blank lines.
	for len(lines) > 0 && len(lines[0].L) == 0 {
		lines = lines[1:]
	}
	for len(lines) > 0 && len(lines[len(lines)-1].L) == 0 {
		lines = lines[:len(lines)-1]
	}
	return
}

// parseDims parses the height and width arguments of an .iframe or .video
// command, each a number of pixels or _ for none. With no height or width,
// the template omits that attribute and the browser keeps the aspect ratio.
func parseDims(height, width string) (h, w int, err error) {
	if h, err = parsePixels(height); err != nil {
		return 0, 0, err
	}
	if w, err = parsePixels(width); err != nil {
		return 0, 0, err
	}
	return h, w, nil
}

// parsePixels parses a number of pixels, or _ for none, which is 0.
func parsePixels(s string) (int, error) {
	if s == "_" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("bad size %q", s)
	}
	return n, nil
}
