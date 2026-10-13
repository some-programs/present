package main

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestRenderNotes(t *testing.T) {
	if err := initTemplates(embedFS, func(path string) string { return path }); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"x.slide": `# Talk
: Welcome **everyone**.

::: notes
- first
- second
:::

Ada Example

## One

Text.

: See [the docs](https://go.dev/doc/).
: A type parameter ` + "`<T>`" + `.

::: notes
## Not a slide

.code not/a/command.go

	// a comment in code
:::

### Part

: A note in a subsection.

:::notes
In a subsection too.
:::

## Two

Nothing to say.
`,
	})
	var buf bytes.Buffer
	if err := renderNotes(&buf, os.DirFS(dir), "x.slide"); err != nil {
		t.Fatal(err)
	}

	// One block for each slide, and one for slides that present adds.
	blocks := regexp.MustCompile(`(?s)<div class='slide-notes' hidden>(.*?)</div>`).FindAllStringSubmatch(buf.String(), -1)
	want := []string{
		"<p>Welcome <strong>everyone</strong>.</p>\n" +
			"<ul>\n<li>first</li>\n<li>second</li>\n</ul>",
		`<p>See <a href="https://go.dev/doc/" target="_blank" rel="noopener">the docs</a>.</p>` + "\n" +
			"<p>A type parameter <code>&lt;T&gt;</code>.</p>\n" +
			"<h2>Not a slide</h2>\n" +
			"<p>.code not/a/command.go</p>\n" +
			"<pre><code>// a comment in code\n</code></pre>\n" +
			"<p>A note in a subsection.</p>\n" +
			"<p>In a subsection too.</p>",
		"",
		"",
	}
	if len(blocks) != len(want) {
		t.Fatalf("got %d blocks of notes, want %d:\n%s", len(blocks), len(want), buf.String())
	}
	for i, b := range blocks {
		if got := strings.TrimSpace(b[1]); got != want[i] {
			t.Errorf("notes of slide %d:\ngot  %s\nwant %s", i+1, got, want[i])
		}
	}
}
