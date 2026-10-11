package main

import (
	"bufio"
	"bytes"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/some-programs/present/pkg/present"
)

// sourceFileName returns the name that the source panel shows for the
// presentation file served at the URL path urlPath: its path in the content
// directory, or in the examples directory of the repository.
func sourceFileName(urlPath string) string {
	urlPath = path.Clean("/" + urlPath)
	if prefix, _ := contentRoot(urlPath); prefix == examplesPath {
		return "examples" + strings.TrimPrefix(urlPath, examplesPath)
	}
	return strings.TrimPrefix(urlPath, "/")
}

// A slideSource is the source of one slide of a presentation, which the
// slides show beside it with -doc when S is pressed.
type slideSource struct {
	Number int          // the number of the slide, from 1
	Text   string       // the lines of the slide in the presentation file
	Files  []sourceFile // the files that the slide's commands use
}

// A sourceLine is a line of a slideSource, with the class that marks what
// it is in present markup, if anything.
type sourceLine struct {
	Text, Class string
}

// Lines returns the lines of s.Text.
func (s slideSource) Lines() []sourceLine {
	var lines []sourceLine
	for line := range strings.SplitSeq(s.Text, "\n") {
		var class string
		switch {
		case strings.HasPrefix(line, "#"):
			class = "src-heading"
		case strings.HasPrefix(line, "."):
			class = "src-command"
		case strings.HasPrefix(line, ": "):
			class = "src-note"
		case strings.HasPrefix(line, "//"):
			class = "src-comment"
		}
		lines = append(lines, sourceLine{line, class})
	}
	return lines
}

// A sourceFile is a file, or a page, that a command of a slide uses.
type sourceFile struct {
	Name string // as written in the command
	URL  string // where it is served
	Text string // the contents of a text file, such as a program
}

// slideSources returns the sources of the slides of doc, parsed from src,
// the contents of docFile, which is served at the URL path urlPath: first
// the title slide, made from the header and authors, then one for each
// section.
func slideSources(doc *present.Doc, src []byte, docFile, urlPath string) ([]slideSource, error) {
	var lines []string
	s := bufio.NewScanner(bytes.NewReader(src))
	for s.Scan() {
		lines = append(lines, s.Text())
	}
	if err := s.Err(); err != nil {
		return nil, err
	}

	// Each slide runs up to the heading of the next one.
	starts := []int{0}
	for _, s := range doc.Sections {
		starts = append(starts, s.Line-1)
	}
	var sources []slideSource
	for i, start := range starts {
		end := len(lines)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		slide := lines[start:end]
		for len(slide) > 0 && strings.TrimSpace(slide[len(slide)-1]) == "" {
			slide = slide[:len(slide)-1]
		}
		sources = append(sources, slideSource{
			Number: i + 1,
			Text:   strings.Join(slide, "\n"),
			Files:  commandFiles(slide, docFile, urlPath),
		})
	}
	return sources, nil
}

// commandFiles returns the files that the commands in lines use. The
// contents of programs, included HTML and diagram sources are included.
func commandFiles(lines []string, docFile, urlPath string) []sourceFile {
	var files []sourceFile
	for _, line := range lines {
		if !strings.HasPrefix(line, ".") {
			continue
		}
		args := strings.Fields(line)
		var name string
		if len(args) > 1 {
			name = args[1]
		}
		var text bool
		switch args[0] {
		case ".code", ".play":
			// Flags such as -edit come before the file name.
			name = ""
			for _, a := range args[1:] {
				if !strings.HasPrefix(a, "-") {
					name = a
					break
				}
			}
			text = true
		case ".html":
			text = true
		case ".image":
			_, text = diagramTools[path.Ext(name)]
		case ".background", ".video", ".iframe":
		default:
			continue
		}
		if name == "" || slices.ContainsFunc(files, func(f sourceFile) bool { return f.Name == name }) {
			continue
		}
		f := sourceFile{Name: name, URL: name}
		if u, err := url.Parse(name); err != nil || u.Scheme != "" || u.Host != "" || strings.HasPrefix(u.Path, "/") {
			text = false
		} else {
			f.URL = path.Join(path.Dir(urlPath), name)
		}
		if text {
			if data, err := os.ReadFile(filepath.Join(filepath.Dir(docFile), filepath.FromSlash(name))); err == nil {
				f.Text = string(data)
			}
		}
		files = append(files, f)
	}
	return files
}
