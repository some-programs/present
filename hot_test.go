package main

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// setContentPath sets *contentPath to dir for the duration of the test.
func setContentPath(t *testing.T, dir string) {
	old := *contentPath
	*contentPath = dir
	t.Cleanup(func() { *contentPath = old })
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		name = filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHotFiles(t *testing.T) {
	dir := t.TempDir()
	setContentPath(t, dir)
	writeFiles(t, dir, map[string]string{
		"talk/x.slide": `# Talk

## Code

.code src/a.go
.html b.html

## Images

.background bg.jpg
.image pic%20one.png
.image g.dot
.image /shared/logo.png
.image https://example.com/remote.png
.video clip.mp4 video/mp4

### Nested

.image nested.png
`,
		"talk/src/a.go": "package a\n",
		"talk/b.html":   "<p>b</p>\n",
	})

	var got []string
	for _, f := range hotFiles(filepath.Join(dir, "talk/x.slide"), nil) {
		rel, err := filepath.Rel(dir, f)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, filepath.ToSlash(rel))
	}
	want := []string{
		"shared/logo.png",
		"talk/b.html",
		"talk/bg.jpg",
		"talk/clip.mp4",
		"talk/g.dot",
		"talk/nested.png",
		"talk/pic one.png",
		"talk/src/a.go",
		"talk/x.slide",
	}
	if !slices.Equal(got, want) {
		t.Errorf("hotFiles:\ngot  %q\nwant %q", got, want)
	}

	// A presentation that does not parse keeps the files watched before.
	old := []string{filepath.Join(dir, "talk/src/a.go")}
	writeFiles(t, dir, map[string]string{"talk/x.slide": "# Talk\n\n## Bad\n\n.nosuchcommand\n"})
	got = hotFiles(filepath.Join(dir, "talk/x.slide"), old)
	want = []string{filepath.Join(dir, "talk/src/a.go"), filepath.Join(dir, "talk/x.slide")}
	if !slices.Equal(got, want) {
		t.Errorf("hotFiles after a syntax error:\ngot  %q\nwant %q", got, want)
	}
}

func TestHotHandler(t *testing.T) {
	dir := t.TempDir()
	setContentPath(t, dir)
	old := hotInterval
	hotInterval = 10 * time.Millisecond
	t.Cleanup(func() { hotInterval = old })
	// isDoc knows the presentation extensions from the templates.
	if err := initTemplates(embedFS, func(path string) string { return path }); err != nil {
		t.Fatal(err)
	}

	writeFiles(t, dir, map[string]string{
		"x.slide": "# Talk\n\n## One\n\n.code a.go\n",
		"a.go":    "package a\n",
	})

	srv := httptest.NewServer(http.HandlerFunc(hotHandler))
	defer srv.Close()

	if res, err := http.Get(srv.URL + "/_hot?doc=/nosuch.txt"); err != nil {
		t.Fatal(err)
	} else if res.Body.Close(); res.StatusCode != http.StatusNotFound {
		t.Errorf("status for a file that is not a presentation: got %d, want 404", res.StatusCode)
	}

	res, err := http.Get(srv.URL + "/_hot?doc=/x.slide")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type: got %q, want text/event-stream", ct)
	}

	events := make(chan string)
	go func() {
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			if ev, ok := strings.CutPrefix(sc.Text(), "event: "); ok {
				events <- ev
			}
		}
		// The stream ends when the test closes it; any error then is
		// of no interest.
		close(events)
	}()
	expect := func(what string) {
		t.Helper()
		select {
		case ev := <-events:
			if ev != "change" {
				t.Fatalf("after %s: got event %q, want change", what, ev)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("after %s: no change event", what)
		}
	}
	quiet := func(what string) {
		t.Helper()
		select {
		case ev := <-events:
			t.Fatalf("after %s: got event %q, want none", what, ev)
		case <-time.After(100 * time.Millisecond):
		}
	}

	quiet("connecting")

	// A file included by the presentation.
	writeFiles(t, dir, map[string]string{"a.go": "package a // changed\n"})
	expect("changing an included file")
	quiet("the change event")

	// A file the presentation starts to use is watched from then on.
	writeFiles(t, dir, map[string]string{"x.slide": "# Talk\n\n## One\n\n.code a.go\n.image p.png\n"})
	expect("changing the presentation")
	writeFiles(t, dir, map[string]string{"p.png": "png"})
	expect("adding a new image")
}
