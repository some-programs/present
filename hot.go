package main

import (
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/some-programs/present/pkg/present"
)

var hot = serveFlags.Bool("hot", false, "reload presentations in the browser when they or the files they use change (not allowed with -notes)")

// hotInterval is how often the files of a presentation open in a browser
// are checked for changes.
var hotInterval = 200 * time.Millisecond

// initHot registers the handler that static/js/hot.js listens to, if -hot
// is set.
func initHot() {
	if !*hot {
		return
	}
	http.HandleFunc("/_hot", hotHandler)
}

// hotHandler sends a "change" server-sent event each time the presentation
// named by the doc parameter, or a file it uses, has changed. It waits for
// the files to stay the same for one check, since editors may save a file in
// several steps.
func hotHandler(w http.ResponseWriter, r *http.Request) {
	name := contentFile(r.URL.Query().Get("doc"))
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

	files := hotFiles(name, nil)
	sent := stamps(files)
	prev := sent
	ticker := time.NewTicker(hotInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
		cur := stamps(files)
		if maps.Equal(cur, prev) && !maps.Equal(cur, sent) {
			// The presentation may now use other files. Look before
			// telling the browser, so that a file changed while it
			// reloads is not taken as already sent.
			files = hotFiles(name, files)
			cur = stamps(files)
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

// hotFiles returns the files that the presentation in the named file is made
// from: the file itself, the files it includes, and its images and videos.
// If the presentation does not parse, the result also holds the files in
// old, the result of an earlier call, so that they are still watched.
func hotFiles(name string, old []string) []string {
	files := []string{name}
	ctx := present.Context{ReadFile: func(name string) ([]byte, error) {
		files = append(files, name)
		return os.ReadFile(name)
	}}
	var doc *present.Doc
	f, err := os.Open(name)
	if err == nil {
		doc, err = ctx.Parse(f, name, 0)
		f.Close()
	}
	if err != nil {
		files = append(files, old...)
	} else {
		for _, s := range doc.Sections {
			files = sectionFiles(files, filepath.Dir(name), s)
		}
	}
	slices.Sort(files)
	return slices.Compact(files)
}

// sectionFiles appends the images and videos of s to files. Their URLs are
// relative to dir, the directory of the presentation.
func sectionFiles(files []string, dir string, s present.Section) []string {
	files = appendURLFile(files, dir, s.Background)
	for _, e := range s.Elem {
		switch e := e.(type) {
		case present.Section:
			files = sectionFiles(files, dir, e)
		case present.Image:
			// The SVG made from a diagram file changes only with it.
			if e.Source != "" {
				files = appendURLFile(files, dir, e.Source)
			} else {
				files = appendURLFile(files, dir, e.URL)
			}
		case present.Video:
			files = appendURLFile(files, dir, e.URL)
		}
	}
	return files
}

// appendURLFile appends the file served at the URL ref to files, if it is
// served from the content directory.
func appendURLFile(files []string, dir, ref string) []string {
	u, err := url.Parse(ref)
	if err != nil || ref == "" || u.Scheme != "" || u.Host != "" {
		return files
	}
	if strings.HasPrefix(u.Path, "/") {
		return append(files, contentFile(u.Path))
	}
	return append(files, filepath.Join(dir, filepath.FromSlash(u.Path)))
}

// A stamp tells whether a file has changed. It is zero for a missing file.
type stamp struct {
	modTime int64
	size    int64
}

func stamps(files []string) map[string]stamp {
	m := make(map[string]stamp, len(files))
	for _, name := range files {
		var s stamp
		if fi, err := os.Stat(name); err == nil {
			s = stamp{fi.ModTime().UnixNano(), fi.Size()}
		}
		m[name] = s
	}
	return m
}
