package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/some-programs/present/pkg/present"
)

var (
	noRender = serveFlags.Bool("norender", false, "do not render the diagram files (.dot, .mmd, .mermaid) of .image commands; show their existing .svg files as they are")

	dotCmd  = serveFlags.String("dot", "dot", "Graphviz command that renders the .dot files of .image commands to SVG; empty shows their existing .svg files as they are")
	mmdcCmd = serveFlags.String("mmdc", "mmdc", "Mermaid CLI command that renders the .mmd and .mermaid files of .image commands to SVG; empty shows their existing .svg files as they are")
)

// A diagramTool renders diagram files of one kind to SVG.
type diagramTool struct {
	// name is the name of the flag that sets cmd, also used in the stamp
	// that marks its renderings.
	name string
	// cmd is the command to run; empty turns rendering off.
	cmd *string
	// args returns the arguments that make cmd write the file src, in the
	// current directory, to standard output as SVG, with the background
	// color bg, if not empty.
	args func(src, bg string) []string
}

var (
	dotTool = &diagramTool{
		name: "dot",
		cmd:  dotCmd,
		args: func(src, _ string) []string { return []string{"-Tsvg", src} },
	}
	mmdcTool = &diagramTool{
		name: "mmdc",
		cmd:  mmdcCmd,
		args: func(src, bg string) []string {
			args := []string{"--quiet", "--input", src, "--output", "-", "--outputFormat", "svg"}
			if bg != "" {
				args = append(args, "--backgroundColor", bg)
			}
			return args
		},
	}
)

// diagramTools maps the extensions of diagram files to their tools.
var diagramTools = map[string]*diagramTool{
	".dot":     dotTool,
	".mmd":     mmdcTool,
	".mermaid": mmdcTool,
}

// diagramMu serializes rendering, since the slides and the notes window
// request a presentation at the same time.
var diagramMu sync.Mutex

// renderDiagrams renders the diagram images of doc, read from docFile in
// root, to SVG files next to their sources. It records any failure in the
// image, to be shown with the slide.
func renderDiagrams(doc *present.Doc, root contentRoot, docFile string) {
	dir := path.Dir(docFile)
	paths := func(img *present.Image) (src, out string) {
		return path.Join(dir, img.Source), path.Join(dir, img.URL)
	}

	// Different renderings to the same SVG file, such as of g.dot and
	// g.mmd, would overwrite each other, so none of them is made.
	renderings := make(map[string][]string)
	eachDiagram(doc, func(img *present.Image) {
		src, out := paths(img)
		r := src
		if img.BackgroundColor != "" {
			r = "-background-color " + img.BackgroundColor + " " + src
		}
		if !slices.Contains(renderings[out], r) {
			renderings[out] = append(renderings[out], r)
		}
	})

	eachDiagram(doc, func(img *present.Image) {
		src, out := paths(img)
		if rs := renderings[out]; len(rs) > 1 {
			err := fmt.Errorf("%s: all render to %s", strings.Join(rs, ", "), out)
			log.Print(err)
			img.Error = err.Error()
			img.Missing = true
			return
		}
		svg, err := renderDiagram(root, src, out, img.BackgroundColor)
		if err != nil {
			log.Print(err)
			img.Error = err.Error()
		}
		img.Missing = svg == nil
		if svg != nil {
			// A new URL for each rendering, so that browsers do not
			// show an earlier one from their cache.
			img.URL += fmt.Sprintf("?v=%.12x", sha256.Sum256(svg))
		}
	})
}

// eachDiagram calls f for each image of doc that is made from a diagram
// file, in order, keeping any changes f makes to it.
func eachDiagram(doc *present.Doc, f func(*present.Image)) {
	for i := range doc.Sections {
		eachSectionDiagram(doc.Sections[i].Elem, f)
	}
}

func eachSectionDiagram(elems []present.Elem, f func(*present.Image)) {
	for i, e := range elems {
		switch e := e.(type) {
		case present.Section:
			eachSectionDiagram(e.Elem, f)
		case present.Image:
			if e.Source != "" {
				f(&e)
				elems[i] = e
			}
		}
	}
}

// renderDiagram renders the diagram file src in root to the SVG file out,
// with the tool for its extension and the background color bg, if not
// empty, unless out already holds the same rendering of the same source.
// The rendering ends with a comment holding bg and a hash of the source,
// which is how it is recognized; out is left alone if rendering fails. It
// returns the contents of out, if any, even with an error. Diagrams in a
// read-only root are not rendered, and their SVG files are used as they
// are.
func renderDiagram(root contentRoot, src, out, bg string) (svg []byte, err error) {
	diagramMu.Lock()
	defer diagramMu.Unlock()

	old, _ := fs.ReadFile(root.fsys, out)
	tool := diagramTools[path.Ext(src)]
	if tool == nil {
		return old, fmt.Errorf("%s: no tool renders %s files", src, path.Ext(src))
	}
	source, err := fs.ReadFile(root.fsys, src)
	if err != nil {
		return old, err
	}
	stamp := diagramStamp(tool, bg, source)
	if bytes.Contains(old, stamp) {
		return old, nil
	}
	if root.dir == "" {
		if old == nil {
			return nil, fmt.Errorf("%s: no rendering, and diagrams in read-only content are not rendered", src)
		}
		return old, nil
	}
	if off := offFlag(tool); off != "" {
		if old == nil {
			return nil, fmt.Errorf("%s: no rendering, and automatic rendering is turned off by %s", src, off)
		}
		return old, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, *tool.cmd, tool.args(path.Base(src), bg)...)
	cmd.Dir = filepath.Join(root.dir, filepath.FromSlash(path.Dir(src)))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	svg, err = cmd.Output()
	if err != nil {
		if msg := errorMessage(stderr.Bytes()); len(msg) > 0 {
			return old, fmt.Errorf("%s: %s", src, msg)
		}
		return old, fmt.Errorf("%s: %v", src, err)
	}
	svg = append(svg, stamp...)
	if err := writeFile(filepath.Join(root.dir, filepath.FromSlash(out)), svg); err != nil {
		return old, err
	}
	return svg, nil
}

// diagramStamp returns the comment that ends the rendering of source with
// tool and the background color bg.
func diagramStamp(tool *diagramTool, bg string, source []byte) []byte {
	var opts string
	if bg != "" {
		opts = " background-color=" + bg
	}
	return fmt.Appendf(nil, "<!-- present: %s%s sha256:%x -->\n", tool.name, opts, sha256.Sum256(source))
}

// offFlag returns the flag setting that turns automatic rendering with
// tool off, if any.
func offFlag(tool *diagramTool) string {
	switch {
	case *noRender:
		return "-norender"
	case *tool.cmd == "":
		return "-" + tool.name
	}
	return ""
}

// stackFrame matches a line of a JavaScript stack trace, such as mmdc
// prints after its error messages:
//
//	    at async renderMermaid (file:///…/index.js:689:22)
//	Parser.parse (https://…/chunk.mjs:1633:16)
var stackFrame = regexp.MustCompile(`(?m)^\s*(at .*|\S+ \(\S+:\d+:\d+\))$`)

// errorMessage returns the error output of a tool without any stack trace.
func errorMessage(stderr []byte) []byte {
	if loc := stackFrame.FindIndex(stderr); loc != nil {
		stderr = stderr[:loc[0]]
	}
	return bytes.TrimSpace(stderr)
}

// writeFile writes data to the named file through a temporary file in the
// same directory, so that the file never holds partial data.
func writeFile(name string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(name), "."+filepath.Base(name)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err1 := f.Close(); err == nil {
		err = err1
	}
	if err == nil {
		err = os.Chmod(f.Name(), 0o644)
	}
	if err == nil {
		err = os.Rename(f.Name(), name)
	}
	return err
}
