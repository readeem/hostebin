package cli

import (
	"bytes"
	"cmp"
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
	OwnerID          string     `json:"owner_id"`
	Owner            string     `json:"owner"`
	CreatedAt        time.Time  `json:"created_at"`
	ExpiresAt        *time.Time `json:"expires_at"`
	Bytes            int64
	Files            []any
}

func runLS(args []string, stdout, stderr io.Writer) int {
	cfg, err := NewConfig()
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitUsage
	}
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var jsonOutput, all bool

	cfg.registerClientFlags(fs)
	boolVar(fs, &jsonOutput, "json", "print JSON")
	boolVar(fs, &all, "all", "list every user's bundles (admin only)")

	if err := parseConfig(fs, args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: hostebin ls [flags]")
		return exitUsage
	}
	if err := resolveClientConfig(cfg); err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitUsage
	}
	endpoint := cfg.Server + "/api/v1/bundles"
	if all {
		endpoint += "?all=1"
	}
	body, status, err := request(http.MethodGet, endpoint, cfg.Token)
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
		// The server ignores ?all=1 for non-admins and strips the owner, so the
		// column only appears when it actually carries something.
		owner := cmp.Or(b.Owner, b.OwnerID)
		if all && owner != "" {
			fmt.Fprintf(stdout, "%s\t%s\t%d\t%s\t%s\n", b.ID, owner, b.Bytes, b.CreatedAt.Format(time.RFC3339), title)
		} else {
			fmt.Fprintf(stdout, "%s\t%d\t%s\t%s\n", b.ID, b.Bytes, b.CreatedAt.Format(time.RFC3339), title)
		}
	}
	return exitOK
}

func runRM(args []string, stdout, stderr io.Writer) int {
	cfg, err := NewConfig()
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitUsage
	}
	fs := flag.NewFlagSet("rm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg.registerClientFlags(fs)

	if err := parseConfig(fs, args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: hostebin rm [flags] <id>")
		return exitUsage
	}
	if err := resolveClientConfig(cfg); err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitUsage
	}
	body, status, err := request(http.MethodDelete, cfg.Server+"/api/v1/bundles/"+url.PathEscape(fs.Arg(0)), cfg.Token)
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
	return requestJSON(method, endpoint, token, nil)
}

func requestJSON(method, endpoint, token string, value any) ([]byte, int, error) {
	var requestBody io.Reader
	if value != nil {
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, 0, err
		}
		requestBody = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, endpoint, requestBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if value != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}
