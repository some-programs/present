package main

import (
	"bytes"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/some-programs/present/pkg/present"
)

// TestSkeletons checks that each command, and each of its types, has a
// skeleton, and that each skeleton belongs to a command.
func TestSkeletons(t *testing.T) {
	files, err := fs.Glob(skeletonFS, "skeletons/*")
	if err != nil {
		t.Fatal(err)
	}
	unused := make(map[string]bool)
	for _, file := range files {
		unused[path.Base(file)] = true
	}
	for _, c := range newCommands {
		types := c.types
		if len(types) == 0 {
			types = []string{""}
		}
		for _, typ := range types {
			for _, mono := range []bool{false, true} {
				if _, err := c.skeleton("Title", newOptions{mono: mono, typ: typ}); err != nil {
					t.Errorf("%s -type %q -mono=%v: %v", c.name, typ, mono, err)
				}
			}
			name := strings.TrimPrefix(c.name, "new-")
			if typ != "" {
				name += "-" + typ
			}
			delete(unused, name+".tmpl")
		}
	}
	for file := range unused {
		t.Errorf("skeletons/%s belongs to no command", file)
	}
}

// TestSkeletonFonts checks the fonts that the skeletons can use: the main
// and other fonts, which -mono swaps, and the fonts of text and code, which
// it does not.
func TestSkeletonFonts(t *testing.T) {
	for _, tt := range []struct {
		mono        bool
		font, other string
	}{
		{false, textFont, codeFont},
		{true, codeFont, textFont},
	} {
		d := newSkeletonData("", newOptions{mono: tt.mono})
		if d.Font != tt.font || d.OtherFont != tt.other {
			t.Errorf("-mono=%v: Font %q, OtherFont %q; want %q, %q", tt.mono, d.Font, d.OtherFont, tt.font, tt.other)
		}
		if d.TextFont != textFont || d.CodeFont != codeFont {
			t.Errorf("-mono=%v: TextFont %q, CodeFont %q; want %q, %q", tt.mono, d.TextFont, d.CodeFont, textFont, codeFont)
		}
	}
}

// TestMmdSkeleton checks that each type of Mermaid diagram starts with its
// keyword, and that only a flowchart has the classes text and code, with
// a node of class code.
func TestMmdSkeleton(t *testing.T) {
	i := slices.IndexFunc(newCommands, func(c newCommand) bool { return c.name == "new-mmd" })
	mmd := newCommands[i]
	for _, tt := range []struct {
		typ, keyword string
	}{
		{"flowchart", "flowchart LR"},
		{"sequence", "sequenceDiagram"},
		{"class", "classDiagram"},
		{"state", "stateDiagram-v2"},
	} {
		for _, mono := range []bool{false, true} {
			got, err := mmd.skeleton("", newOptions{mono: mono, typ: tt.typ})
			if err != nil {
				t.Fatal(err)
			}
			main := textFont
			if mono {
				main = codeFont
			}
			if !strings.Contains(got, "fontFamily: "+main+"\n---\n"+tt.keyword+"\n") {
				t.Errorf("type %s, mono %v: want font %s and keyword %s:\n%s", tt.typ, mono, main, tt.keyword, got)
			}
			for _, def := range []string{
				`classDef text font-family:Roboto\,sans-serif`,
				`classDef code font-family:JetBrains Mono\,monospace`,
			} {
				if has := strings.Contains(got, def+"\n"); has != (tt.typ == "flowchart") {
					t.Errorf("type %s, mono %v: has %s: %v:\n%s", tt.typ, mono, def, has, got)
				}
			}
			if tt.typ == "flowchart" && !strings.Contains(got, `b["world"]:::code`+"\n") {
				t.Errorf("mono %v: b is not of class code:\n%s", mono, got)
			}
		}
	}
}

func TestNewCommands(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, tt := range []struct {
		args []string
		file string
		font string // the main font of a diagram
	}{
		{[]string{"new-slide", "my-talk"}, "my-talk.slide", ""},
		{[]string{"new-article", "notes.article"}, "notes.article", ""},
		{[]string{"new-dot", "flow"}, "flow.dot", textFont},
		{[]string{"new-mmd", "seq.mmd"}, "seq.mmd", textFont},
		{[]string{"new-dot", "-mono", "code"}, "code.dot", codeFont},
		{[]string{"new-mmd", "-mono", "code.mmd"}, "code.mmd", codeFont},
		{[]string{"new-mmd", "-type", "sequence", "login"}, "login.mmd", textFont},
		{[]string{"new-mmd", "-type", "class", "-mono", "types"}, "types.mmd", codeFont},
		{[]string{"new-mmd", "-type=state", "parser"}, "parser.mmd", textFont},
		{[]string{"new-puml", "calls"}, "calls.puml", textFont},
		{[]string{"new-puml", "-mono", "-type", "class", "objects.puml"}, "objects.puml", codeFont},
		{[]string{"new-puml", "-type", "activity", "steps"}, "steps.puml", textFont},
		{[]string{"new-puml", "-type", "state", "-mono", "machine"}, "machine.puml", codeFont},
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
		case "flow.dot", "code.dot":
			if !strings.Contains(string(data), `fontname="`+tt.font+`"`) {
				t.Errorf("%v: %s does not use %s:\n%s", tt.args, tt.file, tt.font, data)
			}
		case "calls.puml", "objects.puml", "steps.puml", "machine.puml":
			if !strings.Contains(string(data), "skinparam defaultFontName "+tt.font+"\n") {
				t.Errorf("%v: %s does not use %s:\n%s", tt.args, tt.file, tt.font, data)
			}
		default:
			if !strings.Contains(string(data), "fontFamily: "+tt.font+"\n") {
				t.Errorf("%v: %s does not use %s:\n%s", tt.args, tt.file, tt.font, data)
			}
		}
	}

	var stdout, stderr bytes.Buffer
	if code := runCommand("new-slide", nil, &stdout, &stderr); code != 2 {
		t.Errorf("new-slide without a name: exit code %d, want 2", code)
	}
	if code := runCommand("new-slide", []string{"-mono", "talk"}, &stdout, &stderr); code != 2 {
		t.Errorf("new-slide -mono: exit code %d, want 2", code)
	}
	stderr.Reset()
	if code := runCommand("new-mmd", []string{"-type", "pie", "chart"}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "flowchart, sequence") {
		t.Errorf("new-mmd -type pie: exit code %d, %q; want 2 and the list of types", code, stderr.String())
	}
	if _, err := os.Stat("chart.mmd"); err == nil {
		t.Errorf("new-mmd -type pie wrote chart.mmd")
	}
	if code := runCommand("bogus", nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "new-dot") {
		t.Errorf("unknown command: exit code %d, %q; want 2 and the list of commands", code, stderr.String())
	}
}
