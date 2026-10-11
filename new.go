package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// A newCommand is a subcommand that writes a new file from a skeleton,
// with present's defaults, such as the fonts of diagrams.
type newCommand struct {
	name string
	ext  string // added to the file name if it does not end in it
	help string
	// skeleton returns the contents of the file, given a title made from
	// its name.
	skeleton func(title string) string
	// next tells how to use the file name, after it is written.
	next func(name string) string
}

var newCommands = []newCommand{
	{
		name:     "new-slide",
		ext:      ".slide",
		help:     "write a new slide presentation",
		skeleton: func(title string) string { return docSkeleton(title, "First slide") },
		next:     runPresent,
	},
	{
		name:     "new-article",
		ext:      ".article",
		help:     "write a new article",
		skeleton: func(title string) string { return docSkeleton(title, "First section") },
		next:     runPresent,
	},
	{
		name: "new-dot",
		ext:  ".dot",
		help: "write a new Graphviz diagram that uses the fonts of the slides",
		skeleton: func(string) string {
			return `digraph {
	fontname="Roboto,sans-serif"
	node [fontname="Roboto,sans-serif"]
	edge [fontname="Roboto,sans-serif"]
	bgcolor=transparent

	a -> b
}
`
		},
		next: showImage,
	},
	{
		name: "new-mmd",
		ext:  ".mmd",
		help: "write a new Mermaid diagram that uses the fonts of the slides",
		skeleton: func(string) string {
			return `---
config:
  fontFamily: Roboto, sans-serif
---
flowchart LR
    a --> b
`
		},
		next: showImage,
	},
}

func runPresent(string) string { return "Run present in its directory to show it." }

func showImage(name string) string { return "Show it on a slide with .image " + name }

// docSkeleton returns a presentation or article with the given title,
// dated today, with one slide or section.
func docSkeleton(title, first string) string {
	return fmt.Sprintf(`# %s
%s

Your Name

## %s

Text in Markdown.
`, title, time.Now().Format("2 Jan 2006"), first)
}

func (c newCommand) run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(c.name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintf(stderr, "usage: present %s name[%s]\n\n%s, named name%s.\n", c.name, c.ext, capitalize(c.help), c.ext)
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}
	name := flags.Arg(0)
	if filepath.Ext(name) != c.ext {
		name += c.ext
	}
	if err := writeNew(name, c.skeleton(titleFromName(name))); err != nil {
		fmt.Fprintf(stderr, "present: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Wrote %s. %s\n", name, c.next(filepath.ToSlash(name)))
	return 0
}

// writeNew writes data to a new file name, and fails if it exists.
func writeNew(name string, data string) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%s already exists", name)
	}
	if err != nil {
		return err
	}
	_, err = io.WriteString(f, data)
	if err2 := f.Close(); err == nil {
		err = err2
	}
	return err
}

// titleFromName returns a title for the file name, such as "My talk" for
// my-talk.slide.
func titleFromName(name string) string {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	title := strings.TrimSpace(strings.NewReplacer("-", " ", "_", " ").Replace(base))
	if title == "" {
		return "Title"
	}
	return capitalize(title)
}

func capitalize(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}
