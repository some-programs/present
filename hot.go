package main

import (
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/some-programs/present/pkg/present"
)

// hotInterval is how often the files of a presentation open in a browser
// are checked for changes.
var hotInterval = 200 * time.Millisecond

// hotHandler sends a "change" server-sent event each time the presentation
// named by the doc parameter, or a file it uses, has changed. It waits for
// the files to stay the same for one check, since editors may save a file in
// several steps.
func (s *server) hotHandler(w http.ResponseWriter, r *http.Request) {
	root, name := s.file(r.URL.Query().Get("doc"))
	if !isDoc(name) {
		http.NotFound(w, r)
		return
	}
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	// Have the browser reconnect soon after the server restarts.
	if _, err := io.WriteString(w, "retry: 1000\n\n"); err != nil {
		return
	}
	if err := rc.Flush(); err != nil {
		return
	}

	files := s.hotFiles(root, name, nil)
	sent := stamps(root.fsys, files)
	prev := sent
	ticker := time.NewTicker(hotInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
		cur := stamps(root.fsys, files)
		if maps.Equal(cur, prev) && !maps.Equal(cur, sent) {
			// The presentation may now use other files. Look before
			// telling the browser, so that a file changed while it
			// reloads is not taken as already sent.
			files = s.hotFiles(root, name, files)
			cur = stamps(root.fsys, files)
			sent = cur
			if _, err := io.WriteString(w, "event: change\ndata: change\n\n"); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
		prev = cur
	}
}

// hotFiles returns the files in root that the presentation in the named
// file is made from: the file itself, the files it includes, and its images
// and videos. If the presentation does not parse, the result also holds the
// files in old, the result of an earlier call, so that they are still
// watched.
func (s *server) hotFiles(root contentRoot, name string, old []string) []string {
	files := []string{name}
	ctx := present.Context{ReadFile: func(name string) ([]byte, error) {
		// The present package joins names with filepath.
		name = filepath.ToSlash(name)
		files = append(files, name)
		return fs.ReadFile(root.fsys, name)
	}}
	var doc *present.Doc
	f, err := root.fsys.Open(name)
	if err == nil {
		doc, err = ctx.Parse(f, name, 0)
		f.Close()
	}
	if err != nil {
		files = append(files, old...)
	} else {
		for _, sec := range doc.Sections {
			files = s.sectionFiles(files, root, path.Dir(name), sec)
		}
	}
	slices.Sort(files)
	return slices.Compact(files)
}

// sectionFiles appends the images and videos of sec to files. Their URLs
// are relative to dir, the directory of the presentation in root.
func (s *server) sectionFiles(files []string, root contentRoot, dir string, sec present.Section) []string {
	files = s.appendURLFile(files, root, dir, sec.Background)
	for _, e := range sec.Elem {
		switch e := e.(type) {
		case present.Section:
			files = s.sectionFiles(files, root, dir, e)
		case present.Image:
			// The SVG made from a diagram file changes only with it.
			if e.Source != "" {
				files = s.appendURLFile(files, root, dir, e.Source)
			} else {
				files = s.appendURLFile(files, root, dir, e.URL)
			}
		case present.Video:
			files = s.appendURLFile(files, root, dir, e.URL)
		}
	}
	return files
}

// appendURLFile appends the file served at the URL ref to files, if it is
// served from root.
func (s *server) appendURLFile(files []string, root contentRoot, dir, ref string) []string {
	u, err := url.Parse(ref)
	if err != nil || ref == "" || u.Scheme != "" || u.Host != "" {
		return files
	}
	if strings.HasPrefix(u.Path, "/") {
		if r, name := s.file(u.Path); r.prefix == root.prefix {
			files = append(files, name)
		}
		return files
	}
	return append(files, path.Join(dir, u.Path))
}

// A stamp tells whether a file has changed. It is zero for a missing file.
type stamp struct {
	modTime int64
	size    int64
}

func stamps(fsys fs.FS, files []string) map[string]stamp {
	m := make(map[string]stamp, len(files))
	for _, name := range files {
		var s stamp
		if fi, err := fs.Stat(fsys, name); err == nil {
			s = stamp{fi.ModTime().UnixNano(), fi.Size()}
		}
		m[name] = s
	}
	return m
}
