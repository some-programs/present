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

An .image of a diagram file, Graphviz (.dot), Mermaid (.mmd, .mermaid) or
PlantUML (.puml, .plantuml, .pu), shows the SVG file of the same name, which
present renders with dot, mmdc or plantuml whenever the diagram file
changes. With -norender, present does not render them, and shows the
existing SVG files as they are; -dot=, -mmdc= or -plantuml= does the same
for one kind of diagram.

With -doc, the Talks page lists this documentation, served at /_doc, and
the example presentations, which are built into present and served at
/_examples, for reading them next to the presentations being written.
Pressing S on a slide then shows its source beside it: the lines of the
presentation file that make it, and the files that its commands use.
Adding ?source=true to the URL of a presentation opens it with the source
shown, and ?source=false with it hidden. The fonts of the slides are served too, at /download/fonts.tar, for
installing them.

Other commands write new files from skeletons with present's defaults, and
fail rather than overwrite a file:

	present new-slide my-talk                    # my-talk.slide, a slide presentation
	present new-article notes                    # notes.article, an article
	present new-dot diagrams/flow                # diagrams/flow.dot, a Graphviz diagram
	present new-mmd diagrams/flow                # diagrams/flow.mmd, a Mermaid flowchart
	present new-mmd -type sequence diagrams/seq  # diagrams/seq.mmd, a Mermaid sequence diagram
	present new-puml diagrams/seq                # diagrams/seq.puml, a PlantUML sequence diagram

The diagrams use the fonts of the slides, as described under Diagram Style in
the present package: Roboto, or JetBrains Mono with -mono, for a diagram of
code. The -type of a Mermaid diagram is flowchart, the default, sequence,
class or state, and of a PlantUML diagram sequence, the default, class,
activity or state. Run "present COMMAND -h" for the usage of a command.

The export command writes slide presentations and articles as static HTML
pages, for any web server to serve:

	present export my-talk.slide                 # present.export/my-talk.html
	present export -o public talks/*.slide       # public/talks/*.html

Each page is written to the -o directory, present.export by default, at
the path of its file under the current directory, with the images, videos
and iframe pages it shows, and the style sheets, scripts and fonts of the
pages in the static directory. Links to other presentations point at
their pages. Diagrams are rendered first, and export fails if one cannot
be. The pages cannot run code, and have no presenter notes. They work
opened from files as well as served by any web server, since the style
sheet embeds the fonts, which browsers do not load from files.

With -source, pressing S on an exported slide shows its source, notes
included, as with -doc, and the files that its commands use are written
too.

The convert-legacy command converts presentations in the legacy present
markup, which golang.org/x/tools/present also reads, to Markdown. It prints
the result, or with -w, converts each file in place:

	present convert-legacy my-talk.slide         # print my-talk.slide in Markdown
	present convert-legacy -w *.slide *.article  # convert them all in place

The fmt command formats slide presentations and articles, as gofmt does Go
files. It changes only spaces and blank lines, and never how a presentation
looks: it removes trailing spaces, but for those in code, and makes the
line breaks of trailing spaces backslashes; it keeps a single blank line
where there were more; and it sets headings, commands, notes and text
apart with one. It prints the result, or with -w, formats each file in
place; -l lists the files it would change, and -d prints the changes:

	present fmt -w my-talk.slide                 # format my-talk.slide in place
	present fmt -l -r .                          # list the files to format, in all directories

The lint command checks slide presentations and articles for problems,
such as commands that are not known, or whose files do not exist, notes
or headings that are text since a space is missing, and files that fmt
would change. It prints each as file:line: message (rule), or as JSON
with -json, and exits with status 1 if it finds any:

	present lint my-talk.slide                   # check my-talk.slide
	present lint -json -r talks                  # check all presentations in talks, as JSON

With -r, a directory stands for the presentations in it and in the
directories below it, but for those whose names start with . or _, for
both commands.

Input files are named foo.extension, where "extension" defines the format of
the generated output. The supported formats are:

	.slide        // HTML5 slide presentation
	.article      // article format, such as a blog post

*/
package main

//go:generate go run ./internal/readmegen
