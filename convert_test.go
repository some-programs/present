package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvertLegacy(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	const legacyTalk = "Title\n\n* Slide\n\nOne\ntwo\n"
	const markdownTalk = "# Title\n\n## Slide\n\nOne\\\ntwo\n"
	files := map[string]string{
		"legacy.slide":   legacyTalk,
		"markdown.slide": markdownTalk,
	}
	for name, data := range files {
		if err := os.WriteFile(name, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, tt := range []struct {
		args       []string
		code       int
		out, inErr string // the output and text the error output must contain
	}{
		{[]string{"legacy.slide"}, 0, markdownTalk, ""},
		{[]string{"markdown.slide"}, 1, "", "already in the Markdown markup"},
		{[]string{"missing.slide"}, 1, "", "missing.slide"},
		{[]string{"legacy.slide", "markdown.slide"}, 2, "", "usage:"},
		{[]string{}, 2, "", "usage:"},
	} {
		var stdout, stderr bytes.Buffer
		code := runCommand("convert-legacy", tt.args, &stdout, &stderr)
		if code != tt.code || stdout.String() != tt.out || !strings.Contains(stderr.String(), tt.inErr) {
			t.Errorf("present convert-legacy %s: exit code %d, output %q, error output %q; want %d, %q and %q",
				strings.Join(tt.args, " "), code, stdout.String(), stderr.String(), tt.code, tt.out, tt.inErr)
		}
	}

	var stdout, stderr bytes.Buffer
	code := runCommand("convert-legacy", []string{"-w", "legacy.slide", "markdown.slide"}, &stdout, &stderr)
	const wantOut = "Converted legacy.slide.\nSkipped markdown.slide, already in Markdown.\n"
	if code != 0 || stdout.String() != wantOut || stderr.Len() > 0 {
		t.Errorf("present convert-legacy -w: exit code %d, output %q, error output %q; want 0, %q and none",
			code, stdout.String(), stderr.String(), wantOut)
	}
	for _, name := range []string{"legacy.slide", "markdown.slide"} {
		data, err := os.ReadFile(filepath.Join(tmp, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != markdownTalk {
			t.Errorf("%s after -w = %q, want %q", name, data, markdownTalk)
		}
	}
}
