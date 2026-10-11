# present

[![Go Reference](https://pkg.go.dev/badge/github.com/some-programs/present.svg)](https://pkg.go.dev/github.com/some-programs/present)

Present is a simple presentation tool for slides written as plain text files
with Markdown, and shown in the browser.

Source: [github.com/some-programs/present](https://github.com/some-programs/present)

## Features

- Slides and articles written in plain-text Markdown, shown in the browser.
- Code from source files that runs live from the slide: Go, Python, JavaScript,
  TypeScript, or anything with a shebang.
- Images, video, and Graphviz and Mermaid diagrams.
- Presenter notes window, kept in sync with the slides.
- Hot reload while you write.
- Single binary that works offline.

## This is a fork

Present is based on [golang.org/x/tools/cmd/present](https://pkg.go.dev/golang.org/x/tools/cmd/present).
It started as a verbatim copy of it at commit
[a3af45f248c005a2f2485ca99ad978547753a17c](https://go.googlesource.com/tools/+/a3af45f248c005a2f2485ca99ad978547753a17c)
("internal/refactor/inline: reduce void calls in tail position").

Most markdown based slides made for the x/tools present should work but the old
non markdown syntax support has been removed, the fonts have changed and the
padding outside of the slide body is slightly reduced. Presentations in the old
syntax can be converted to Markdown with `present convert-legacy`.

## Start demo

For an online preview, see the
[examples on GitHub Pages](https://some-programs.github.io/present/examples/),
exported with `present export -source`. Code cannot be run there, and there
are no presenter notes.

The quickest way to learn present is to look at the [examples](examples):
presentations that show every feature, from Markdown text and speaker notes
to runnable code, diagrams and video. They are built into present, so one
line downloads and runs the latest version and shows them along with this
documentation:

    go run github.com/some-programs/present@latest serve -doc -notes

Then open http://127.0.0.1:3999. The index lists the presentations in the
current directory, below links to the documentation and the examples. On a
slide, press S to see its source beside it.
