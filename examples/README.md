# Examples

Example presentations that together use every feature of present.
To look at them, run present from the root of this repository:

    go run . serve -content examples -notes

and open http://127.0.0.1:3999. Press N on a slide to open the speaker
notes. Without a checkout, run

    go run github.com/some-programs/present@latest serve -doc -notes

and open Examples in the Documentation section of the index. There, press
S on a slide to see its source beside it.

- [intro.slide](intro.slide): the header, authors, Markdown text, tables, `.fontsize`,
  subsections, speaker notes and a block of them, comments, `.link`, `.html` and `.iframe`.
- [code.slide](code.slide): `.code` with addresses, `OMIT`, highlighting,
  `-numbers` and `-edit`, `.fontsize`, and `.play` with a Go program, a
  multi-file program in a txtar archive, a program that does not compile,
  Python, JavaScript and TypeScript programs and a shell script.
- [media.slide](media.slide): `.image` sizes, `.caption`, Graphviz and
  Mermaid diagrams, `.background` and `.video`.
- [notes.article](notes.article): the article format.

The diagrams in [diagrams](diagrams) are rendered to the SVG files next to
them with `dot` and `mmdc` when a slide shows them. The SVG files are
checked in, so the slides also work without those tools, or with
`-norender`.
