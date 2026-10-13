// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLint(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{
		"main.go":    "package main\n\n// START OMIT\nfunc main() {\n\tprintln() // HLmain\n}\n// START OMIT\n",
		"script.rb":  "puts 1\n",
		"script.sh":  "#!/bin/sh\necho 1\n",
		"image.png":  "",
		"frame.html": "<p>frame</p>\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const src = `# Title
October 9, 2026

## Commands {#a}

.imgae image.png
...and so on
.image image.png 2 3 4
.image missing.png
.image image.png
.code main.go /^func nothing/
.code main.go /^func main/,/^}/ HLother
.code main.go /^package/ HLmain
.play script.rb
.play script.sh
.html frame.html
.iframe https://example.com 300 400
.background missing.jpg

## Text {#a}

#hashtag
:note
::: note
See https://example.com, <https://example.com> and [the site](https://example.com).
An [empty link]() and ` + "`https://example.com`" + `.

` + "```" + `
code
` + "```" + `

` + "```go" + `
#hashtag
` + "```" + `

:-(
`
	name := filepath.Join(dir, "x.slide")
	want := []string{
		"2: header-date: write 9 Oct 2026",
		"6: unknown-command: unknown command \".imgae\"",
		"7: unknown-command: a backslash before the period",
		"8: command: too many arguments",
		"9: missing-file: missing.png does not exist",
		"11: code-address",
		"12: highlight: HLother marks no line: no line of main.go",
		"14: play-language: script.rb is run as Go",
		"18: missing-file: missing.jpg does not exist",
		"20: duplicate-id: the heading on line 4",
		"22: heading-space",
		"23: note-space",
		"24: notes-block",
		"26: empty-link",
	}
	var got []string
	for _, p := range Lint(name, []byte(src)) {
		if p.File != name {
			t.Errorf("problem in file %q, want %q", p.File, name)
		}
		got = append(got, p.String())
	}
	for _, w := range want {
		line, rest, _ := strings.Cut(w, ": ")
		rule, msg, _ := strings.Cut(rest, ": ")
		i := slices.IndexFunc(got, func(g string) bool {
			return strings.HasPrefix(g, name+":"+line+": ") && strings.HasSuffix(g, "("+rule+")") && strings.Contains(g, msg)
		})
		if i < 0 {
			t.Errorf("missing problem %s", w)
			continue
		}
		got = slices.Delete(got, i, i+1)
	}
	for _, g := range got {
		t.Errorf("unexpected problem %s", g)
	}
}

func TestLintSyntax(t *testing.T) {
	problems := Lint("x.slide", []byte("# Title\n\n## One\n\n#### Deep\n"))
	if len(problems) != 1 || problems[0].Rule != "syntax" || problems[0].Line != 5 || problems[0].Severity != Error {
		t.Errorf("got %v, want one syntax error on line 5", problems)
	}
}

func TestLintNotFormatted(t *testing.T) {
	problems := Lint("x.slide", []byte("# Title\n\n## One\nText.  \n"))
	if len(problems) != 1 || problems[0].Rule != "not-formatted" || problems[0].Line != 4 || problems[0].Severity != Warning {
		t.Errorf("got %v, want not-formatted on line 4", problems)
	}
}

func TestLintExamples(t *testing.T) {
	for _, pattern := range []string{"../../examples/*.slide", "../../examples/*.article"} {
		files, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range Lint(file, data) {
				t.Errorf("%v", p)
			}
		}
	}
}

func TestLintHeaderDate(t *testing.T) {
	for _, tt := range []struct {
		subtitle, want string // want is the date to write, if any
	}{
		{"2026-10-09", "9 Oct 2026"},
		{"October 9, 2026", "9 Oct 2026"},
		{"Oct 9 2026", "9 Oct 2026"},
		{"9 October 2026", "9 Oct 2026"},
		{"Friday, October 9, 2026", "9 Oct 2026"},
		{"GopherCon, Oct 9, 2026", ""},
		{"Where we are in May 2015", ""},
		{"October 2014", ""},
		{"Subtitle", ""},
	} {
		src := "# Title\n" + tt.subtitle + "\n\n## Slide\n"
		var got []Problem
		for _, p := range Lint("x.slide", []byte(src)) {
			if p.Rule == "header-date" {
				got = append(got, p)
			}
		}
		switch {
		case tt.want == "" && len(got) > 0:
			t.Errorf("subtitle %q: got %v, want no header-date", tt.subtitle, got)
		case tt.want != "" && (len(got) != 1 || !strings.HasSuffix(got[0].Message, "write "+tt.want)):
			t.Errorf("subtitle %q: got %v, want header-date to write %s", tt.subtitle, got, tt.want)
		}
	}
}
