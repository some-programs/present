package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/some-programs/present/internal/legacy"
)

// convertLegacy runs the convert-legacy command, which converts
// presentations from the legacy present markup to Markdown.
func convertLegacy(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("convert-legacy", flag.ContinueOnError)
	flags.SetOutput(stderr)
	write := flags.Bool("w", false, "write the results to the files rather than to standard output")
	flags.Usage = func() {
		fmt.Fprintf(stderr, "usage: present convert-legacy [-w] file...\n\n"+
			"Convert-legacy converts a presentation in the legacy present markup to\n"+
			"Markdown, and prints it. With -w, it converts each file in place, and\n"+
			"skips those already in Markdown.\n\nFlags:\n")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() == 0 || (!*write && flags.NArg() > 1) {
		flags.Usage()
		return 2
	}
	code := 0
	for _, name := range flags.Args() {
		data, err := os.ReadFile(name)
		if err != nil {
			fmt.Fprintf(stderr, "present: %v\n", err)
			code = 1
			continue
		}
		if *write && !legacy.IsLegacy(data) {
			fmt.Fprintf(stdout, "Skipped %s, already in Markdown.\n", name)
			continue
		}
		data, err = legacy.Convert(name, data)
		if err == nil {
			if *write {
				err = os.WriteFile(name, data, 0o644)
			} else {
				_, err = stdout.Write(data)
			}
		}
		if err != nil {
			fmt.Fprintf(stderr, "present: %v\n", err)
			code = 1
			continue
		}
		if *write {
			fmt.Fprintf(stdout, "Converted %s.\n", name)
		}
	}
	return code
}
