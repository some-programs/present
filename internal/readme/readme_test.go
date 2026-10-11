package readme

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	got, err := Render([]byte("See [examples](examples), [intro](examples/intro.slide), [LICENSE](LICENSE), [Go](https://go.dev) and [x](#x).\n\n![shot](images/shot.png)\n"), "/_examples")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`href="/_examples"`,
		`href="/_examples/intro.slide"`,
		`href="` + RepoURL + `LICENSE"`,
		`href="https://go.dev"`,
		`href="#x"`,
		`src="` + RawURL + `images/shot.png"`,
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("Render output %q does not contain %q", got, want)
		}
	}
}
