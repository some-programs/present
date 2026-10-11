package legacy

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/some-programs/present/pkg/present"
)

func TestConvert(t *testing.T) {
	for _, tt := range []struct {
		name     string
		in, want string
	}{
		{
			"header",
			`#comment above the title
Title
Subtitle
10 Nov 2009
Tags: go, talk
OldURL: /old
: a note on the title

Author Name
Job, Company
author@example.com
http://example.com/
@twitter

* Slide

Text
`,
			`# Title
//comment above the title
Subtitle
10 Nov 2009
Tags: go, talk
: a note on the title

Author Name
Job, Company
author@example.com
http://example.com/
@twitter

## Slide

Text
`,
		},
		{
			"line breaks in a slide",
			`Title

* Slide

One
two
three

- a list
 continued
- two
`,
			`# Title

## Slide

One\
two\
three

- a list
  continued
- two
`,
		},
		{
			"inline markup",
			"Title\n\n* Slide\n\n" +
				"Some *bold*, _italic_too_ and `code`.\n" +
				"A [[https://go.dev/][link with *bold*]] and [[https://go.dev/doc]].\n" +
				"Not markup: a*b, snake_case, x < y, <b>, &amp; and []int.\n",
			"# Title\n\n## Slide\n\n" +
				"Some **bold**, *italic too* and `code`.\\\n" +
				"A [link with **bold**](https://go.dev/) and [go.dev/doc](https://go.dev/doc).\\\n" +
				"Not markup: a\\*b, snake_case, x < y, \\<b>, \\&amp; and []int.\n",
		},
		{
			"line starts",
			`Title

* Slide

Text
- not a list
1. not a list
+ not a list
> not a quote
---
\.not a command
//not a comment
`,
			`# Title

## Slide

Text\
\- not a list\
1\. not a list\
\+ not a list\
\> not a quote\
\---\
\.not a command\
\//not a comment
`,
		},
		{
			"article",
			`Title

* Section

Lines of an article
are filled.
`,
			`# Title

## Section

Lines of an article
are filled.
`,
		},
		{
			"preformatted",
			"Title\n\n* Slide\n\n" +
				"\tfunc main() {\n\t\t// comment\n\t}\n\n\n" +
				"- list\n\n" +
				"\tafter a list\n\n" +
				"    after code\n",
			"# Title\n\n## Slide\n\n" +
				"\tfunc main() {\n\t\t// comment\n\t}\n\n" +
				"- list\n\n" +
				" ```\n after a list\n ```\n\n" +
				" ```\n after code\n ```\n",
		},
		{
			"lists",
			"Title\n\n* Slide\n\n- one\n- two\n\n- three\n\n- four\n",
			"# Title\n\n## Slide\n\n- one\n- two\n\n+ three\n\n- four\n",
		},
		{
			"commands, notes and comments",
			`Title

* Slide

.image a.png
.caption _Gopher_ by [[https://reneefrench.blogspot.com][Renee French]]
# a comment

.link https://go.dev The *Go* site
: a *note*
:
Text
# in a paragraph
more

** Subsection

Sub text
`,
			`# Title

## Slide

.image a.png
.caption *Gopher* by [Renee French](https://reneefrench.blogspot.com)

// a comment
.link https://go.dev The **Go** site
: a \*note\*
:

// in a paragraph
Text\
more

### Subsection

Sub text
`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			name := "test.slide"
			if tt.name == "article" {
				name = "test.article"
			}
			got, err := Convert(name, []byte(tt.in))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("Convert:\n%s\nwant:\n%s", got, tt.want)
			}
			if _, err := present.Parse(bytes.NewReader(got), name, 0); err != nil {
				t.Errorf("present.Parse of the result: %v", err)
			}
		})
	}
}

func TestConvertErrors(t *testing.T) {
	for _, tt := range []struct {
		in, want string
	}{
		{"# Title\n\n## Slide\n", "test.slide: already in the Markdown markup"},
		{"", "test.slide: unexpected EOF; expected title"},
		{"Title\n\nAuthor\n", "test.slide: unexpected EOF; expected a section"},
		{"Title\n\n* Slide\n\n*** Too deep\n", "test.slide:5: badly nested section inside *: *** Too deep"},
	} {
		_, err := Convert("test.slide", []byte(tt.in))
		if err == nil || err.Error() != tt.want {
			t.Errorf("Convert(%q) error = %v, want %q", tt.in, err, tt.want)
		}
	}
}

// TestCorpus converts the legacy presentations in the directory that
// $PRESENT_LEGACY_CORPUS names, such as the _content/talks directory of
// golang.org/x/website, and checks that present parses the results into
// as many sections. With $PRESENT_LEGACY_OUT, it writes the results into
// that directory too.
func TestCorpus(t *testing.T) {
	dir := os.Getenv("PRESENT_LEGACY_CORPUS")
	if dir == "" {
		t.Skip("$PRESENT_LEGACY_CORPUS is not set")
	}
	out := os.Getenv("PRESENT_LEGACY_OUT")
	heading := regexp.MustCompile(`(?m)^\*+( |$)`)
	n := 0
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || (filepath.Ext(path) != ".slide" && filepath.Ext(path) != ".article") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil || !IsLegacy(data) {
			return err
		}
		n++
		got, err := Convert(path, data)
		if err != nil {
			t.Error(err)
			return nil
		}
		if out != "" {
			rel, _ := filepath.Rel(dir, path)
			if err := os.MkdirAll(filepath.Join(out, filepath.Dir(rel)), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(out, rel), got, 0o644); err != nil {
				return err
			}
		}
		doc, err := present.Parse(bytes.NewReader(got), path, 0)
		if err != nil {
			t.Error(err)
			return nil
		}
		// Headings in comments do not count, nor do lines in paragraphs
		// that look like headings.
		want := 0
		for line := range strings.SplitSeq(string(data), "\n") {
			if heading.MatchString(line) {
				want++
			}
		}
		if got := countSections(doc.Sections); got > want {
			t.Errorf("%s: %d sections, want at most %d", path, got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("converted %d presentations", n)
}

func countSections(sections []present.Section) int {
	n := len(sections)
	for _, s := range sections {
		for _, e := range s.Elem {
			if s, ok := e.(present.Section); ok {
				n += countSections([]present.Section{s})
			}
		}
	}
	return n
}
