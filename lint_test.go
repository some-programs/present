package main

import (
	"bytes"
	"encoding/json/v2"
	"path/filepath"
	"strings"
	"testing"

	"github.com/some-programs/present/pkg/present"
)

func TestLintCommand(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"good.slide":    formatted,
		"sub/bad.slide": formatted + "\n.image missing.png\n",
		"_skip/x.slide": unformatted,
	})
	bad := filepath.Join(dir, "sub", "bad.slide")

	var stdout, stderr bytes.Buffer
	if code := lint([]string{filepath.Join(dir, "good.slide")}, &stdout, &stderr); code != 0 || stdout.Len() != 0 {
		t.Errorf("lint good.slide: exit code %d, output %q, error output %q; want 0 and none", code, stdout.String(), stderr.String())
	}

	want := bad + ":7: missing.png does not exist (missing-file)\n"
	if code := lint([]string{"-r", dir}, &stdout, &stderr); code != 1 || stdout.String() != want {
		t.Errorf("lint -r: exit code %d, output %q, error output %q; want 1 and %q", code, stdout.String(), stderr.String(), want)
	}

	stdout.Reset()
	if code := lint([]string{"-json", "-r", dir}, &stdout, &stderr); code != 1 {
		t.Errorf("lint -json: exit code %d, want 1", code)
	}
	var problems []present.Problem
	if err := json.Unmarshal(stdout.Bytes(), &problems); err != nil {
		t.Fatalf("lint -json: %v in %q", err, stdout.String())
	}
	if len(problems) != 1 || problems[0].File != bad || problems[0].Line != 7 || problems[0].Rule != "missing-file" || problems[0].Severity != present.Error {
		t.Errorf("lint -json: got %+v, want the missing file", problems)
	}

	stdout.Reset()
	if code := lint([]string{"-json", filepath.Join(dir, "good.slide")}, &stdout, &stderr); code != 0 || strings.TrimSpace(stdout.String()) != "[]" {
		t.Errorf("lint -json good.slide: exit code %d, output %q; want 0 and []", code, stdout.String())
	}
}
