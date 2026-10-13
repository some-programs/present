// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package present

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestBackgroundErrors(t *testing.T) {
	const input = `# Title

## Slide

%s
`
	const want = "test.slide:5: .background expects exactly one argument"

	for _, directive := range []string{
		".background",
		".background a.png b.png",
	} {
		t.Run(directive, func(t *testing.T) {
			_, err := Parse(strings.NewReader(fmt.Sprintf(input, directive)), "test.slide", 0)
			if err == nil {
				t.Fatalf("Parse did not return an error")
			}
			if got := err.Error(); got != want {
				t.Errorf("Parse error = %q, want %q", got, want)
			}
		})
	}
}

func TestFontSizeErrors(t *testing.T) {
	for _, tt := range []struct {
		line, want string
	}{
		{".fontsize big", `test.slide:5: invalid .fontsize size "big": want a factor such as 0.8, a percentage, or a size in px or pt`},
		{".fontsize 2em", `test.slide:5: invalid .fontsize size "2em": want a factor such as 0.8, a percentage, or a size in px or pt`},
		{".fontsize 1 2", "test.slide:5: .fontsize takes at most one size"},
	} {
		t.Run(tt.line, func(t *testing.T) {
			input := "# Title\n\n## Slide\n\n" + tt.line + "\n"
			_, err := Parse(strings.NewReader(input), "test.slide", 0)
			if err == nil {
				t.Fatalf("Parse did not return an error")
			}
			if got := err.Error(); got != tt.want {
				t.Errorf("Parse error = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLegacyRejected(t *testing.T) {
	const input = `
Title

* Slide
`
	const want = `test.slide:2: title must begin with "# " (legacy present markup is not supported; convert it with present convert-legacy)`
	_, err := Parse(strings.NewReader(input), "test.slide", TitlesOnly)
	if err == nil {
		t.Fatalf("Parse did not return an error")
	}
	if got := err.Error(); got != want {
		t.Errorf("Parse error = %q, want %q", got, want)
	}
}

func TestTestdata(t *testing.T) {
	tmpl := template.Must(Template().Parse(testTmpl))
	files, err := filepath.Glob("testdata/*.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		name := filepath.Base(file)
		if name == "README" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("%s: %v", file, err)
			}
			marker := []byte("\n---\n")
			i := bytes.Index(data, marker)
			if i < 0 {
				t.Fatalf("%s: cannot find --- marker in input", file)
			}
			input, html := data[:i+1], data[i+len(marker):]
			doc, err := Parse(bytes.NewReader(input), name, 0)
			if err != nil {
				t.Fatalf("%s: %v", file, err)
			}
			var buf bytes.Buffer
			if err := doc.Render(&buf, tmpl); err != nil {
				t.Fatalf("%s: %v", file, err)
			}
			if !bytes.Equal(buf.Bytes(), html) {
				diffText, err := diff("present-test-", "want", html, "have", buf.Bytes())
				if err != nil {
					t.Fatalf("%s: diff: %v", file, err)
				}
				t.Errorf("%s: incorrect html:\n%s", file, diffText)
			}
		})
	}
}

func diff(prefix string, name1 string, b1 []byte, name2 string, b2 []byte) ([]byte, error) {
	f1, err := writeTempFile(prefix, b1)
	if err != nil {
		return nil, err
	}
	defer os.Remove(f1)

	f2, err := writeTempFile(prefix, b2)
	if err != nil {
		return nil, err
	}
	defer os.Remove(f2)

	cmd := "diff"
	if runtime.GOOS == "plan9" {
		cmd = "/bin/ape/diff"
	}

	data, err := exec.Command(cmd, "-u", f1, f2).Output()
	if len(data) > 0 {
		// diff exits with a non-zero status when the files don't match.
		// Ignore that failure as long as we get output.
		err = nil
	} else if exit, ok := err.(*exec.ExitError); ok && len(exit.Stderr) > 0 {
		err = fmt.Errorf("%w\nstderr:\n%s)", err, exit.Stderr)
	}

	data = bytes.Replace(data, []byte(f1), []byte(name1), -1)
	data = bytes.Replace(data, []byte(f2), []byte(name2), -1)

	return data, err
}

func writeTempFile(prefix string, data []byte) (string, error) {
	file, err := os.CreateTemp("", prefix)
	if err != nil {
		return "", err
	}
	_, err = file.Write(data)
	if err1 := file.Close(); err == nil {
		err = err1
	}
	if err != nil {
		os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

var testTmpl = `
{{define "root" -}}
<h1>{{.Title}}</h1>
{{with .Subtitle}}<h2>{{.}}</h2>
{{end -}}
{{range .Authors}}<author>
{{range .Elem}}{{elem $.Template .}}{{end}}</author>
{{end -}}
{{range .Sections}}<section>{{elem $.Template .}}</section>
{{end -}}
{{end}}

{{define "newline"}}{{/* No automatic line break. Paragraphs are free-form. */}}
{{end}}

{{define "section"}}
{{if .Title}}<h2 id="TOC_{{.FormattedNumber}}">{{.Title}}</h2>
{{end -}}
{{elems $.Template .Elem}}
{{- end}}

{{define "fontsize"}}{{end}}

{{define "text" -}}
<p>{{range $i, $l := .Lines}}{{if $i}}{{template "newline"}}{{end}}{{$l}}{{end}}</p>
{{end}}

{{define "code" -}}
{{if .Play -}}
<div class="playground">{{.Text}}</div>
{{else -}}
<div class="code">{{.Text}}</div>
{{end -}}
{{end}}

{{define "image" -}}
<img src="{{.URL}}"{{with .Height}} height="{{.}}"{{end}}{{with .Width}} width="{{.}}"{{end}} alt="">
{{end}}

{{define "caption" -}}
<figcaption>{{markdown .Text}}</figcaption>
{{end}}

{{define "iframe" -}}
<iframe src="{{.URL}}"{{with .Height}} height="{{.}}"{{end}}{{with .Width}} width="{{.}}"{{end}}></iframe>
{{end}}

{{define "link" -}}
<p class="link"><a href="{{.URL}}">{{markdown .Label}}</a></p>
{{end}}

{{define "html" -}}{{.HTML}}{{end}}
`

func TestNotesBlock(t *testing.T) {
	const input = `# Title

## One

::: notes
- a
// kept

## Not a slide
:::

## Two
`
	doc, err := Parse(strings.NewReader(input), "x.slide", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sections) != 2 {
		t.Fatalf("got %d sections, want 2", len(doc.Sections))
	}
	if got, want := doc.Sections[0].Notes, []string{"- a\n// kept\n\n## Not a slide"}; !slices.Equal(got, want) {
		t.Errorf("notes: got %q, want %q", got, want)
	}
	if got := doc.Sections[1].Line; got != 12 {
		t.Errorf("second section on line %d, want 12", got)
	}

	_, err = Parse(strings.NewReader("# Title\n\n## One\n\n::: notes\nnever closed\n"), "x.slide", 0)
	if err == nil || !strings.Contains(err.Error(), "x.slide:5: notes block is not closed") {
		t.Errorf("unclosed block: got error %v", err)
	}
}

// TestAdjacentLines checks that a line right after a text block, with no
// blank line between them, is not lost.
func TestAdjacentLines(t *testing.T) {
	const input = "# Title\n\n## One\n\nText.\n: A note.\n   \n: Another note.\n"
	doc, err := Parse(strings.NewReader(input), "x.slide", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := doc.Sections[0].Notes, []string{"A note.", "Another note."}; !slices.Equal(got, want) {
		t.Errorf("notes: got %q, want %q", got, want)
	}
}

func TestFencedCode(t *testing.T) {
	const input = "# Title\n\n## One\n\n::: notes\n```\n:::\n```\n:::\n\n```\n: code\n::: notes\n```\n\n## Two\n"
	doc, err := Parse(strings.NewReader(input), "x.slide", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sections) != 2 {
		t.Fatalf("got %d sections, want 2", len(doc.Sections))
	}
	if got, want := doc.Sections[0].Notes, []string{"```\n:::\n```"}; !slices.Equal(got, want) {
		t.Errorf("notes: got %q, want %q", got, want)
	}

	for _, tt := range []struct{ input, want string }{
		{"# Title\n\n## One\n\n```go\ncode\n## Two\n", "x.slide:5: fenced code block is not closed by a line with ```"},
		{"# Title\n\n## One\n\n::: notes\n```\n:::\n", "x.slide:5: notes block is not closed by a line with :::"},
		{"# Title\n\n## One\n\nText.\n\n# Two\n\n## Three\n", `x.slide:7: unexpected heading "# Two": only the title begins with "# ", and sections with "## "`},
		{"# Title\n\n## One\n\n<pre>\ncode\n\n## Two\n", "x.slide:5: HTML block is not closed by a line with </pre>"},
		{"# Title\n\n## One\n\n<!-- comment\n\n## Two\n", "x.slide:5: HTML block is not closed by a line with -->"},
		{"# Title\n\n## One\n\n- item\n\n  ```\n  code\n\n## Two\n", ""},
	} {
		_, err := Parse(strings.NewReader(tt.input), "x.slide", 0)
		if tt.want == "" {
			// A code block in a list ends with the list.
			if err != nil {
				t.Errorf("Parse(%q): %v", tt.input, err)
			}
			continue
		}
		if err == nil || err.Error() != tt.want {
			t.Errorf("Parse(%q): got error %v, want %s", tt.input, err, tt.want)
		}
	}
}

func TestCodeFence(t *testing.T) {
	for _, tt := range []struct {
		line, fence string
	}{
		{"```", "```"},
		{"```go", "```"},
		{"   ~~~~ info `x`", "~~~~"},
		{"    ```", ""},
		{"``", ""},
		{"``` `x`", ""},
		{"\t```", ""},
	} {
		if fence, _ := codeFence(tt.line); fence != tt.fence {
			t.Errorf("codeFence(%q) = %q, want %q", tt.line, fence, tt.fence)
		}
	}
	for _, tt := range []struct {
		line, fence string
		want        bool
	}{
		{"```", "```", true},
		{"  ````  ", "```", true},
		{"``", "```", false},
		{"``` go", "```", false},
		{"~~~", "```", false},
		{"    ```", "```", false},
	} {
		if got := closesFence(tt.line, tt.fence); got != tt.want {
			t.Errorf("closesFence(%q, %q) = %v, want %v", tt.line, tt.fence, got, tt.want)
		}
	}
}

// TestAuthors checks that authors are separated by blank lines, however
// many, also lines of spaces.
func TestAuthors(t *testing.T) {
	const input = "# Title\n\nAda\nada@example.com  \n\n\n\nBob\n   \nCy\n\n\n## Slide\n"
	doc, err := Parse(strings.NewReader(input), "x.slide", 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, a := range doc.Authors {
		var lines []string
		for _, e := range a.Elem {
			switch e := e.(type) {
			case Text:
				lines = append(lines, e.Lines...)
			case Link:
				lines = append(lines, e.Label)
			}
		}
		got = append(got, strings.Join(lines, "|"))
	}
	if want := []string{"Ada|ada@example.com", "Bob", "Cy"}; !slices.Equal(got, want) {
		t.Errorf("authors: got %q, want %q", got, want)
	}
}
