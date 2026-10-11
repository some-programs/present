package main

import (
	"strings"
	"testing"
)

func TestRenderReadme(t *testing.T) {
	got, err := renderReadme([]byte("See [examples](examples), [intro](examples/intro.slide), [LICENSE](LICENSE), [Go](https://go.dev) and [x](#x).\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`href="/_examples"`,
		`href="/_examples/intro.slide"`,
		`href="` + repoURL + `LICENSE"`,
		`href="https://go.dev"`,
		`href="#x"`,
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("renderReadme output %q does not contain %q", got, want)
		}
	}
}
