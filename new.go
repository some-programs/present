package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
	"time"
	"unicode"
	"unicode/utf8"
)

// A newCommand is a subcommand that writes a new file from a skeleton,
// with present's defaults, such as the fonts of diagrams. The skeleton is
// the template skeletons/NAME.tmpl, where NAME is the command's name
// without "new-", followed by "-TYPE" for a command with types.
type newCommand struct {
	name string
	ext  string // added to the file name if it does not end in it
	help string
	// mono reports whether the command takes the -mono flag, for a diagram
	// in the font of code.
	mono bool
	// types are the values of the -type flag, the default first. A command
	// without types takes no -type.
	types []string
	// next tells how to use the file name, after it is written.
	next func(name string) string
}

// newOptions are the flags of a newCommand.
type newOptions struct {
	mono bool   // -mono
	typ  string // -type, or "" if the command has no types
}

var newCommands = []newCommand{
	{
		name: "new-slide",
		ext:  ".slide",
		help: "write a new slide presentation",
		next: runPresent,
	},
	{
		name: "new-article",
		ext:  ".article",
		help: "write a new article",
		next: runPresent,
	},
	{
		name: "new-dot",
		ext:  ".dot",
		help: "write a new Graphviz diagram that uses the fonts of the slides",
		mono: true,
		next: showImage,
	},
	{
		name:  "new-mmd",
		ext:   ".mmd",
		help:  "write a new Mermaid diagram that uses the fonts of the slides",
		mono:  true,
		types: []string{"flowchart", "sequence", "class", "state"},
		next:  showImage,
	},
}

// The fonts of the slides, for diagrams: textFont for text and codeFont for
// code.
const (
	textFont = "Roboto,sans-serif"
	codeFont = "JetBrains Mono,monospace"
)

//go:embed skeletons
var skeletonFS embed.FS

var skeletons = template.Must(template.New("").Funcs(template.FuncMap{
	// escapeCommas escapes the commas in a font for a style of a Mermaid
	// class, which a comma would end.
	"escapeCommas": func(s string) string { return strings.ReplaceAll(s, ",", `\,`) },
}).ParseFS(skeletonFS, "skeletons/*.tmpl"))

// skeletonData is what the skeletons are executed with.
type skeletonData struct {
	Title string
	Date  string
	// Font is the main font of a diagram, and OtherFont the other font of
	// the slides. -mono swaps them.
	Font, OtherFont string
	// TextFont and CodeFont are the fonts of text and code, for a part of
	// a skeleton that uses one of them whatever -mono says.
	TextFont, CodeFont string
}

func newSkeletonData(title string, opts newOptions) skeletonData {
	data := skeletonData{
		Title:     title,
		Date:      time.Now().Format("2 Jan 2006"),
		Font:      textFont,
		OtherFont: codeFont,
		TextFont:  textFont,
		CodeFont:  codeFont,
	}
	if opts.mono {
		data.Font, data.OtherFont = codeFont, textFont
	}
	return data
}

// skeleton returns the contents of a new file, given a title made from its
// name.
func (c newCommand) skeleton(title string, opts newOptions) (string, error) {
	name := strings.TrimPrefix(c.name, "new-")
	if opts.typ != "" {
		name += "-" + opts.typ
	}
	var b strings.Builder
	if err := skeletons.ExecuteTemplate(&b, name+".tmpl", newSkeletonData(title, opts)); err != nil {
		return "", err
	}
	return b.String(), nil
}

func runPresent(string) string { return "Run present in its directory to show it." }

func showImage(name string) string { return "Show it on a slide with .image " + name }

func (c newCommand) run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(c.name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	var opts newOptions
	usage := "present " + c.name
	if c.mono {
		flags.BoolVar(&opts.mono, "mono", false, "use a monospaced font rather than a sans-serif one")
		usage += " [-mono]"
	}
	if len(c.types) > 0 {
		flags.StringVar(&opts.typ, "type", c.types[0], "the type of diagram: "+strings.Join(c.types, ", "))
		usage += " [-type type]"
	}
	usage += fmt.Sprintf(" name[%s]", c.ext)
	flags.Usage = func() {
		fmt.Fprintf(stderr, "usage: %s\n\n%s, named name%s.\n", usage, capitalize(c.help), c.ext)
		if c.mono || len(c.types) > 0 {
			fmt.Fprintln(stderr)
			flags.PrintDefaults()
		}
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
	if len(c.types) > 0 && !slices.Contains(c.types, opts.typ) {
		fmt.Fprintf(stderr, "present: unknown -type %q, want one of: %s\n", opts.typ, strings.Join(c.types, ", "))
		return 2
	}
	name := flags.Arg(0)
	if filepath.Ext(name) != c.ext {
		name += c.ext
	}
	data, err := c.skeleton(titleFromName(name), opts)
	if err == nil {
		err = writeNew(name, data)
	}
	if err != nil {
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
