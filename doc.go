// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Present displays slide presentations and articles. Its serve command runs a
web server that presents slide and article files from the current directory:

	present serve [flags]

Present without a command lists the commands. The flags described below
are flags of serve, as in "present serve -notes"; "present serve -h" lists
them all.

Present assumes presentation authors are trusted, and should not be used with untrusted inputs.

Programs in .play blocks are built and run on the machine running present,
and their output is streamed to the browser over a WebSocket. Run with
-play=false to disable this.

With -hot, an open presentation reloads in the browser whenever its file, or
a file it uses, changes. It opens at the first slide that changed, and a
presentation that no longer parses shows its error over the old slides, as
does a diagram that cannot be rendered. -hot cannot be used with -notes.

An .image of a diagram file, Graphviz (.dot) or Mermaid (.mmd, .mermaid), shows
the SVG file of the same name, which present renders with dot or mmdc
whenever the diagram file changes. With -norender, present does not render
them, and shows the existing SVG files as they are; -dot= or -mmdc= does
the same for one kind of diagram.

With -doc, the Talks page lists this documentation, served at /_doc, and
the example presentations, which are built into present and served at
/_examples, for reading them next to the presentations being written.
Pressing S on a slide then shows its source beside it: the lines of the
presentation file that make it, and the files that its commands use.

Other commands write new files from skeletons with present's defaults, and
fail rather than overwrite a file:

	present new-slide my-talk                    # my-talk.slide, a slide presentation
	present new-article notes                    # notes.article, an article
	present new-dot diagrams/flow                # diagrams/flow.dot, a Graphviz diagram
	present new-mmd diagrams/flow                # diagrams/flow.mmd, a Mermaid flowchart
	present new-mmd -type sequence diagrams/seq  # diagrams/seq.mmd, a Mermaid sequence diagram

The diagrams use the fonts of the slides, as described under Diagram Style in
the present package: Roboto, or JetBrains Mono with -mono, for a diagram of
code. The -type of a Mermaid diagram is flowchart, the default, sequence,
class or state. Run "present COMMAND -h" for the usage of a command.

The convert-legacy command converts presentations in the legacy present
markup, which golang.org/x/tools/present also reads, to Markdown. It prints
the result, or with -w, converts each file in place:

	present convert-legacy my-talk.slide         # print my-talk.slide in Markdown
	present convert-legacy -w *.slide *.article  # convert them all in place

Input files are named foo.extension, where "extension" defines the format of
the generated output. The supported formats are:

	.slide        // HTML5 slide presentation
	.article      // article format, such as a blog post

*/
package main

//go:generate go run ./internal/readmegen
