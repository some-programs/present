package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/some-programs/present/pkg/present"
)

var dotCmd = flag.String("dot", "dot", "Graphviz command that renders the .dot files of .image commands to SVG; empty shows their existing .svg files as they are")

// dotMu serializes rendering, since the slides and the notes window
// request a presentation at the same time.
var dotMu sync.Mutex

// renderDots renders the Graphviz images of doc, read from docFile, to SVG
// files next to their sources. It records any failure in the image, to be
// shown with the slide.
func renderDots(doc *present.Doc, docFile string) {
	for i := range doc.Sections {
		renderSectionDots(doc.Sections[i].Elem, docFile)
	}
}

func renderSectionDots(elems []present.Elem, docFile string) {
	for i, e := range elems {
		switch e := e.(type) {
		case present.Section:
			renderSectionDots(e.Elem, docFile)
		case present.Image:
			if e.Source == "" {
				continue
			}
			dir := filepath.Dir(docFile)
			src := filepath.Join(dir, filepath.FromSlash(e.Source))
			out := filepath.Join(dir, filepath.FromSlash(e.URL))
			svg, err := renderDot(src, out)
			if err != nil {
				log.Print(err)
				e.Error = strings.ReplaceAll(err.Error(), filepath.Clean(*contentPath)+string(filepath.Separator), "")
			}
			e.Missing = svg == nil
			if svg != nil {
				// A new URL for each rendering, so that browsers do not
				// show an earlier one from their cache.
				e.URL += fmt.Sprintf("?v=%.12x", sha256.Sum256(svg))
			}
			elems[i] = e
		}
	}
}

// renderDot renders the Graphviz file src to the SVG file out, unless out
// already holds a rendering of the same source. The rendering ends with a
// comment holding a hash of the source, which is how it is recognized; out
// is left alone if rendering fails. It returns the contents of out, if any,
// even with an error.
func renderDot(src, out string) (svg []byte, err error) {
	dotMu.Lock()
	defer dotMu.Unlock()

	old, _ := os.ReadFile(out)
	source, err := os.ReadFile(src)
	if err != nil {
		return old, err
	}
	stamp := fmt.Appendf(nil, "<!-- present: dot sha256:%x -->\n", sha256.Sum256(source))
	if bytes.Contains(old, stamp) {
		return old, nil
	}
	if *dotCmd == "" {
		if old == nil {
			return nil, fmt.Errorf("%s: no rendering, and rendering is turned off by -dot", src)
		}
		return old, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, *dotCmd, "-Tsvg", filepath.Base(src))
	cmd.Dir = filepath.Dir(src)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	svg, err = cmd.Output()
	if err != nil {
		if msg := bytes.TrimSpace(stderr.Bytes()); len(msg) > 0 {
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
