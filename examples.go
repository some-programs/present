package main

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// examplesFS holds the example presentations, which -doc serves at
// examplesPath.
//
//go:embed all:examples
var examplesFS embed.FS

// examplesPath is the URL path that the examples are served at.
const examplesPath = "/_examples"

// examplesDir is the directory that the examples are served from with
// -doc. Presentations are read from files, and diagrams are rendered next
// to them, so the embedded examples are written out to it by
// initExamples.
var examplesDir string

// initExamples writes the embedded examples to a directory in the user's
// cache directory named by a hash of them, unless an earlier run already
// has, and sets examplesDir to it.
func initExamples() error {
	sub, err := fs.Sub(examplesFS, "examples")
	if err != nil {
		return err
	}
	sum, err := hashFS(sub)
	if err != nil {
		return err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	parent := filepath.Join(cache, "present")
	dir := filepath.Join(parent, fmt.Sprintf("examples-%.12x", sum))
	if _, err := os.Stat(dir); err == nil {
		examplesDir = dir
		return nil
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	// Write to a new directory and rename it into place, so that dir is
	// either complete or missing, also with several present servers
	// starting at once.
	tmp, err := os.MkdirTemp(parent, "examples-tmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := os.CopyFS(tmp, sub); err != nil {
		return err
	}
	if err := os.Rename(tmp, dir); err != nil {
		if _, err2 := os.Stat(dir); err2 != nil {
			return err
		}
	}
	examplesDir = dir
	return nil
}

// hashFS returns a hash of the names and contents of the files in fsys.
func hashFS(fsys fs.FS) ([]byte, error) {
	h := sha256.New()
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%q %d\n", name, len(data))
		h.Write(data)
		return nil
	})
	return h.Sum(nil), err
}

// contentRoot returns the directory that the URL path p is served from:
// examplesDir for the examples with -doc, otherwise the content directory.
// The files in the directory are served at prefix.
func contentRoot(p string) (prefix, dir string) {
	if *showDoc && (p == examplesPath || strings.HasPrefix(p, examplesPath+"/")) {
		return examplesPath, examplesDir
	}
	return "", *contentPath
}

// contentFile returns the name of the file served at the URL path p.
func contentFile(p string) string {
	p = path.Clean("/" + p)
	prefix, dir := contentRoot(p)
	return filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(p, prefix)))
}

// trimContentDirs removes the directories that content is served from from
// the file names in s, such as an error message shown to authors.
func trimContentDirs(s string) string {
	s = strings.ReplaceAll(s, filepath.Clean(*contentPath)+string(filepath.Separator), "")
	if examplesDir != "" {
		s = strings.ReplaceAll(s, examplesDir+string(filepath.Separator), "")
	}
	return s
}
