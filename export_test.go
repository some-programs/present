package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/some-programs/present/pkg/present"
)

// runExport runs the export command with args, keeping the settings it
// changes for the rest of the test only.
func runExport(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	play, notes, preload := present.PlayEnabled, present.NotesEnabled, present.PreloadEnabled
	t.Cleanup(func() {
		present.PlayEnabled, present.NotesEnabled, present.PreloadEnabled = play, notes, preload
	})
	var out, errOut bytes.Buffer
	code = runCommand("export", args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestExport(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(t.TempDir(), "out")
	writeFiles(t, dir, map[string]string{
		"talk/x.slide": `# Talk

## Media

.background bg.jpg
.image pic%20one.png
.image /shared/logo.svg
.image https://example.com/remote.png
.video clip.mp4 video/mp4
.iframe frame.html 300 400

### Nested

.image ../shared/nested.png

## Code

.play prog.go

## Links

[Other](../other.article#TOC_1.) and [Go](https://go.dev/x.slide)
`,
		"talk/prog.go":       "package main\n\nfunc main() {}\n",
		"talk/bg.jpg":        "jpg",
		"talk/pic one.png":   "png",
		"talk/clip.mp4":      "mp4",
		"talk/frame.html":    "<p>frame</p>\n",
		"talk/unused.png":    "unused",
		"shared/logo.svg":    `<svg xmlns="http://www.w3.org/2000/svg"><text font-family="Roboto">Logo</text></svg>`,
		"shared/nested.png":  "nested",
		"other.article":      "# Other\n\n## One\n\n.image shared/nested.png\n",
		"static/styles.css":  "the content's own",
		"talk/untouched.txt": "x",
	})

	t.Chdir(dir)
	code, stdout, stderr := runExport(t, "-o", out, "talk/x.slide", "other.article")
	if code != 0 {
		t.Fatalf("exit code %d, error output %q", code, stderr)
	}
	for _, page := range []string{"talk/x.html", "other.html"} {
		if !strings.Contains(stdout, filepath.Join(out, page)) {
			t.Errorf("output %q does not name %s", stdout, page)
		}
	}

	for name, want := range map[string]string{
		"talk/bg.jpg":       "jpg",
		"talk/pic one.png":  "png",
		"talk/clip.mp4":     "mp4",
		"talk/frame.html":   "<p>frame</p>\n",
		"shared/nested.png": "nested",
	} {
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil || string(data) != want {
			t.Errorf("%s: %q, %v; want %q", name, data, err, want)
		}
	}
	for _, name := range []string{"talk/unused.png", "talk/untouched.txt", "talk/prog.go", "static/fonts/ttf"} {
		if _, err := os.Stat(filepath.Join(out, name)); err == nil {
			t.Errorf("%s was written", name)
		}
	}
	for _, name := range []string{"static/styles.css", "static/js/slides.js", "static/fonts/OFL-Roboto.txt"} {
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Error(err)
			continue
		}
		if want, _ := embedFS.ReadFile(name); !bytes.Equal(data, want) {
			t.Errorf("%s differs from the embedded file", name)
		}
	}
	css, err := os.ReadFile(filepath.Join(out, "static/fonts.css"))
	if err != nil || !bytes.Contains(css, []byte("src:url(data:font/woff2;base64,")) || bytes.Contains(css, []byte("url(fonts/")) {
		t.Errorf("static/fonts.css: %v, or the fonts are not embedded", err)
	}
	if woff2s, _ := filepath.Glob(filepath.Join(out, "static/fonts/*.woff2")); woff2s != nil {
		t.Errorf("font files written: %q", woff2s)
	}
	svg, err := os.ReadFile(filepath.Join(out, "shared/logo.svg"))
	if err != nil || !bytes.Contains(svg, []byte("@font-face")) {
		t.Errorf("shared/logo.svg: %q, %v; want the fonts embedded", svg, err)
	}

	page, err := os.ReadFile(filepath.Join(out, "talk/x.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`href='../static/styles.css'`,
		`url('bg.jpg')`,
		`src="pic%20one.png"`,
		`src="../shared/logo.svg"`,
		`src="https://example.com/remote.png"`,
		`src="clip.mp4"`,
		`src="frame.html"`,
		`src="../shared/nested.png"`,
		`href="../other.html#TOC_1."`,
		`href="https://go.dev/x.slide"`,
	} {
		if !bytes.Contains(page, []byte(want)) {
			t.Errorf("talk/x.html does not contain %s", want)
		}
	}
	for _, bad := range []string{`'/static/`, "code playground", "play.js", "hot.js", "woff2", "module", "Press 'S'"} {
		if bytes.Contains(page, []byte(bad)) {
			t.Errorf("talk/x.html contains %s", bad)
		}
	}
	scripts := regexp.MustCompile(`<script defer src='\.\./static/js/(\w+)\.js'>`).FindAllSubmatch(page, -1)
	var names []string
	for _, m := range scripts {
		names = append(names, string(m[1]))
	}
	if want := []string{"notes", "source", "slides"}; !slices.Equal(names, want) {
		t.Errorf("talk/x.html loads scripts %q, want %q", names, want)
	}
	if !regexp.MustCompile(`notesEnabled = +false`).Match(page) {
		t.Error("talk/x.html does not turn notes off")
	}
	article, err := os.ReadFile(filepath.Join(out, "other.html"))
	if err != nil || !bytes.Contains(article, []byte(`href="static/article.css"`)) {
		t.Errorf("other.html does not link to static/article.css: %v", err)
	}
}

func TestExportSource(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(t.TempDir(), "out")
	writeFiles(t, dir, map[string]string{
		"talk/x.slide": `# Talk

## Code

.code prog.go
.html inc.html
.image /shared/logo.png
: A note.
`,
		"talk/y.article":  "# Article\n\n## One\n\n.code prog.go\n",
		"talk/prog.go":    "package main\n",
		"talk/inc.html":   "<p>inc</p>\n",
		"shared/logo.png": "png",
	})
	t.Chdir(dir)
	code, _, stderr := runExport(t, "-o", out, "-source", "talk/x.slide", "talk/y.article")
	if code != 0 {
		t.Fatalf("exit code %d, error output %q", code, stderr)
	}
	for _, name := range []string{"talk/prog.go", "talk/inc.html", "shared/logo.png"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Error(err)
		}
	}
	page, err := os.ReadFile(filepath.Join(out, "talk/x.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<aside id="source"`,
		`<div class="src-name">talk/x.slide</div>`,
		`<a href="prog.go" target="_blank" rel="noopener">prog.go</a>`,
		`<a href="inc.html" target="_blank" rel="noopener">inc.html</a>`,
		`<a href="../shared/logo.png" target="_blank" rel="noopener">/shared/logo.png</a>`,
		`<span class="src-note">: A note.`,
		"Press 'S' to show or hide the source of the slide.",
	} {
		if !bytes.Contains(page, []byte(want)) {
			t.Errorf("talk/x.html does not contain %s", want)
		}
	}
	// Articles have no source panel.
	article, err := os.ReadFile(filepath.Join(out, "talk/y.html"))
	if err != nil || bytes.Contains(article, []byte(`id="source"`)) {
		t.Errorf("talk/y.html: %v, or it has a source panel", err)
	}
}

func TestExportErrors(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(t.TempDir(), "out")
	writeFiles(t, dir, map[string]string{
		"a.slide":       "# A\n\n## One\n\nText.\n",
		"a.article":     "# A\n\n## One\n\nText.\n",
		"missing.slide": "# M\n\n## One\n\n.image gone.png\n",
		"notes.txt":     "x",
	})
	outside := filepath.Join(t.TempDir(), "b.slide")
	writeFiles(t, filepath.Dir(outside), map[string]string{"b.slide": "# B\n"})

	t.Chdir(dir)
	for _, tt := range []struct {
		args  []string
		code  int
		inErr string
	}{
		{[]string{"-o", out}, 2, "usage: present export"},
		{[]string{"-o", out, "notes.txt"}, 1, "is not a slide presentation"},
		{[]string{"-o", out, outside}, 1, "is not in the current directory"},
		{[]string{"-o", ".", "a.slide"}, 1, "is the current directory"},
		{[]string{"-o", dir, "a.slide"}, 1, "is the current directory"},
		{[]string{"-o", out, "a.slide", "a.article"}, 1, "would both be written to a.html"},
		{[]string{"-o", out, "missing.slide"}, 1, "gone.png"},
		{[]string{"-o", out, "-content", dir, "a.slide"}, 2, "flag provided but not defined: -content"},
	} {
		code, _, stderr := runExport(t, tt.args...)
		if code != tt.code || !strings.Contains(stderr, tt.inErr) {
			t.Errorf("present export %s: exit code %d, error output %q; want %d and %q",
				strings.Join(tt.args, " "), code, stderr, tt.code, tt.inErr)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "static")); err == nil {
		t.Error("export wrote to the content directory")
	}
}
