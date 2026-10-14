package main

import (
	"embed"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/some-programs/present/pkg/present"
)

// examplesFS holds the example presentations, which -doc serves at
// examplesPath.
//
//go:embed all:examples
var examplesFS embed.FS

// examplesPath is the URL path that the examples are served at.
const examplesPath = "/_examples"

// examplesRoot is the root that the examples are served from with -doc.
// Their diagrams are not rendered, so they come with their SVG files.
var examplesRoot = contentRoot{prefix: examplesPath, fsys: must(fs.Sub(examplesFS, "examples"))}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// A contentRoot is a tree of files that presentations are served from.
// The files are named by slash-separated paths in fsys, as fs.FS has it.
type contentRoot struct {
	prefix string // the URL path the files are served at, "" for the root
	fsys   fs.FS
	// dir is the directory that fsys reads, which diagrams are rendered
	// to, or "" if fsys is read-only, as the embedded examples are.
	dir string
}

// dirRoot returns the root of the files in the directory dir.
func dirRoot(dir string) contentRoot {
	return contentRoot{fsys: os.DirFS(dir), dir: dir}
}

// contentFile returns the root that the URL path p is served from, the
// examples with -doc or otherwise the content directory, and the name of
// the file served at p in it.
func contentFile(p string) (contentRoot, string) {
	p = path.Clean("/" + p)
	root := dirRoot(*contentPathFlag)
	if *showDoc && (p == examplesPath || strings.HasPrefix(p, examplesPath+"/")) {
		root = examplesRoot
	}
	name := strings.TrimPrefix(strings.TrimPrefix(p, root.prefix), "/")
	if name == "" {
		name = "."
	}
	return root, name
}

// parse parses the presentation read from r, the file name in fsys, with
// the files it includes read from fsys.
func parse(fsys fs.FS, r io.Reader, name string, mode present.ParseMode) (*present.Doc, error) {
	ctx := present.Context{ReadFile: func(name string) ([]byte, error) {
		// The present package joins names with filepath.
		return fs.ReadFile(fsys, filepath.ToSlash(name))
	}}
	return ctx.Parse(r, name, mode)
}

// parseFile parses the presentation file name in fsys.
func parseFile(fsys fs.FS, name string, mode present.ParseMode) (*present.Doc, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parse(fsys, f, name, mode)
}
