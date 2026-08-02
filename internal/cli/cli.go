package cli

import (
	"fmt"
	"io"
)

const (
	exitOK      = 0
	exitUsage   = 1
	exitNetwork = 2
)

var Version = "dev"

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "serve":
		return runServe(args[1:], stderr)
	case "up":
		return runUp(args[1:], stdin, stdout, stderr)
	case "ls":
		return runLS(args[1:], stdout, stderr)
	case "rm":
		return runRM(args[1:], stdout, stderr)
	case "version", "--version", "-version":
		fmt.Fprintln(stdout, Version)
		return exitOK
	case "help", "--help", "-h":
		usage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "hostebin: unknown command %q\n", args[0])
		usage(stderr)
		return exitUsage
	}
}

func usage(w io.Writer) { fmt.Fprintln(w, "usage: hostebin <serve|up|ls|rm|version> [flags]") }
