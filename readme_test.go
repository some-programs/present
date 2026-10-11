package main

import (
	"strings"
	"testing"
)

func TestRenderReadme(t *testing.T) {
	got, err := renderReadme([]byte("See [examples](examples), [Go](https://go.dev) and [x](#x).\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`href="` + repoURL + `examples"`,
		`href="https://go.dev"`,
		`href="#x"`,
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("renderReadme output %q does not contain %q", got, want)
		}
	}
}
