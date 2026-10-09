// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Present displays slide presentations and articles. It runs a web server that
presents slide and article files from the current directory.

Present assumes presentation authors are trusted, and should not be used with untrusted inputs.

Programs in .play blocks are built and run on the machine running present,
and their output is streamed to the browser over a WebSocket. Run with
-play=false to disable this.

With -hot, an open presentation reloads in the browser whenever its file, or
a file it uses, changes. It opens at the first slide that changed, and a
presentation that no longer parses shows its error over the old slides.
-hot cannot be used with -notes.

An .image of a diagram file, Graphviz (.dot) or Mermaid (.mmd, .mermaid), shows
the SVG file of the same name, which present renders with dot or mmdc
whenever the diagram file changes. With -norender, present does not render
them, and shows the existing SVG files as they are; -dot= or -mmdc= does
the same for one kind of diagram.

Input files are named foo.extension, where "extension" defines the format of
the generated output. The supported formats are:

	.slide        // HTML5 slide presentation
	.article      // article format, such as a blog post

The present file format is documented by the present package:
https://pkg.go.dev/github.com/some-programs/present/pkg/present
*/
package main

//go:generate go run ./internal/readmegen
