package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	unformatted = "# Title\n\n## Slide\nText.  \n"
	formatted   = "# Title\n\n## Slide\n\nText.\n"
)

func TestDocFiles(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"a.slide":         "",
		"b.article":       "",
		"c.txt":           "",
		"sub/d.slide":     "",
		"_skip/e.slide":   "",
		".hidden/f.slide": "",
	})
	got, err := docFiles([]string{dir}, true)
	if err != nil {
		t.Fatal(err)
	}
	var rel []string
	for _, f := range got {
		r, _ := filepath.Rel(dir, f)
		rel = append(rel, filepath.ToSlash(r))
	}
	if want := []string{"a.slide", "b.article", "sub/d.slide"}; !slices.Equal(rel, want) {
		t.Errorf("docFiles(-r) = %q, want %q", rel, want)
	}
	if _, err := docFiles([]string{dir}, false); err == nil || !strings.Contains(err.Error(), "use -r") {
		t.Errorf("docFiles of a directory without -r: got error %v, want one saying to use -r", err)
	}
}

func TestFmtCommand(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"a.slide":     unformatted,
		"sub/b.slide": formatted,
	})
	a := filepath.Join(dir, "a.slide")

	var stdout, stderr bytes.Buffer
	if code := format([]string{a}, nil, &stdout, &stderr); code != 0 || stdout.String() != formatted {
		t.Errorf("fmt a.slide: exit code %d, output %q, error output %q; want 0 and %q", code, stdout.String(), stderr.String(), formatted)
	}

	stdout.Reset()
	if code := format([]string{"-l", "-r", dir}, nil, &stdout, &stderr); code != 0 || stdout.String() != a+"\n" {
		t.Errorf("fmt -l -r: exit code %d, output %q; want 0 and %q", code, stdout.String(), a+"\n")
	}

	stdout.Reset()
	if code := format([]string{"-d", a}, nil, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "-Text.  \n+\n+Text.\n") {
		t.Errorf("fmt -d: exit code %d, output %q; want a diff", code, stdout.String())
	}

	stdout.Reset()
	if code := format([]string{"-w", a}, nil, &stdout, &stderr); code != 0 || stdout.Len() != 0 {
		t.Errorf("fmt -w: exit code %d, output %q; want 0 and none", code, stdout.String())
	}
	if data, _ := os.ReadFile(a); string(data) != formatted {
		t.Errorf("fmt -w wrote %q, want %q", data, formatted)
	}

	stdout.Reset()
	if code := format(nil, strings.NewReader(unformatted), &stdout, &stderr); code != 0 || stdout.String() != formatted {
		t.Errorf("fmt of standard input: exit code %d, output %q; want 0 and %q", code, stdout.String(), formatted)
	}

	stderr.Reset()
	if code := format(nil, strings.NewReader("# T\n\n## S\n\n```\n"), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "<standard input>:5: fenced code block is not closed") {
		t.Errorf("fmt of an invalid file: exit code %d, error output %q; want 1 and the error", code, stderr.String())
	}
}
