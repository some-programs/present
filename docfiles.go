package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// docFiles returns the presentation files that paths name, as given on the
// command line. A directory is an error, unless recursive is set: then it
// stands for the slide presentations and articles in it and the
// directories below it, but for those whose names start with . or _,
// which serve does not list either.
func docFiles(paths []string, recursive bool) ([]string, error) {
	var files []string
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			files = append(files, p)
			continue
		}
		if !recursive {
			return nil, fmt.Errorf("%s is a directory; use -r for the presentations in it", p)
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != p && !showDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			switch filepath.Ext(path) {
			case ".slide", ".article":
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}
