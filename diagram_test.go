package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		svg, err := renderDiagram(src, out)
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
	out := filepath.Join(dir, "g.svg")
	if err := os.WriteFile(src, []byte("graph TD"), 0o644); err != nil {
		t.Fatal(err)
	}
	svg, err := renderDiagram(src, out)
	if err != nil {
		t.Fatal(err)
	}
	if want := "<svg>graph TD</svg>\n<!-- present: mmdc sha256:"; !strings.HasPrefix(string(svg), want) {
		t.Errorf("renderDiagram = %q, want it to start with %q", svg, want)
	}

	if err := os.WriteFile(src, []byte("bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = renderDiagram(src, out)
	want := src + ": Error: Parse error on line 1:\nbad\n---^"
	if err == nil || err.Error() != want {
		t.Errorf("renderDiagram error = %v, want %q", err, want)
	}
}
