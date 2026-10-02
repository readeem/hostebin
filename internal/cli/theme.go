package cli

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/readeem/hostebin/internal/theme"
)

func runTheme(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "set":
			return runThemeSet(args[1:], stderr)
		case "rm":
			return runThemeRM(args[1:], stderr)
		}
	}
	if len(args) > 1 {
		fmt.Fprintln(stderr, "usage: hostebin theme [default|omarchy|FILE] | set [omarchy|FILE] | rm")
		return exitUsage
	}
	spec := ""
	if len(args) == 1 {
		spec = args[0]
	}
	css, err := resolveTheme(spec)
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitUsage
	}
	_, _ = stdout.Write(css)
	return exitOK
}

func resolveTheme(spec string) ([]byte, error) {
	src, _, err := theme.Resolve(spec)
	if err != nil {
		return nil, err
	}
	return src()
}

func runThemeSet(args []string, stderr io.Writer) int {
	var user string
	leading, rest := takeLeadingArg(args)
	cfg, fs, ok := clientCommand("theme set", rest, stderr, func(fs *flag.FlagSet) {
		fs.StringVar(&user, "user", "", "user name or id")
	})
	if !ok {
		return exitUsage
	}
	spec := leading
	if leading == "" && fs.NArg() == 0 {
		ok = true
	} else {
		spec, ok = commandTarget(leading, fs)
	}
	if !ok {
		fmt.Fprintln(stderr, "usage: hostebin theme set [omarchy|FILE] [--user NAME]")
		return exitUsage
	}
	var css []byte
	var err error
	if spec == "" || spec == "omarchy" {
		css, err = resolveTheme("omarchy")
	} else {
		css, err = os.ReadFile(spec)
	}
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitUsage
	}
	return sendTheme(cfg, user, http.MethodPut, css, stderr)
}

func runThemeRM(args []string, stderr io.Writer) int {
	var user string
	cfg, fs, ok := clientCommand("theme rm", args, stderr, func(fs *flag.FlagSet) {
		fs.StringVar(&user, "user", "", "user name or id")
	})
	if !ok || fs.NArg() != 0 {
		return exitUsage
	}
	return sendTheme(cfg, user, http.MethodDelete, nil, stderr)
}

func sendTheme(cfg *Config, user, method string, css []byte, stderr io.Writer) int {
	id, err := resolveTargetUserID(cfg, user)
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitNetwork
	}
	endpoint := cfg.Server + "/api/v1/users/" + url.PathEscape(id) + "/theme"
	body, status, reqErr := requestBody(method, endpoint, cfg.Token, "text/css; charset=utf-8", css)
	if !successful(body, status, reqErr, stderr) {
		return exitNetwork
	}
	return exitOK
}
