package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/some-programs/present/pkg/present"
)

// exportSharedFlags are the flags of serve that export takes too, setting
// the same variables.
var exportSharedFlags = []string{"thankyou", "norender"}

// export runs the export command, which writes presentations as static
// HTML pages.
func export(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("export", flag.ContinueOnError)
	flags.SetOutput(stderr)
	out := flags.String("o", "present.export", "directory to write the pages to")
	source := flags.Bool("source", false, "include the source of each slide, which pressing S shows beside it, and the files that its commands use")
	for _, name := range exportSharedFlags {
		f := serveFlags.Lookup(name)
		flags.Var(f.Value, f.Name, f.Usage)
	}
	flags.Usage = func() {
		fmt.Fprintf(stderr, "usage: present export [flags] file...\n\n"+
			"Export writes each slide presentation or article as a static HTML page\n"+
			"in the -o directory, at its path under the current directory with the\n"+
			"extension .html, along with the files that it shows and the style\n"+
			"sheets, scripts and fonts of the pages, to open from files or serve\n"+
			"with any web server. Code cannot be run on the pages, and they have no\n"+
			"presenter notes, though with -source the source of a slide shows its\n"+
			"notes.\n\nFlags:\n")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() == 0 {
		flags.Usage()
		return 2
	}
	// Diagrams that cannot be rendered are logged.
	log.SetFlags(0)
	log.SetPrefix("present: ")
	if err := exportDocs(*out, flags.Args(), *source, stdout); err != nil {
		fmt.Fprintf(stderr, "present: %v\n", err)
		return 1
	}
	return 0
}

// An exporter writes presentations, read from the current directory, to
// out.
type exporter struct {
	files  contentRoot // the current directory
	out    string
	source bool // include the sources of slides, as -doc does for serve
	stdout io.Writer
	faces  []fontFace // embedded in SVG files
	// root is the relative URL of out from the page being written, which
	// static files are linked to by.
	root string
	// copied holds the files, by their slash-separated paths under the
	// current directory, that have been copied to out.
	copied map[string]bool
}

// exportDocs writes the presentation files to the directory out, as the
// export command does, with the sources of slides if source is set.
func exportDocs(out string, files []string, source bool, stdout io.Writer) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	if fi, err := os.Stat(out); err == nil {
		if wi, err := os.Stat(wd); err == nil && os.SameFile(fi, wi) {
			return fmt.Errorf("%s is the current directory; export to another directory", out)
		}
	}

	e := &exporter{files: dirRoot("."), out: out, source: source, stdout: stdout, copied: make(map[string]bool)}
	staticURL := func(p string) string { return e.root + strings.TrimPrefix(p, "/") }
	// Code cannot be run on the pages, and they have no presenter notes.
	settings := pageSettings{Preload: true, InlineFonts: true, ThankYou: *thankYou}
	if err := initTemplates(embedFS, staticURL, settings); err != nil {
		return err
	}
	if e.faces, err = loadFontFaces(embedFS); err != nil {
		return err
	}

	// Check every file before writing anything.
	var docs []string // the files, by their paths under wd
	pages := make(map[string]string)
	for _, file := range files {
		if !isDoc(file) {
			return fmt.Errorf("%s is not a slide presentation (.slide) or article (.article)", file)
		}
		abs, err := filepath.Abs(file)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(wd, abs)
		if err != nil || !filepath.IsLocal(rel) {
			return fmt.Errorf("%s is not in the current directory", file)
		}
		rel = filepath.ToSlash(rel)
		pageFile := pageName(rel)
		if other, ok := pages[pageFile]; ok {
			if other == rel {
				continue
			}
			return fmt.Errorf("%s and %s would both be written to %s", other, rel, pageFile)
		}
		pages[pageFile] = rel
		docs = append(docs, rel)
	}

	if err := writeStatic(out, e.faces); err != nil {
		return err
	}
	for _, rel := range docs {
		if err := e.exportDoc(rel); err != nil {
			return err
		}
	}
	return nil
}

// pageName returns the name of the page that the presentation file name is
// written to.
func pageName(name string) string {
	return strings.TrimSuffix(name, path.Ext(name)) + ".html"
}

// exportDoc writes the presentation file rel, a slash-separated path under
// the current directory, to its page in e.out, along with the files it
// shows.
func (e *exporter) exportDoc(rel string) error {
	src, err := fs.ReadFile(e.files.fsys, rel)
	if err != nil {
		return err
	}
	doc, err := parse(e.files.fsys, bytes.NewReader(src), rel, 0)
	if err != nil {
		return err
	}
	renderDiagrams(doc, e.files, rel)
	failed := false
	eachDiagram(doc, func(img *present.Image) { failed = failed || img.Error != "" })
	if failed {
		return fmt.Errorf("%s: not exported, since a diagram could not be rendered", rel)
	}
	err = eachMediaURL(doc.Sections, func(ref string) (string, error) { return e.link(rel, ref) })
	if err != nil {
		return err
	}

	pageFile := pageName(rel)
	e.root = strings.Repeat("../", strings.Count(pageFile, "/"))
	data := newPageData(doc, rel)
	if e.source && path.Ext(rel) == ".slide" {
		data.SourceFile = rel
		data.Sources, err = slideSources(doc, src, e.files.fsys, rel, "/"+rel)
		if err != nil {
			return err
		}
		for _, s := range data.Sources {
			for i := range s.Files {
				if s.Files[i].URL, err = e.link(rel, s.Files[i].Name); err != nil {
					return err
				}
			}
		}
	}
	var buf bytes.Buffer
	if err := data.Template.ExecuteTemplate(&buf, "root", data); err != nil {
		return err
	}
	dst := filepath.Join(e.out, filepath.FromSlash(pageFile))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := writeFile(dst, linkPages(buf.Bytes())); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "Exported %s to %s.\n", rel, dst)
	return nil
}

// link copies the file that the URL ref in the presentation file rel
// refers to, if it is on the same server, to e.out, and returns the URL to
// link to it by from the page of rel.
func (e *exporter) link(rel, ref string) (string, error) {
	u, err := url.Parse(ref)
	if err != nil || ref == "" || u.Scheme != "" || u.Host != "" {
		return ref, nil
	}
	// Browsers resolve a URL path that leaves the root at the root, and
	// so does path.Clean.
	dir := path.Dir(rel)
	name := u.Path
	if !strings.HasPrefix(name, "/") {
		name = path.Join("/", dir, name)
	}
	name = strings.TrimPrefix(path.Clean(name), "/")
	if err := e.copyFile(name); err != nil {
		return "", fmt.Errorf("%s: %v", rel, err)
	}
	if !strings.HasPrefix(u.Path, "/") {
		return ref, nil
	}
	// Link to the file relative to the page, so that the pages can be
	// served under any path.
	r, err := filepath.Rel(filepath.FromSlash(dir), filepath.FromSlash(name))
	if err != nil {
		return "", err
	}
	u.Path, u.RawPath = filepath.ToSlash(r), ""
	return u.String(), nil
}

// hrefRE matches the href attribute of a link in a page.
var hrefRE = regexp.MustCompile(`href="([^"]*)"`)

// linkPages returns page with its links to presentation files on
// the same server changed to the pages they are exported to.
func linkPages(page []byte) []byte {
	return hrefRE.ReplaceAllFunc(page, func(attr []byte) []byte {
		ref := html.UnescapeString(string(hrefRE.FindSubmatch(attr)[1]))
		u, err := url.Parse(ref)
		if err != nil || u.Scheme != "" || u.Host != "" || !isDoc(u.Path) {
			return attr
		}
		u.Path, u.RawPath = pageName(u.Path), ""
		return []byte(`href="` + html.EscapeString(u.String()) + `"`)
	})
}

// copyFile copies the file name, a slash-separated path under the current
// directory, to the same path under e.out, unless it has been copied
// already. An SVG file gets the fonts it uses embedded, as serveSVG serves
// it.
func (e *exporter) copyFile(name string) error {
	if e.copied[name] {
		return nil
	}
	dst := filepath.Join(e.out, filepath.FromSlash(name))
	fi, err := fs.Stat(e.files.fsys, name)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a file", name)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if path.Ext(name) == ".svg" {
		svg, err := fs.ReadFile(e.files.fsys, name)
		if err != nil {
			return err
		}
		err = writeFile(dst, embedSVGFonts(svg, e.faces))
	} else {
		err = copyFileData(dst, e.files.fsys, name)
	}
	if err != nil {
		return err
	}
	e.copied[name] = true
	return nil
}

// copyFileData copies the contents of the file src in fsys to the file
// dst, without reading it all into memory, since it may be a large video.
func copyFileData(dst string, fsys fs.FS, src string) error {
	r, err := fsys.Open(src)
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, r)
	if err1 := w.Close(); err == nil {
		err = err1
	}
	return err
}

// writeStatic writes the static files of the pages to the static
// directory of out: those embedded in present, except that fonts.css has
// faces, the fonts in it, embedded, so that the pages need no font files
// and work when opened from files, which browsers do not load fonts from.
// The TrueType fonts, which only fonts.tar holds, are left out too.
func writeStatic(out string, faces []fontFace) error {
	return fs.WalkDir(embedFS, "static", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(out, filepath.FromSlash(name))
		switch {
		case name == fontsDir:
			return fs.SkipDir
		case d.IsDir():
			return os.MkdirAll(dst, 0o755)
		case path.Ext(name) == ".woff2":
			return nil
		case name == fontsCSS:
			return writeFile(dst, fontsCSSData(faces))
		}
		data, err := fs.ReadFile(embedFS, name)
		if err != nil {
			return err
		}
		return writeFile(dst, data)
	})
}

// fontsCSSData returns a style sheet of faces, with their fonts embedded.
func fontsCSSData(faces []fontFace) []byte {
	var b bytes.Buffer
	b.WriteString("/* Roboto and JetBrains Mono, embedded so that the pages work when opened\n" +
		"   from files. Licensed under the SIL Open Font License 1.1; see\n" +
		"   fonts/README.md. */\n")
	for _, f := range faces {
		b.WriteString(f.rule + "\n")
	}
	return b.Bytes()
}

// eachMediaURL replaces the URL of each file that the slides of sections
// show, as an image, video, iframe or background, with the result of f.
func eachMediaURL(sections []present.Section, f func(ref string) (string, error)) error {
	for i := range sections {
		if err := sectionMediaURLs(&sections[i], f); err != nil {
			return err
		}
	}
	return nil
}

func sectionMediaURLs(s *present.Section, f func(ref string) (string, error)) error {
	if s.Background != "" {
		u, err := f(s.Background)
		if err != nil {
			return err
		}
		// The background is shown by a style of the section.
		old, new := "url('"+s.Background+"')", "url('"+u+"')"
		for i, style := range s.Styles {
			s.Styles[i] = strings.ReplaceAll(style, old, new)
		}
		s.Background = u
	}
	for i, e := range s.Elem {
		var err error
		switch e := e.(type) {
		case present.Section:
			err = sectionMediaURLs(&e, f)
			s.Elem[i] = e
		case present.Image:
			e.URL, err = f(e.URL)
			s.Elem[i] = e
		case present.Video:
			e.URL, err = f(e.URL)
			s.Elem[i] = e
		case present.Iframe:
			e.URL, err = f(e.URL)
			s.Elem[i] = e
		}
		if err != nil {
			return err
		}
	}
	return nil
}
