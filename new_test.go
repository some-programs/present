package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/some-programs/present/pkg/present"
)

func TestNewCommands(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, tt := range []struct {
		args []string
		file string
	}{
		{[]string{"new-slide", "my-talk"}, "my-talk.slide"},
		{[]string{"new-article", "notes.article"}, "notes.article"},
		{[]string{"new-dot", "flow"}, "flow.dot"},
		{[]string{"new-mmd", "seq.mmd"}, "seq.mmd"},
	} {
		var stdout, stderr bytes.Buffer
		if code := runCommand(tt.args[0], tt.args[1:], &stdout, &stderr); code != 0 {
			t.Fatalf("%v: exit code %d: %s", tt.args, code, stderr.String())
		}
		data, err := os.ReadFile(tt.file)
		if err != nil {
			t.Fatalf("%v: %v", tt.args, err)
		}
		if !strings.Contains(stdout.String(), "Wrote "+tt.file+".") {
			t.Errorf("%v: output %q does not name %s", tt.args, stdout.String(), tt.file)
		}

		// A second run does not overwrite the file.
		stderr.Reset()
		if code := runCommand(tt.args[0], tt.args[1:], &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "already exists") {
			t.Errorf("%v again: exit code %d, %q; want 1 and already exists", tt.args, code, stderr.String())
		}

		switch tt.file {
		case "my-talk.slide", "notes.article":
			doc, err := present.Parse(bytes.NewReader(data), tt.file, 0)
			if err != nil {
				t.Errorf("%s does not parse: %v", tt.file, err)
				continue
			}
			if tt.file == "my-talk.slide" && doc.Title != "My talk" {
				t.Errorf("title %q, want My talk", doc.Title)
			}
			if doc.Time.IsZero() || len(doc.Authors) != 1 || len(doc.Sections) != 1 {
				t.Errorf("%s: want a date, an author and a section, got %+v", tt.file, doc)
			}
		default:
			if !strings.Contains(string(data), "Roboto") {
				t.Errorf("%s does not use Roboto:\n%s", tt.file, data)
			}
		}
	}

	var stdout, stderr bytes.Buffer
	if code := runCommand("new-slide", nil, &stdout, &stderr); code != 2 {
		t.Errorf("new-slide without a name: exit code %d, want 2", code)
	}
	if code := runCommand("bogus", nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "new-dot") {
		t.Errorf("unknown command: exit code %d, %q; want 2 and the list of commands", code, stderr.String())
	}
}
