// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package present implements parsing and rendering of present files,
which can be slide presentations as in golang.org/x/tools/cmd/present
or articles as in golang.org/x/blog (the Go blog).

This package assumes presentation authors are trusted,
and should not be used with untrusted inputs.

# File Format

Present files begin with a header giving the title of the document
and other metadata, which looks like:

	# Title of document
	Subtitle of document
	15:04 2 Jan 2006
	Tags: foo, bar, baz
	Summary: This is a great document you want to read.
	OldURL: former-path-for-this-doc

The title line must begin with "# ". The body of the file uses
Markdown for text markup. Files in the legacy present markup, whose title
line has no "# " prefix, are not supported.

The date line may be written without a time:

	2 Jan 2006

In this case, the time will be interpreted as 11am UTC on that date.

The tags line is a comma-separated list of tags that may be used to categorize
the document.

The summary line gives a short summary used in blog feeds.

The old URL line, which may be repeated, gives an older (perhaps relative) URL
for this document.
A server might use these to generate appropriate redirects.

Only the title is required;
the subtitle, date, tags, summary, and old URL lines are optional.
The summary defaults to being empty.

After the header come zero or more author blocks, like this:

	Author Name
	Job title, Company
	joe@example.com
	https://url/
	@twitter_name

The first line of the author block is conventionally the author name.
Otherwise, the author section may contain a mixture of text, twitter names, and links.
For slide presentations, only the plain text lines will be displayed on the
first slide.

If multiple author blocks are listed, each new block must be preceded
by its own blank line.

After the author blocks come the presentation slides or article sections,
which can in turn have subsections.
Each slide or section begins with a "##" header line,
subsections begin with a "###" header line, and so on.

In addition to the marked-up text in a section (or subsection),
a present file can contain present command invocations, each of which begins
with a dot, as in:

	.code x.go /^func main/,/^}/
	.play y.go
	.image image.jpg
	.background image.jpg
	.iframe https://foo
	.link https://foo label
	.html file.html
	.caption _Gopher_ by [Renee French](https://instagram.com/reneefrench)

Other than the commands, the text in a section is interpreted as Markdown.

# Markdown Syntax

Markdown typically means the generic name for a family of similar markup languages.
The specific variant used in present is CommonMark,
with the tables of GitHub Flavored Markdown (https://github.github.com/gfm/#tables-extension-).
See https://commonmark.org/help/tutorial/ for a quick tutorial.

Section headings can end in {#name} to set the HTML anchor ID for the heading to "name".

Lines beginning with "//" (outside of code blocks, of course)
are treated as present comments and have no effect.

Lines beginning with ": " are treated as speaker notes, described below.

Example:

	# Title of Talk

	My Name
	9 Mar 2020
	me@example.com

	## Title of Slide or Section (must begin with ##)

	Some Text

	### Subsection {#anchor}

	- bullets
	- more bullets
	- a bullet continued
	  on the next line

	#### Sub-subsection

	Some More text

		Preformatted text (code block)
		is indented (by one tab, or four spaces)

	Further Text, including command invocations.

	## Section 2: Example formatting {#fmt}

	Formatting:

	_italic_
	// A comment that is completely ignored.
	: Speaker notes.
	**bold**
	`program`
	Markup—_especially italic text_—can easily be overused.
	_Why use scoped\_ptr_? Use plain **\*ptr** instead.

	Visit [the Go home page](https://golang.org/).

	| Column | Aligned right |
	|--------|--------------:|
	| cell   | cell          |

# Command Invocations

A number of special commands are available through invocations
in the input text. Each such invocation contains a period as the
first character on the line, followed immediately by the name of
the function, followed by any arguments. A typical invocation might
be

	.play demo.go /^func show/,/^}/

(except that the ".play" must be at the beginning of the line and
not be indented as in this comment.)

Here follows a description of the functions:

code:

Injects program source into the output by extracting code from files
and injecting them as HTML-escaped <pre> blocks.  The argument is
a file name followed by an optional address that specifies what
section of the file to display. The address syntax is similar in
its simplest form to that of ed, but comes from sam and is more
general. See

	https://plan9.io/sys/doc/sam/sam.html Table II

for full details. The displayed block is always rounded out to a
full line at both ends.

If no pattern is present, the entire file is displayed.

Any line in the program that ends with the four characters

	OMIT

is deleted from the source before inclusion, making it easy
to write things like

	.code test.go /START OMIT/,/END OMIT/

to find snippets like this

	tedious_code = boring_function()
	// START OMIT
	interesting_code = fascinating_function()
	// END OMIT

and see only this:

	interesting_code = fascinating_function()

Also, inside the displayed text a line that ends

	// HL

will be highlighted in the display. A highlighting mark may have a
suffix word, such as

	// HLxxx

Such highlights are enabled only if the code invocation ends with
"HL" followed by the word:

	.code test.go /^type Foo/,/^}/ HLxxx

The .code function may take one or more flags immediately preceding
the filename. This command shows test.go in an editable text area:

	.code -edit test.go

This command shows test.go with line numbers:

	.code -numbers test.go

play:

The function "play" is the same as "code" but puts a button
on the displayed source so the program can be run from the browser.
Although only the selected text is shown, all the source is included
in the HTML output so it can be presented to the compiler.
A file whose name ends in .py is run with python3, and one ending
in .js with node. One ending in .ts is type checked and compiled
with tsc and then run with node; without tsc, node runs it by
stripping its types. A program that starts with "#!" is run with
that interpreter; anything else is built and run as Go.

link:

Create a hyperlink. The syntax is 1 or 2 space-separated arguments.
The first argument is always the HTTP URL.  If there is a second
argument, it is the text label to display for this link, rendered as
inline Markdown.

	.link https://golang.org golang.org

image:

The template uses the function "image" to inject picture files.

The first argument is always the file name. On slides, the image is
fitted into the space left for it, below any text and above any caption,
keeping its aspect ratio; with no other arguments it fills that space.
A single further argument is a fraction from 0.0 to 1.0 of the slide's
content area that the image may use. Two further arguments are the height
and width, each a fraction, a number of pixels, or an underscore for no
limit. The image never grows beyond the space left for it.

	.image images/betsy.jpg
	.image images/betsy.jpg 0.5
	.image images/janet.jpg _ 300
	.image images/janet.jpg 0.4 _

A diagram file is shown as the SVG file of the same name next to it: a
Graphviz file (.dot), rendered with dot, or a Mermaid file (.mmd or
.mermaid), rendered with the Mermaid CLI, mmdc. The present program
renders it whenever the diagram file has changed, and marks the SVG with a
hash of the diagram file to tell. If the tool is missing or fails, an
earlier SVG is shown, and the error appears in the presenter notes window,
or at the top of the page with -hot; without an earlier SVG, the error
takes the image's place. See Diagram Style for making diagrams match the
slides.

	.image diagrams/flow.dot
	.image diagrams/sequence.mmd

video:

The template uses the function "video" to inject video files.

The syntax is simple: 2 or 4 space-separated arguments.
The first argument is always the file name.
The second argument is always the file content-type.
If there are more arguments, they are the height and width;
both must be present, or substituted with an underscore.
Replacing a dimension argument with the underscore parameter
preserves the aspect ratio of the video when scaling.

	.video videos/evangeline.mp4 video/mp4 400 600

	.video videos/mabel.ogg video/ogg 500 _

background:

The template uses the function "background" to set the background image for
a slide.  The only argument is the file name of the image.

	.background images/susan.jpg

caption:

The template uses the function "caption" to inject figure captions.

The text after ".caption" is rendered as inline Markdown and embedded in
a figcaption element.

	.caption _Gopher_ by [Renee French](https://instagram.com/reneefrench)

fontsize:

The function "fontsize" sets the font size of what follows it in the
section, up to the next fontsize or the end of the section. Code and
subsection headings are sized along with the text. The size is a factor
of the default size, a percentage of it, or a size in px or pt; without
a size, fontsize goes back to the default.

	.fontsize 0.8
	.fontsize 120%
	.fontsize 20px
	.fontsize

iframe:

The function "iframe" injects iframes (pages inside pages).
Its syntax is the same as that of image.

html:

The function html includes the contents of the specified file as
unescaped HTML. This is useful for including custom HTML elements
that cannot be created using only the slide format.
It is your responsibility to make sure the included HTML is valid and safe.

	.html file.html

# Diagram Style

Diagrams look like part of the presentation when they use its fonts:
Roboto for text and JetBrains Mono for code. A Graphviz file sets them at the
top of the graph, for the graph's own label, the nodes and the edges, and
for single nodes that show code:

	digraph example {
		fontname="Roboto,sans-serif"
		node [fontname="Roboto,sans-serif"]
		edge [fontname="Roboto,sans-serif"]
		bgcolor=transparent

		main -> worker
		worker [fontname="JetBrains Mono,monospace", label="go work(jobs)"]
	}

Graphviz paints a white background; bgcolor=transparent lets a slide's
.background show through instead.

A Mermaid file sets its font in a config block at its start. Mermaid uses
one font for a whole diagram, so a diagram of code uses
"JetBrains Mono, monospace" instead.

	---
	config:
	  fontFamily: Roboto, sans-serif
	---
	sequenceDiagram
	    Browser->>present: Run

The same block can set "theme: neutral", which draws in grays rather than
Mermaid's default purple and yellow.

The SVG is shown as an image, which cannot use the fonts that present
serves to the page, so a diagram uses the fonts installed on the computer
showing it, falling back to the system's sans-serif or monospace font.
Both tools also size labels with the fonts installed where present runs.
Install Roboto and JetBrains Mono on both for diagrams to match the slides
and their labels to fit. A running present serves both as TrueType files,
with their licenses, at /download/fonts.tar.

# Presenter Notes

Lines that begin with ": " are treated as presenter notes.
By default, presenter notes are collected but ignored.

When running the present command with -notes,
typing 'N' in your browser displaying your slides
will create a second window displaying the notes.
The second window is completely synced with the main
window, except that presenter notes are only visible in the second window.

Notes may appear anywhere within the slide text. For example:

	## Title of slide

	Some text.

	: Presenter notes (first paragraph)

	Some more text.

	: Presenter notes (subsequent paragraph(s))
*/
package present // import "github.com/some-programs/present/pkg/present"
