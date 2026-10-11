package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"os/exec"
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
	// current directory, to standard output as SVG.
	args func(src string) []string
}

var (
	dotTool = &diagramTool{
		name: "dot",
		cmd:  dotCmd,
		args: func(src string) []string { return []string{"-Tsvg", src} },
	}
	mmdcTool = &diagramTool{
		name: "mmdc",
		cmd:  mmdcCmd,
		args: func(src string) []string {
			return []string{"--quiet", "--input", src, "--output", "-", "--outputFormat", "svg"}
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

// renderDiagrams renders the diagram images of doc, read from docFile, to
// SVG files next to their sources. It records any failure in the image, to
// be shown with the slide.
func renderDiagrams(doc *present.Doc, docFile string) {
	dir := filepath.Dir(docFile)
	paths := func(img *present.Image) (src, out string) {
		return filepath.Join(dir, filepath.FromSlash(img.Source)), filepath.Join(dir, filepath.FromSlash(img.URL))
	}

	// Diagram files with the same SVG file, such as g.dot and g.mmd, would
	// overwrite each other's renderings, so none of them is rendered.
	sources := make(map[string][]string)
	eachDiagram(doc, func(img *present.Image) {
		src, out := paths(img)
		if !slices.Contains(sources[out], src) {
			sources[out] = append(sources[out], src)
		}
	})

	eachDiagram(doc, func(img *present.Image) {
		src, out := paths(img)
		if srcs := sources[out]; len(srcs) > 1 {
			err := fmt.Errorf("%s: all render to %s", strings.Join(srcs, ", "), out)
			log.Print(err)
			img.Error = trimContentDirs(err.Error())
			img.Missing = true
			return
		}
		svg, err := renderDiagram(src, out)
		if err != nil {
			log.Print(err)
			img.Error = trimContentDirs(err.Error())
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

// renderDiagram renders the diagram file src to the SVG file out, with the
// tool for its extension, unless out already holds a rendering of the same
// source. The rendering ends with a comment holding a hash of the source,
// which is how it is recognized; out is left alone if rendering fails. It
// returns the contents of out, if any, even with an error.
func renderDiagram(src, out string) (svg []byte, err error) {
	diagramMu.Lock()
	defer diagramMu.Unlock()

	old, _ := os.ReadFile(out)
	tool := diagramTools[filepath.Ext(src)]
	if tool == nil {
		return old, fmt.Errorf("%s: no tool renders %s files", src, filepath.Ext(src))
	}
	source, err := os.ReadFile(src)
	if err != nil {
		return old, err
	}
	stamp := fmt.Appendf(nil, "<!-- present: %s sha256:%x -->\n", tool.name, sha256.Sum256(source))
	if bytes.Contains(old, stamp) {
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
	cmd := exec.CommandContext(ctx, *tool.cmd, tool.args(filepath.Base(src))...)
	cmd.Dir = filepath.Dir(src)
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
	if err := writeFile(out, svg); err != nil {
		return old, err
	}
	return svg, nil
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
