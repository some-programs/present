package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/aymanbagabas/go-udiff"
	"github.com/some-programs/present/pkg/present"
)

// format runs the fmt command, which formats presentations.
func format(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("fmt", flag.ContinueOnError)
	flags.SetOutput(stderr)
	list := flags.Bool("l", false, "list the files whose formatting differs from present fmt's")
	write := flags.Bool("w", false, "write the results to the files rather than to standard output")
	diff := flags.Bool("d", false, "print diffs rather than the formatted files")
	recursive := flags.Bool("r", false, "format the presentations in directories, and in the directories below them")
	flags.Usage = func() {
		fmt.Fprintf(stderr, "usage: present fmt [-l] [-w] [-d] [-r] [path...]\n\n"+
			"Fmt formats slide presentations and articles, and prints them. It\n"+
			"changes only spaces and blank lines, and never how a presentation\n"+
			"looks. Without paths, it formats standard input. A directory stands for\n"+
			"the presentations in it with -r, but for those in directories whose\n"+
			"names start with . or _.\n\nFlags:\n")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() == 0 {
		if *write {
			fmt.Fprintf(stderr, "present fmt: cannot use -w with standard input\n")
			return 2
		}
		src, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "present: %v\n", err)
			return 1
		}
		return formatFile("<standard input>", src, *list, false, *diff, stdout, stderr)
	}
	files, err := docFiles(flags.Args(), *recursive)
	if err != nil {
		fmt.Fprintf(stderr, "present: %v\n", err)
		return 1
	}
	code := 0
	for _, name := range files {
		src, err := os.ReadFile(name)
		if err != nil {
			fmt.Fprintf(stderr, "present: %v\n", err)
			code = 1
			continue
		}
		code = max(code, formatFile(name, src, *list, *write, *diff, stdout, stderr))
	}
	return code
}

// formatFile formats src, the contents of the file name, as the fmt command
// does with the flags -l, -w and -d, and returns the exit code.
func formatFile(name string, src []byte, list, write, diff bool, stdout, stderr io.Writer) int {
	formatted, err := present.Format(name, src)
	if err != nil {
		fmt.Fprintf(stderr, "present: %v\n", err)
		return 1
	}
	changed := !bytes.Equal(src, formatted)
	if list && changed {
		fmt.Fprintln(stdout, name)
	}
	if write && changed {
		fi, err := os.Stat(name)
		if err == nil {
			err = os.WriteFile(name, formatted, fi.Mode().Perm())
		}
		if err != nil {
			fmt.Fprintf(stderr, "present: %v\n", err)
			return 1
		}
	}
	if diff && changed {
		io.WriteString(stdout, udiff.Unified(name, name, string(src), string(formatted)))
	}
	if !list && !write && !diff {
		stdout.Write(formatted)
	}
	return 0
}
