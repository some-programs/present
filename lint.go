package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/some-programs/present/pkg/present"
)

// lint runs the lint command, which checks presentations for problems.
func lint(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lint", flag.ContinueOnError)
	flags.SetOutput(stderr)
	asJSON := flags.Bool("json", false, "print the problems as a JSON array of objects with the fields file, line, rule, severity and message")
	recursive := flags.Bool("r", false, "check the presentations in directories, and in the directories below them")
	flags.Usage = func() {
		fmt.Fprintf(stderr, "usage: present lint [-json] [-r] path...\n\n"+
			"Lint checks slide presentations and articles for problems, such as\n"+
			"commands with files that do not exist, and prints each as\n"+
			"file:line: message (rule). It exits with status 1 if it finds any.\n"+
			"A directory stands for the presentations in it with -r, but for those\n"+
			"in directories whose names start with . or _. The rules are listed in\n"+
			"the documentation of present.Lint, in the present package.\n\nFlags:\n")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() == 0 {
		flags.Usage()
		return 2
	}
	files, err := docFiles(flags.Args(), *recursive)
	if err != nil {
		fmt.Fprintf(stderr, "present: %v\n", err)
		return 1
	}
	code := 0
	problems := []present.Problem{}
	for _, name := range files {
		src, err := os.ReadFile(name)
		if err != nil {
			fmt.Fprintf(stderr, "present: %v\n", err)
			code = 1
			continue
		}
		problems = append(problems, present.Lint(name, src)...)
	}
	if *asJSON {
		if err := json.MarshalWrite(stdout, problems, jsontext.WithIndent("  ")); err != nil {
			fmt.Fprintf(stderr, "present: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout)
	} else {
		for _, p := range problems {
			fmt.Fprintln(stdout, p)
		}
	}
	if len(problems) > 0 {
		code = 1
	}
	return code
}
