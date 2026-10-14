// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"embed"
	"fmt"
	"io"
	"os"
	"strings"
)

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
	commands = append(commands, command{"export", "write presentations as static HTML pages", export})
	commands = append(commands, command{"fmt", "format presentations", func(args []string, stdout, stderr io.Writer) int {
		return format(args, os.Stdin, stdout, stderr)
	}})
	commands = append(commands, command{"lint", "check presentations for problems", lint})
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
