package main

import (
	"bytes"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/some-programs/present/pkg/present"
)

func TestRenderDiagram(t *testing.T) {
	dir := t.TempDir()

	// A stand-in for dot that wraps its input in an svg element, fails on
	// input containing "bad", and counts its runs.
	fake := filepath.Join(dir, "fakedot")
	runs := filepath.Join(dir, "runs")
	script := `#!/bin/sh
echo run >> ` + runs + `
if grep -q bad "$2"; then echo "Error: $2: syntax error" >&2; exit 1; fi
printf '<svg>%s</svg>\n' "$(cat "$2")"
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := *dotCmd
	*dotCmd = fake
	t.Cleanup(func() { *dotCmd = old })

	src := filepath.Join(dir, "g.dot")
	out := filepath.Join(dir, "g.svg")
	write := func(name, s string) {
		t.Helper()
		if err := os.WriteFile(name, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	check := func(wantErr bool, wantSVG string, wantRuns int) {
		t.Helper()
		svg, err := renderDiagram(dirRoot(dir), "g.dot", "g.svg", "")
		if (err != nil) != wantErr {
			t.Errorf("renderDiagram error = %v, want error %v", err, wantErr)
		}
		got, _ := os.ReadFile(out)
		if !strings.HasPrefix(string(got), wantSVG) {
			t.Errorf("g.svg = %q, want it to start with %q", got, wantSVG)
		}
		if string(svg) != string(got) {
			t.Errorf("renderDiagram returned %q, want the contents of g.svg, %q", svg, got)
		}
		n, _ := os.ReadFile(runs)
		if got := strings.Count(string(n), "run"); got != wantRuns {
			t.Errorf("dot ran %d times, want %d", got, wantRuns)
		}
	}

	write(src, "digraph { a }")
	check(false, "<svg>digraph { a }</svg>\n<!-- present: dot sha256:", 1)

	// Unchanged source: the stamp matches, so dot does not run.
	check(false, "<svg>digraph { a }</svg>", 1)

	write(src, "digraph { b }")
	check(false, "<svg>digraph { b }</svg>", 2)

	// A failure leaves the earlier rendering in place.
	write(src, "digraph { bad }")
	check(true, "<svg>digraph { b }</svg>", 3)

	// Without dot, an up-to-date rendering is fine and a stale one is not.
	*dotCmd = filepath.Join(dir, "nosuchdot")
	write(src, "digraph { b }")
	check(false, "<svg>digraph { b }</svg>", 3)
	write(src, "digraph { c }")
	check(true, "<svg>digraph { b }</svg>", 3)

	// With automatic rendering turned off, any earlier rendering is used as it is.
	*dotCmd = fake
	*noRender = true
	check(false, "<svg>digraph { b }</svg>", 3)
	*noRender = false
	*dotCmd = ""
	check(false, "<svg>digraph { b }</svg>", 3)
	os.Remove(out)
	check(true, "", 3)

	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".g.svg.*")); len(leftovers) > 0 {
		t.Errorf("temporary files left: %v", leftovers)
	}
}

func TestRenderMermaid(t *testing.T) {
	dir := t.TempDir()

	// A stand-in for mmdc that wraps its input in an svg element, and
	// fails like mmdc, with a stack trace, on input containing "bad".
	fake := filepath.Join(dir, "fakemmdc")
	script := `#!/bin/sh
[ "$*" = "--quiet --input g.mmd --output - --outputFormat svg" ] || { echo "bad arguments: $*" >&2; exit 2; }
if grep -q bad g.mmd; then
	printf 'Error: Parse error on line 1:\nbad\n---^\nParser.parse (https://x.invalid/chunk.mjs:1633:16)\n    at async renderMermaid (file:///x/index.js:689:22)\n' >&2
	exit 1
fi
printf '<svg>%s</svg>\n' "$(cat g.mmd)"
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := *mmdcCmd
	*mmdcCmd = fake
	t.Cleanup(func() { *mmdcCmd = old })

	src := filepath.Join(dir, "g.mmd")
	if err := os.WriteFile(src, []byte("graph TD"), 0o644); err != nil {
		t.Fatal(err)
	}
	svg, err := renderDiagram(dirRoot(dir), "g.mmd", "g.svg", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := "<svg>graph TD</svg>\n<!-- present: mmdc sha256:"; !strings.HasPrefix(string(svg), want) {
		t.Errorf("renderDiagram = %q, want it to start with %q", svg, want)
	}

	if err := os.WriteFile(src, []byte("bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = renderDiagram(dirRoot(dir), "g.mmd", "g.svg", "")
	want := "g.mmd: Error: Parse error on line 1:\nbad\n---^"
	if err == nil || err.Error() != want {
		t.Errorf("renderDiagram error = %v, want %q", err, want)
	}
}

func TestRenderMermaidBackground(t *testing.T) {
	dir := t.TempDir()

	// A stand-in for mmdc that writes its arguments as the SVG.
	fake := filepath.Join(dir, "fakemmdc")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nprintf '<svg>%s</svg>\\n' \"$*\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := *mmdcCmd
	*mmdcCmd = fake
	t.Cleanup(func() { *mmdcCmd = old })

	if err := os.WriteFile(filepath.Join(dir, "g.mmd"), []byte("graph TD"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ bg, want string }{
		{"", "<svg>--quiet --input g.mmd --output - --outputFormat svg</svg>\n<!-- present: mmdc sha256:"},
		{"transparent", "<svg>--quiet --input g.mmd --output - --outputFormat svg --backgroundColor transparent</svg>\n<!-- present: mmdc background-color=transparent sha256:"},
		// A change of background alone renders again.
		{"#fff", "<svg>--quiet --input g.mmd --output - --outputFormat svg --backgroundColor #fff</svg>\n<!-- present: mmdc background-color=#fff sha256:"},
	} {
		svg, err := renderDiagram(dirRoot(dir), "g.mmd", "g.svg", tt.bg)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(svg), tt.want) {
			t.Errorf("renderDiagram with background %q = %q, want it to start with %q", tt.bg, svg, tt.want)
		}
	}
}

func TestRenderDiagramsSameSVG(t *testing.T) {
	dir := t.TempDir()
	write := func(name, s string, perm os.FileMode) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), perm); err != nil {
			t.Fatal(err)
		}
	}
	// A stand-in for dot and mmdc that wraps its input in an svg element.
	write("fake", "#!/bin/sh\nprintf '<svg>%s</svg>\\n' \"$(cat \"$2\")\"\n", 0o755)
	oldDot, oldMmdc := *dotCmd, *mmdcCmd
	*dotCmd, *mmdcCmd = filepath.Join(dir, "fake"), filepath.Join(dir, "fake")
	t.Cleanup(func() { *dotCmd, *mmdcCmd = oldDot, oldMmdc })

	write("g.dot", "digraph { a }", 0o644)
	write("g.mmd", "graph TD", 0o644)
	write("h.dot", "digraph { h }", 0o644)
	write("k.mmd", "graph TD", 0o644)
	slides := "# Title\n\n## One\n.image g.dot\n.image h.dot\n.image k.mmd\n\n## Two\n.image ./g.mmd\n.image h.dot\n.image -background-color transparent k.mmd\n"
	doc, err := present.Parse(strings.NewReader(slides), "index.slide", 0)
	if err != nil {
		t.Fatal(err)
	}
	renderDiagrams(doc, dirRoot(dir), "index.slide")

	var imgs []present.Image
	eachDiagram(doc, func(img *present.Image) { imgs = append(imgs, *img) })
	if len(imgs) != 6 {
		t.Fatalf("got %d diagram images, want 6", len(imgs))
	}
	for _, i := range []int{0, 3} {
		img := imgs[i]
		if !img.Missing || !strings.Contains(img.Error, "g.dot") || !strings.Contains(img.Error, "g.mmd") || !strings.Contains(img.Error, "g.svg") {
			t.Errorf("image %s: Missing = %v, Error = %q, want an error naming g.dot, g.mmd and g.svg", img.Source, img.Missing, img.Error)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "g.svg")); err == nil {
		t.Errorf("g.svg was rendered")
	}
	// One diagram with two backgrounds is two renderings too.
	for _, i := range []int{2, 5} {
		img := imgs[i]
		if want := "k.mmd, -background-color transparent k.mmd: all render to k.svg"; !img.Missing || img.Error != want {
			t.Errorf("image %s: Missing = %v, Error = %q, want %q", img.Source, img.Missing, img.Error, want)
		}
	}
	// The same diagram on two slides is fine.
	for _, i := range []int{1, 4} {
		img := imgs[i]
		if img.Missing || img.Error != "" || !strings.HasPrefix(img.URL, "h.svg?v=") {
			t.Errorf("image %s: URL = %q, Missing = %v, Error = %q, want it rendered", img.Source, img.URL, img.Missing, img.Error)
		}
	}
}

// TestExampleDiagrams checks that the SVG files of the diagrams of the
// examples are renderings of their sources, since the examples are
// read-only and their diagrams are not rendered.
func TestExampleDiagrams(t *testing.T) {
	fsys := examplesRoot.fsys
	docs, err := fs.Glob(fsys, "*.slide")
	if err != nil {
		t.Fatal(err)
	}
	articles, err := fs.Glob(fsys, "*.article")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, name := range append(docs, articles...) {
		doc, err := parseFile(fsys, name, 0)
		if err != nil {
			t.Fatal(err)
		}
		eachDiagram(doc, func(img *present.Image) {
			n++
			source, err := fs.ReadFile(fsys, path.Join(path.Dir(name), img.Source))
			if err != nil {
				t.Fatal(err)
			}
			svg, err := fs.ReadFile(fsys, path.Join(path.Dir(name), img.URL))
			if err != nil {
				t.Fatal(err)
			}
			stamp := diagramStamp(diagramTools[path.Ext(img.Source)], img.BackgroundColor, source)
			if !bytes.Contains(svg, stamp) {
				t.Errorf("%s: %s is not a rendering of %s; render it again", name, img.URL, img.Source)
			}
		})
	}
	if n == 0 {
		t.Error("the examples have no diagrams")
	}
}
