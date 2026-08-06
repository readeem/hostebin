package cli

import (
	"fmt"
	"io"

	"github.com/readeem/hostebin/internal/version"
)

const (
	exitOK      = 0
	exitUsage   = 1
	exitNetwork = 2
)

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
	case "user":
		return runUser(args[1:], stdout, stderr)
	case "token":
		return runToken(args[1:], stdout, stderr)
	case "whoami":
		return runWhoami(args[1:], stdout, stderr)
	case "version", "--version", "-version":
		fmt.Fprintln(stdout, version.String())
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

func usage(w io.Writer) {
	fmt.Fprint(w, `usage: hostebin <command> [flags]

Commands:
  up [flags] <file|dir|->...  upload a bundle and print its URL
  ls [flags]                  list live bundles
  rm [flags] <id>             delete a bundle
  user <command> [flags]      manage users
  token new|rm [flags]        rotate or revoke a token
  whoami [--json]             show the authenticated identity
  serve [flags]               run the server
  version                     print version information
  help                        print this message

Client flags:
  --server URL     server base URL      (HOSTEBIN_SERVER)
  --token TOKEN    bearer token         (HOSTEBIN_TOKEN)
  --config PATH    config file          (HOSTEBIN_CONFIG)

up flags:
  --title TEXT     bundle title
  --ttl DURATION   expiry such as 30m, 7d, or never
  --entry PATH     file served at the bundle root
  --id ID          replace an existing bundle in place
  -n, --name PATH  name to give data read from stdin
  --json           print the full JSON response instead of the URL
  --open           open the URL in the system browser
  --quiet          suppress optional diagnostics

Run "hostebin serve --help" for the full server flag list.
Documentation: https://github.com/readeem/hostebin
`)
}
