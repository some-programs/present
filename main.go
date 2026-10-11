// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
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

// serveFlags holds the flags of the serve command, which the files that
// use them define.
var serveFlags = flag.NewFlagSet("present serve", flag.ContinueOnError)

var (
	httpAddr    = serveFlags.String("http", "127.0.0.1:3999", "HTTP service address (e.g., '127.0.0.1:3999')")
	originHost  = serveFlags.String("orighost", "", "host component of web origin URL (e.g., 'localhost')")
	basePath    = serveFlags.String("base", "", "base path for slide template and static resources")
	contentPath = serveFlags.String("content", ".", "base path for presentation content")
	thankYou    = serveFlags.Bool("thankyou", false, "end slide presentations with a \"Thank you\" slide listing the authors")
	noPreload   = serveFlags.Bool("no-preload", false, "disable preloading slide media (images and video)")
)

func init() {
	serveFlags.BoolVar(&present.PlayEnabled, "play", true, "enable playground (permit execution of arbitrary user code)")
	serveFlags.BoolVar(&present.NotesEnabled, "notes", false, "enable presenter notes (press 'N' from the browser to display them)")
	serveFlags.Usage = func() {
		w := serveFlags.Output()
		fmt.Fprintf(w, "usage: present serve [flags]\n\n")
		fmt.Fprintf(w, "Serve serves the slide presentations and articles in -content, the\ncurrent directory by default, at -http.\n\nFlags:\n")
		serveFlags.PrintDefaults()
	}
}

// A command is a subcommand of present, which the first argument names.
type command struct {
	name string
	help string
	// run runs the command with the arguments after its name, and returns
	// the exit code.
	run func(args []string, stdout, stderr io.Writer) int
}

// commands lists the commands, serve first.
var commands []command

func init() {
	commands = append(commands, command{"serve", "serve slide presentations and articles", serve})
	for _, c := range newCommands {
		commands = append(commands, command{c.name, c.help, c.run})
	}
	commands = append(commands, command{"convert-legacy", "convert presentations from the legacy markup to Markdown", convertLegacy})
}

//go:embed static templates
var embedFS embed.FS

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(2)
	}
	os.Exit(runCommand(os.Args[1], os.Args[2:], os.Stdout, os.Stderr))
}

// runCommand runs the command name with args, the arguments after it, and
// returns the exit code.
func runCommand(name string, args []string, stdout, stderr io.Writer) int {
	switch {
	case name == "-h" || name == "-help" || name == "--help" || name == "help":
		printUsage(stdout)
		return 0
	case strings.HasPrefix(name, "-"):
		fmt.Fprintf(stderr, "present: flags go after the command, as in: present serve %s\n\n", name)
		printUsage(stderr)
		return 2
	}
	for _, c := range commands {
		if c.name == name {
			return c.run(args, stdout, stderr)
		}
	}
	fmt.Fprintf(stderr, "present: unknown command %q\n\n", name)
	printUsage(stderr)
	return 2
}

// printUsage writes the usage of present, with its commands, to w.
func printUsage(w io.Writer) {
	fmt.Fprintf(w, "usage: present COMMAND [args]\n\nCommands:\n")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-15s %s\n", c.name, c.help)
	}
	fmt.Fprintf(w, "\nRun present COMMAND -h for the usage of a command, such as the flags of serve.\n")
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
	svgFontFaces, err = loadFontFaces(fsys)
	if err != nil {
		log.Fatalf("Failed to load fonts: %v", err)
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
	if err := initDoc(fsys, staticURL); err != nil {
		log.Fatalf("Failed to render documentation: %v", err)
	}
	http.Handle("/static/", hashfs.FileServer(staticFS))
	http.Handle("/download/fonts.tar", fontsTarHandler(fsys))

	if !ln.Addr().(*net.TCPAddr).IP.IsLoopback() && present.PlayEnabled {
		log.Print(localhostWarning)
	}

	log.Printf("Open your web browser and visit %s", origin.String())
	if present.NotesEnabled {
		log.Println("Notes are enabled, press 'N' from the browser to display them.")
	}
	log.Print(http.Serve(ln, compressHandler(http.DefaultServeMux)))
	return 1
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
