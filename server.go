// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/benbjohnson/hashfs"
	"github.com/some-programs/present/pkg/playground/socket"
)

// serveFlags holds the flags of the serve command.
var serveFlags = flag.NewFlagSet("present serve", flag.ContinueOnError)

var (
	httpAddrFlag    = serveFlags.String("http", "127.0.0.1:3999", "HTTP service address (e.g., '127.0.0.1:3999')")
	originHostFlag  = serveFlags.String("orighost", "", "host component of web origin URL (e.g., 'localhost')")
	contentPathFlag = serveFlags.String("content", ".", "base path for presentation content")
	thankYouFlag    = serveFlags.Bool("thankyou", false, "end slide presentations with a \"Thank you\" slide listing the authors")
	noPreloadFlag   = serveFlags.Bool("no-preload", false, "disable preloading slide media (images and video)")
	playFlag        = serveFlags.Bool("play", true, "enable playground (permit execution of arbitrary user code)")
	notesFlag       = serveFlags.Bool("notes", false, "enable presenter notes (press 'N' from the browser to display them)")
	hotFlag         = serveFlags.Bool("hot", false, "reload presentations in the browser when they or the files they use change (not allowed with -notes)")
	docFlag         = serveFlags.Bool("doc", false, "serve the present documentation (README.md) at /_doc, the example presentations at /_examples and the fonts at /download/fonts.tar, and list them on the Talks page")
)

func init() {
	serveFlags.Usage = func() {
		w := serveFlags.Output()
		fmt.Fprintf(w, "usage: present serve [flags]\n\n")
		fmt.Fprintf(w, "Serve serves the slide presentations and articles in -content, the\ncurrent directory by default, at -http.\n\nFlags:\n")
		serveFlags.PrintDefaults()
	}
}

// serve runs the serve command, which serves presentations until it fails.
func serve(args []string, stdout, stderr io.Writer) int {
	serveFlags.SetOutput(stderr)
	if err := serveFlags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if serveFlags.NArg() > 0 {
		fmt.Fprintf(stderr, "present serve: unexpected argument %q\n\n", serveFlags.Arg(0))
		serveFlags.Usage()
		return 2
	}
	if *hotFlag && *notesFlag {
		log.Fatal("-hot and -notes cannot be used together")
	}

	ln, err := net.Listen("tcp", *httpAddrFlag)
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	origin, err := originURL(ln.Addr(), *httpAddrFlag, *originHostFlag)
	if err != nil {
		log.Fatal(err)
	}

	s, err := newServer(embedFS, *contentPathFlag, pageSettings{
		Play:     *playFlag,
		Notes:    *notesFlag,
		Preload:  !*noPreloadFlag,
		ThankYou: *thankYouFlag,
		Hot:      *hotFlag,
	}, *docFlag, origin)
	if err != nil {
		log.Fatal(err)
	}

	if !ln.Addr().(*net.TCPAddr).IP.IsLoopback() && *playFlag {
		log.Print(localhostWarning)
	}

	log.Printf("Open your web browser and visit %s", origin.String())
	if *notesFlag {
		log.Println("Notes are enabled, press 'N' from the browser to display them.")
	}
	log.Print(http.Serve(ln, s.routes()))
	return 1
}

// originURL returns the origin of the pages served by a server listening
// at addr, which was asked to listen at httpAddr: originHost, with the port
// of addr; or with no originHost, the name of this machine if addr is any
// address, and otherwise httpAddr, with the port of addr if that was 0.
// originHost may start with http:// or https://.
func originURL(addr net.Addr, httpAddr, originHost string) (*url.URL, error) {
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return nil, err
	}
	origin := &url.URL{Scheme: "http"}
	if originHost != "" {
		if after, ok := strings.CutPrefix(originHost, "https://"); ok {
			originHost = after
			origin.Scheme = "https"
		}
		originHost = strings.TrimPrefix(originHost, "http://")
		origin.Host = net.JoinHostPort(originHost, port)
	} else if addr.(*net.TCPAddr).IP.IsUnspecified() {
		name, _ := os.Hostname()
		origin.Host = net.JoinHostPort(name, port)
	} else {
		reqHost, reqPort, err := net.SplitHostPort(httpAddr)
		if err != nil {
			return nil, err
		}
		if reqPort == "0" {
			origin.Host = net.JoinHostPort(reqHost, port)
		} else {
			origin.Host = httpAddr
		}
	}
	return origin, nil
}

const localhostWarning = `
WARNING!  WARNING!  WARNING!

The present server appears to be listening on an address that is not localhost
and is configured to run code snippets locally. Anyone with access to this address
and port will have access to this machine as the user running present.

To avoid this message, listen on localhost or run with -play=false.

If you don't understand this message, hit Control-C to terminate this process.

WARNING!  WARNING!  WARNING!
`

// A server serves the presentations in a directory over HTTP, as the serve
// command does.
type server struct {
	fsys     fs.FS       // the static files and the templates
	staticFS *hashfs.FS  // fsys, with content-hashed names
	content  contentRoot // the -content directory, served at /
	settings pageSettings
	doc      bool     // serve the README, the examples and the fonts too
	origin   *url.URL // the origin of the pages, which /socket accepts

	renderer        *renderer
	dirListTemplate *template.Template
	svgFonts        []fontFace // embedded in SVG files
	readme          []byte     // README.md as a page, with doc
}

// newServer returns a server of the presentations in the directory
// content, with the static files and templates in fsys, for pages with
// settings. With doc, it serves the documentation and the examples too.
// The playground accepts pages from origin.
func newServer(fsys fs.FS, content string, settings pageSettings, doc bool, origin *url.URL) (*server, error) {
	srv := &server{
		fsys:     fsys,
		staticFS: hashfs.NewFS(fsys),
		content:  dirRoot(content),
		settings: settings,
		doc:      doc,
		origin:   origin,
	}
	var err error
	if srv.renderer, err = newRenderer(fsys, srv.staticURL, settings); err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}
	if srv.dirListTemplate, err = parseDirList(fsys, srv.staticURL); err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}
	if srv.svgFonts, err = loadFontFaces(fsys); err != nil {
		return nil, fmt.Errorf("loading fonts: %w", err)
	}
	if doc {
		if srv.readme, err = readmePage(fsys, srv.staticURL); err != nil {
			return nil, fmt.Errorf("rendering documentation: %w", err)
		}
	}
	return srv, nil
}

// routes returns the handler of every URL path that s serves.
func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	// Presentations, directory listings and other files.
	mux.HandleFunc("/", s.serveFiles)
	mux.Handle("/static/", hashfs.FileServer(s.staticFS))
	if s.doc {
		mux.HandleFunc("/_examples", s.serveFiles)
		mux.HandleFunc("/_examples/", s.serveFiles)
		mux.HandleFunc("/_doc", s.serveReadme)
		mux.Handle("/download/fonts.tar", fontsTarHandler(s.fsys))
	}
	if s.settings.Play {
		// Runs programs for static/js/play.js.
		mux.Handle("/socket", socket.NewHandler(s.origin))
	}
	if s.settings.Hot {
		// Tells static/js/hot.js when to reload.
		mux.HandleFunc("/_hot", s.hotHandler)
	}
	return compressHandler(mux)
}

// staticURL returns the URL that the static file at path, such as
// "/static/styles.css", is served at. The embedded files cannot change
// while the server runs, so pages can link to content-hashed names that
// browsers cache for good. Fonts keep their plain names, by which
// fonts.css loads them.
func (s *server) staticURL(path string) string {
	if strings.HasPrefix(path, "/static/fonts/") {
		return path
	}
	return "/" + s.staticFS.HashName(strings.TrimPrefix(path, "/"))
}

// file returns the root that the URL path p is served from, the examples
// with -doc or otherwise the content directory, and the name of the file
// served at p in it.
func (s *server) file(p string) (contentRoot, string) {
	p = path.Clean("/" + p)
	root := s.content
	if s.doc && (p == examplesPath || strings.HasPrefix(p, examplesPath+"/")) {
		root = examplesRoot
	}
	name := strings.TrimPrefix(strings.TrimPrefix(p, root.prefix), "/")
	if name == "" {
		name = "."
	}
	return root, name
}

// serveFiles serves the file at the requested path: the page of a
// presentation, or its presenter notes window with ?notes, a directory
// listing, or the file itself.
func (s *server) serveFiles(w http.ResponseWriter, r *http.Request) {
	root, name := s.file(r.URL.Path)
	if isDoc(name) {
		s.servePage(w, r, root, name)
		return
	}
	fi, err := fs.Stat(root.fsys, name)
	if err != nil {
		serveError(w, r, name, err)
		return
	}
	if fi.IsDir() {
		if err := s.dirList(w, root.fsys, name, r.URL.Path); err != nil {
			serveError(w, r, name, err)
		}
		return
	}
	// Have browsers check for a newer file on every load, such as an
	// image that has been replaced while the slides are open.
	w.Header().Set("Cache-Control", "no-cache")
	if path.Ext(name) == ".svg" && serveSVG(w, r, root.fsys, name, s.svgFonts) {
		return
	}
	http.ServeFileFS(w, r, root.fsys, name)
}

// servePage serves the page of the presentation file name in root, or with
// ?notes, its presenter notes window. The page is rendered in full first,
// so that an error is sent instead of part of it.
func (s *server) servePage(w http.ResponseWriter, r *http.Request, root contentRoot, name string) {
	var buf bytes.Buffer
	var err error
	if r.URL.Query().Has("notes") {
		if !s.settings.Notes || path.Ext(name) != ".slide" {
			http.NotFound(w, r)
			return
		}
		err = s.renderNotes(&buf, root, name)
	} else {
		err = s.renderDoc(&buf, root, name, r.URL.Path)
	}
	if err != nil {
		serveError(w, r, name, err)
		return
	}
	w.Write(buf.Bytes())
}

// serveReadme serves README.md as a page.
func (s *server) serveReadme(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(s.readme)
}

// serveError logs err and replies with an error for the request for name.
// If name itself does not exist, the reply is a plain 404. Otherwise the
// error text is sent so that authors see mistakes such as an unknown
// command.
func serveError(w http.ResponseWriter, r *http.Request, name string, err error) {
	addr, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		addr = r.RemoteAddr
	}
	log.Printf("request from %s: %s", addr, err)

	if pathErr, ok := errors.AsType[*fs.PathError](err); ok && pathErr.Path == name && errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// renderDoc reads the present file docFile in root, served at the URL path
// urlPath, and writes its page to w.
func (s *server) renderDoc(w io.Writer, root contentRoot, docFile, urlPath string) error {
	// Read the input and build the doc structure.
	src, err := fs.ReadFile(root.fsys, docFile)
	if err != nil {
		return err
	}
	doc, err := parse(root.fsys, bytes.NewReader(src), docFile, 0)
	if err != nil {
		return err
	}
	renderDiagrams(doc, root, docFile)

	var sourceFile string
	var sources []slideSource
	if s.doc && path.Ext(docFile) == ".slide" {
		sourceFile = sourceFileName(root, docFile)
		if sources, err = slideSources(doc, src, root.fsys, docFile, urlPath); err != nil {
			return err
		}
	}
	return s.renderer.page(w, doc, docFile, sourceFile, sources)
}

// sourceFileName returns the name that the source panel shows for the
// presentation file docFile in root: its path in the content directory, or
// in the examples directory of the repository.
func sourceFileName(root contentRoot, docFile string) string {
	if root.prefix == examplesPath {
		return "examples/" + docFile
	}
	return docFile
}

// renderNotes writes the presenter notes window of the slide presentation
// docFile in root to w. The presentation's own window shows itself in it.
func (s *server) renderNotes(w io.Writer, root contentRoot, docFile string) error {
	doc, err := parseFile(root.fsys, docFile, 0)
	if err != nil {
		return err
	}
	return s.renderer.notes(w, doc)
}
