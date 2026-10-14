package main

import (
	"bytes"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/some-programs/present/pkg/present"
)

func TestSlideSources(t *testing.T) {
	src, err := fs.ReadFile(examplesRoot.fsys, "media.slide")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := parse(examplesRoot.fsys, bytes.NewReader(src), "media.slide", 0)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := slideSources(doc, src, examplesRoot.fsys, "media.slide", "/_examples/media.slide")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(sources), 1+len(doc.Sections); got != want {
		t.Fatalf("got %d slides, want %d", got, want)
	}
	for i, s := range sources {
		if s.Number != i+1 {
			t.Errorf("slide %d has number %d", i+1, s.Number)
		}
	}
	for i, s := range doc.Sections {
		if n := sources[i+1].Lines()[0].N; n != s.Line {
			t.Errorf("slide %d source starts at line %d, want %d", i+2, n, s.Line)
		}
	}
	if n := sources[0].Lines()[0].N; n != 1 {
		t.Errorf("title slide source starts at line %d, want 1", n)
	}
	if !strings.HasPrefix(sources[0].Text, "# ") {
		t.Errorf("title slide source starts with %q, want the title", sources[0].Text)
	}
	for i, s := range doc.Sections {
		text := sources[i+1].Text
		if first, _, _ := strings.Cut(text, "\n"); !strings.HasPrefix(first, "## "+s.Title) {
			t.Errorf("slide %d source starts with %q, want the heading of %q", i+2, first, s.Title)
		}
		if strings.HasSuffix(text, "\n") {
			t.Errorf("slide %d source ends in blank lines", i+2)
		}
	}

	files := make(map[string]sourceFile)
	for _, s := range sources {
		for _, f := range s.Files {
			files[f.Name] = f
		}
	}
	if f := files["diagrams/pipeline.dot"]; f.URL != "/_examples/diagrams/pipeline.dot" || !strings.Contains(f.Text, "digraph") {
		t.Errorf("diagram source = %+v, want its URL and text", f)
	}
	if f := files["images/plasma.jpg"]; f.URL != "/_examples/images/plasma.jpg" || f.Text != "" {
		t.Errorf("image = %+v, want its URL and no text", f)
	}
}

func TestCommandFiles(t *testing.T) {
	var lines []present.Line
	for _, line := range []string{
		".code -edit -numbers code/fib/main.go /^func main/",
		".play code/fib/main.go",
		".image -background-color transparent diagrams/run.mmd 0.5",
		".iframe https://go.dev/ 300 400",
		".link https://go.dev/ Go",
	} {
		lines = append(lines, present.Line{Text: line, Kind: present.CommandLine})
	}
	lines = append(lines, present.Line{Text: "\\.code not/a/command.go", Kind: present.TextLine})
	files := commandFiles(lines, examplesRoot.fsys, "code.slide", "/_examples/code.slide")
	if len(files) != 3 {
		t.Fatalf("got files %+v, want code/fib/main.go once, the diagram and the iframe", files)
	}
	if f := files[0]; f.Name != "code/fib/main.go" || f.Text == "" {
		t.Errorf("code file = %+v, want its text", f)
	}
	if f := files[1]; f.Name != "diagrams/run.mmd" || f.Text == "" {
		t.Errorf("diagram file = %+v, want its text", f)
	}
	if f := files[2]; f.URL != "https://go.dev/" || f.Text != "" {
		t.Errorf("iframe = %+v, want its URL unchanged", f)
	}
}

// TestSourceBlocks checks that the lines of code blocks, HTML blocks and
// blocks of notes are not taken for commands or headings.
func TestSourceBlocks(t *testing.T) {
	for _, tt := range []struct {
		slide string
		want  []string // the class of each line
	}{
		{
			"## One\n\n::: notes\n.code in/notes.go\n:::\n.code code/fib/main.go",
			[]string{"src-heading", "", "src-note", "src-note", "src-note", "src-command"},
		},
		{
			"## One\n\n```\n# shell comment\n.code in/code.go\n```\n.code code/fib/main.go",
			[]string{"src-heading", "", "", "", "", "", "src-command"},
		},
		{
			"## One\n\n<pre>\n# not a heading\n.code in/html.go\n</pre>\n\n.code code/fib/main.go",
			[]string{"src-heading", "", "", "", "", "", "", "src-command"},
		},
	} {
		src := []byte("# Title\n\n" + tt.slide + "\n")
		doc, err := parse(examplesRoot.fsys, bytes.NewReader(src), "code.slide", 0)
		if err != nil {
			t.Fatal(err)
		}
		sources, err := slideSources(doc, src, examplesRoot.fsys, "code.slide", "/_examples/code.slide")
		if err != nil {
			t.Fatal(err)
		}
		s := sources[1]
		var classes []string
		for _, l := range s.Lines() {
			classes = append(classes, l.Class)
		}
		if !slices.Equal(classes, tt.want) {
			t.Errorf("%q: classes %q, want %q", tt.slide, classes, tt.want)
		}
		if len(s.Files) != 1 || s.Files[0].Name != "code/fib/main.go" {
			t.Errorf("%q: files %+v, want only code/fib/main.go", tt.slide, s.Files)
		}
	}
}
