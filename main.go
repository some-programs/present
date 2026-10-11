// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/benbjohnson/hashfs"
	"github.com/some-programs/present/pkg/present"
)

var (
	httpAddr    = flag.String("http", "127.0.0.1:3999", "HTTP service address (e.g., '127.0.0.1:3999')")
	originHost  = flag.String("orighost", "", "host component of web origin URL (e.g., 'localhost')")
	basePath    = flag.String("base", "", "base path for slide template and static resources")
	contentPath = flag.String("content", ".", "base path for presentation content")
	thankYou    = flag.Bool("thankyou", false, "end slide presentations with a \"Thank you\" slide listing the authors")
)

//go:embed static templates
var embedFS embed.FS

func main() {
	flag.BoolVar(&present.PlayEnabled, "play", true, "enable playground (permit execution of arbitrary user code)")
	flag.BoolVar(&present.NotesEnabled, "notes", false, "enable presenter notes (press 'N' from the browser to display them)")
	noPreload := flag.Bool("no-preload", false, "disable preloading slide media (images and video)")
	flag.Parse()
	if *noPreload {
		present.PreloadEnabled = false
	}
	if *hot && present.NotesEnabled {
		log.Fatal("-hot and -notes cannot be used together")
	}

	var fsys fs.FS = embedFS
	if *basePath != "" {
		fsys = os.DirFS(*basePath)
	}
	staticFS := hashfs.NewFS(fsys)
	staticURL := func(path string) string { return path }
	if *basePath == "" {
		// The embedded files cannot change while the server runs, so the
		// templates can link to content-hashed names that browsers cache
		// for good. Files under -base may be edited, and hashfs never
		// rehashes a file, so those are linked to by their plain names.
		staticURL = func(path string) string {
			return "/" + staticFS.HashName(strings.TrimPrefix(path, "/"))
		}
	}
	err := initTemplates(fsys, staticURL)
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}

	ln, err := net.Listen("tcp", *httpAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}

	origin := &url.URL{Scheme: "http"}
	if *originHost != "" {
		if after, ok := strings.CutPrefix(*originHost, "https://"); ok {
			*originHost = after
			origin.Scheme = "https"
		}
		*originHost = strings.TrimPrefix(*originHost, "http://")
		origin.Host = net.JoinHostPort(*originHost, port)
	} else if ln.Addr().(*net.TCPAddr).IP.IsUnspecified() {
		name, _ := os.Hostname()
		origin.Host = net.JoinHostPort(name, port)
	} else {
		reqHost, reqPort, err := net.SplitHostPort(*httpAddr)
		if err != nil {
			log.Fatal(err)
		}
		if reqPort == "0" {
			origin.Host = net.JoinHostPort(reqHost, port)
		} else {
			origin.Host = *httpAddr
		}
	}

	initPlayground(origin)
	initHot()
	http.Handle("/static/", hashfs.FileServer(staticFS))
	http.Handle("/download/fonts.tar", fontsTarHandler(fsys))

	if !ln.Addr().(*net.TCPAddr).IP.IsLoopback() && present.PlayEnabled {
		log.Print(localhostWarning)
	}

	log.Printf("Open your web browser and visit %s", origin.String())
	if present.NotesEnabled {
		log.Println("Notes are enabled, press 'N' from the browser to display them.")
	}
	log.Fatal(http.Serve(ln, nil))
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
