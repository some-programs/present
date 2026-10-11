package main

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/some-programs/present/pkg/present"
)

func TestSlideSources(t *testing.T) {
	src, err := os.ReadFile("examples/media.slide")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := present.Parse(bytes.NewReader(src), "examples/media.slide", 0)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := slideSources(doc, src, "examples/media.slide", "/_examples/media.slide")
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
	files := commandFiles([]string{
		".code -edit -numbers code/fib/main.go /^func main/",
		".play code/fib/main.go",
		".iframe https://go.dev/ 300 400",
		".link https://go.dev/ Go",
		"\\.code not/a/command.go",
	}, "examples/code.slide", "/_examples/code.slide")
	if len(files) != 2 {
		t.Fatalf("got files %+v, want code/fib/main.go once and the iframe", files)
	}
	if f := files[0]; f.Name != "code/fib/main.go" || f.Text == "" {
		t.Errorf("code file = %+v, want its text", f)
	}
	if f := files[1]; f.URL != "https://go.dev/" || f.Text != "" {
		t.Errorf("iframe = %+v, want its URL unchanged", f)
	}
}

func TestSourceFileName(t *testing.T) {
	defer func(old bool) { *showDoc = old }(*showDoc)
	*showDoc = true
	for urlPath, want := range map[string]string{
		"/_examples/media.slide":   "examples/media.slide",
		"/talks/go/intro.slide":    "talks/go/intro.slide",
		"/_examplesx/a.slide":      "_examplesx/a.slide",
		"/_examples/../mine.slide": "mine.slide",
	} {
		if got := sourceFileName(urlPath); got != want {
			t.Errorf("sourceFileName(%q) = %q, want %q", urlPath, got, want)
		}
	}
}

func TestRenderDocSource(t *testing.T) {
	if err := initTemplates(embedFS, func(path string) string { return path }); err != nil {
		t.Fatal(err)
	}
	defer func(old bool) { *showDoc = old }(*showDoc)
	for _, doc := range []bool{false, true} {
		*showDoc = doc
		var buf bytes.Buffer
		if err := renderDoc(&buf, "examples/intro.slide", "/_examples/intro.slide"); err != nil {
			t.Fatal(err)
		}
		page := buf.String()
		if got := strings.Contains(page, `<aside id="source"`); got != doc {
			t.Errorf("with -doc=%v, page has source panel: %v", doc, got)
		}
		if !doc {
			continue
		}
		for _, want := range []string{
			`<div class="src-name">examples/intro.slide</div>`,
			`<span class="src-heading">## Links and HTML` + "\n" + `</span>`,
			`<span class="src-command">.html html/badge.html` + "\n" + `</span>`,
			// The included HTML is shown as text.
			`<pre>&lt;p style=`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("page does not contain %q", want)
			}
		}
	}
}

func TestSourceNotesBlock(t *testing.T) {
	s := slideSource{Text: "## One\n\n::: notes\n.code in/notes.go\n:::\n.code code/fib/main.go"}
	var classes []string
	for _, l := range s.Lines() {
		classes = append(classes, l.Class)
	}
	want := []string{"src-heading", "", "src-note", "src-note", "src-note", "src-command"}
	if !slices.Equal(classes, want) {
		t.Errorf("classes: got %q, want %q", classes, want)
	}
	files := commandFiles(strings.Split(s.Text, "\n"), "examples/code.slide", "/_examples/code.slide")
	if len(files) != 1 || files[0].Name != "code/fib/main.go" {
		t.Errorf("files: got %+v, want only code/fib/main.go", files)
	}
}
