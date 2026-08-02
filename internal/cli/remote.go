package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type listedBundle struct {
	ID, Title, Entry string
	CreatedAt        time.Time  `json:"created_at"`
	ExpiresAt        *time.Time `json:"expires_at"`
	Bytes            int64
	Files            []any
}

func runLS(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var serverURL, token string
	var jsonOutput bool
	fs.StringVar(&serverURL, "server", "", "hostebin server URL")
	fs.StringVar(&token, "token", "", "upload bearer token")
	fs.BoolVar(&jsonOutput, "json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: hostebin ls [flags]")
		return exitUsage
	}
	cfg, err := resolveClientConfig(serverURL, token)
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitUsage
	}
	body, status, err := request(http.MethodGet, cfg.URL+"/api/v1/bundles", cfg.Token)
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitNetwork
	}
	if status < 200 || status >= 300 {
		fmt.Fprintf(stderr, "hostebin: server returned HTTP %d: %s\n", status, strings.TrimSpace(string(body)))
		return exitNetwork
	}
	if jsonOutput {
		var value any
		if json.Unmarshal(body, &value) != nil {
			fmt.Fprintln(stderr, "hostebin: invalid server response")
			return exitNetwork
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(value)
		return exitOK
	}
	var bundles []listedBundle
	if err := json.Unmarshal(body, &bundles); err != nil {
		fmt.Fprintln(stderr, "hostebin: invalid server response:", err)
		return exitNetwork
	}
	for _, b := range bundles {
		title := b.Title
		if title == "" {
			title = b.Entry
		}
		fmt.Fprintf(stdout, "%s\t%d\t%s\t%s\n", b.ID, b.Bytes, b.CreatedAt.Format(time.RFC3339), title)
	}
	return exitOK
}

func runRM(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("rm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var serverURL, token string
	fs.StringVar(&serverURL, "server", "", "hostebin server URL")
	fs.StringVar(&token, "token", "", "upload bearer token")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: hostebin rm [flags] <id>")
		return exitUsage
	}
	cfg, err := resolveClientConfig(serverURL, token)
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitUsage
	}
	body, status, err := request(http.MethodDelete, cfg.URL+"/api/v1/bundles/"+url.PathEscape(fs.Arg(0)), cfg.Token)
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitNetwork
	}
	if status < 200 || status >= 300 {
		fmt.Fprintf(stderr, "hostebin: server returned HTTP %d: %s\n", status, strings.TrimSpace(string(body)))
		return exitNetwork
	}
	_ = stdout
	return exitOK
}

func request(method, endpoint, token string) ([]byte, int, error) {
	req, err := http.NewRequest(method, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}
