package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderDot(t *testing.T) {
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
		svg, err := renderDot(src, out)
		if (err != nil) != wantErr {
			t.Errorf("renderDot error = %v, want error %v", err, wantErr)
		}
		got, _ := os.ReadFile(out)
		if !strings.HasPrefix(string(got), wantSVG) {
			t.Errorf("g.svg = %q, want it to start with %q", got, wantSVG)
		}
		if string(svg) != string(got) {
			t.Errorf("renderDot returned %q, want the contents of g.svg, %q", svg, got)
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

	// With rendering turned off, any earlier rendering is used as it is.
	*dotCmd = ""
	check(false, "<svg>digraph { b }</svg>", 3)
	os.Remove(out)
	check(true, "", 3)

	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".g.svg.*")); len(leftovers) > 0 {
		t.Errorf("temporary files left: %v", leftovers)
	}
}
