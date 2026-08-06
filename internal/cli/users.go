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

type remoteToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

type remoteUser struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Role      string       `json:"role"`
	CreatedAt time.Time    `json:"created_at"`
	Disabled  bool         `json:"disabled"`
	Token     *remoteToken `json:"token"`
}

type remotePrincipal struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Role       string `json:"role"`
	TokenID    string `json:"token_id"`
	TokenLabel string `json:"token_label"`
}

func runUser(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: hostebin user <ls|add|rm|disable|enable> [flags]")
		return exitUsage
	}
	switch args[0] {
	case "ls":
		return runUserLS(args[1:], stdout, stderr)
	case "add":
		return runUserAdd(args[1:], stdout, stderr)
	case "rm":
		return runUserRM(args[1:], stderr)
	case "disable":
		return runUserToggle(args[1:], true, stderr)
	case "enable":
		return runUserToggle(args[1:], false, stderr)
	default:
		fmt.Fprintf(stderr, "hostebin: unknown user command %q\n", args[0])
		return exitUsage
	}
}

func runUserLS(args []string, stdout, stderr io.Writer) int {
	var jsonOutput bool
	cfg, fs, ok := clientCommand("user ls", args, stderr, func(fs *flag.FlagSet) {
		boolVar(fs, &jsonOutput, "json", "print JSON")
	})
	if !ok {
		return exitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: hostebin user ls [--json]")
		return exitUsage
	}
	body, status, err := request(http.MethodGet, cfg.Server+"/api/v1/users", cfg.Token)
	if !successful(body, status, err, stderr) {
		return exitNetwork
	}
	if jsonOutput {
		return printJSON(body, stdout, stderr)
	}
	var all []remoteUser
	if err := json.Unmarshal(body, &all); err != nil {
		fmt.Fprintln(stderr, "hostebin: invalid server response:", err)
		return exitNetwork
	}
	for _, user := range all {
		state := "enabled"
		if user.Disabled {
			state = "disabled"
		}
		tokenID := "-"
		if user.Token != nil {
			tokenID = user.Token.ID
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\n", user.ID, user.Name, user.Role, state, tokenID)
	}
	return exitOK
}

func runUserAdd(args []string, stdout, stderr io.Writer) int {
	leading, args := takeLeadingArg(args)
	var admin, jsonOutput bool
	var label, ttl string
	cfg, fs, ok := clientCommand("user add", args, stderr, func(fs *flag.FlagSet) {
		boolVar(fs, &admin, "admin", "create an admin")
		boolVar(fs, &jsonOutput, "json", "print JSON")
		fs.StringVar(&label, "label", "initial", "first token label")
		fs.StringVar(&ttl, "ttl", "never", "first token expiry")
	})
	if !ok {
		return exitUsage
	}
	name, ok := commandTarget(leading, fs)
	if !ok {
		fmt.Fprintln(stderr, "usage: hostebin user add <name> [--admin] [--label L] [--ttl D] [--json]")
		return exitUsage
	}
	role := "user"
	if admin {
		role = "admin"
	}
	body, status, reqErr := requestJSON(http.MethodPost, cfg.Server+"/api/v1/users", cfg.Token, map[string]any{"name": name, "role": role, "label": label, "ttl": ttl})
	if !successful(body, status, reqErr, stderr) {
		return exitNetwork
	}
	if jsonOutput {
		return printJSON(body, stdout, stderr)
	}
	var response struct {
		User      remoteUser  `json:"user"`
		Token     remoteToken `json:"token"`
		Plaintext string      `json:"plaintext"`
	}
	if json.Unmarshal(body, &response) != nil || response.Plaintext == "" {
		fmt.Fprintln(stderr, "hostebin: invalid server response")
		return exitNetwork
	}
	expiry := "never"
	if response.Token.ExpiresAt != nil {
		expiry = response.Token.ExpiresAt.Format("2006-01-02")
	}
	fmt.Fprintf(stderr, "hostebin: created user %s (%s), token %s, expires %s\n", response.User.Name, response.User.ID, response.Token.ID, expiry)
	fmt.Fprintln(stdout, response.Plaintext)
	return exitOK
}

func runUserRM(args []string, stderr io.Writer) int {
	leading, args := takeLeadingArg(args)
	var bundles string
	cfg, fs, ok := clientCommand("user rm", args, stderr, func(fs *flag.FlagSet) {
		fs.StringVar(&bundles, "bundles", "", "delete or reassign owned bundles")
	})
	if !ok {
		return exitUsage
	}
	target, ok := commandTarget(leading, fs)
	if !ok {
		fmt.Fprintln(stderr, "usage: hostebin user rm <name|id> [--bundles delete|reassign]")
		return exitUsage
	}
	id, err := resolveUserID(cfg, target)
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitNetwork
	}
	endpoint := cfg.Server + "/api/v1/users/" + url.PathEscape(id)
	if bundles != "" {
		endpoint += "?bundles=" + url.QueryEscape(bundles)
	}
	body, status, reqErr := request(http.MethodDelete, endpoint, cfg.Token)
	if !successful(body, status, reqErr, stderr) {
		return exitNetwork
	}
	return exitOK
}

func runUserToggle(args []string, disabled bool, stderr io.Writer) int {
	leading, args := takeLeadingArg(args)
	cfg, fs, ok := clientCommand("user disable|enable", args, stderr, nil)
	if !ok {
		return exitUsage
	}
	target, ok := commandTarget(leading, fs)
	if !ok {
		fmt.Fprintln(stderr, "usage: hostebin user disable|enable <name|id>")
		return exitUsage
	}
	id, err := resolveUserID(cfg, target)
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitNetwork
	}
	body, status, reqErr := requestJSON(http.MethodPatch, cfg.Server+"/api/v1/users/"+url.PathEscape(id), cfg.Token, map[string]bool{"disabled": disabled})
	if !successful(body, status, reqErr, stderr) {
		return exitNetwork
	}
	return exitOK
}

func runToken(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: hostebin token <new|rm> [flags]")
		return exitUsage
	}
	switch args[0] {
	case "new":
		return runTokenNew(args[1:], stdout, stderr)
	case "rm":
		return runTokenRM(args[1:], stderr)
	default:
		fmt.Fprintf(stderr, "hostebin: unknown token command %q\n", args[0])
		return exitUsage
	}
}

func runTokenNew(args []string, stdout, stderr io.Writer) int {
	var user, label, ttl string
	var jsonOutput bool
	cfg, fs, ok := clientCommand("token new", args, stderr, func(fs *flag.FlagSet) {
		fs.StringVar(&user, "user", "", "user name or id")
		fs.StringVar(&label, "label", "token", "replacement token label")
		fs.StringVar(&ttl, "ttl", "never", "token expiry")
		boolVar(fs, &jsonOutput, "json", "print JSON")
	})
	if !ok || fs.NArg() != 0 {
		return exitUsage
	}
	id, err := resolveTargetUserID(cfg, user)
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitNetwork
	}
	body, status, reqErr := requestJSON(http.MethodPut, cfg.Server+"/api/v1/users/"+url.PathEscape(id)+"/token", cfg.Token, map[string]string{"label": label, "ttl": ttl})
	if !successful(body, status, reqErr, stderr) {
		return exitNetwork
	}
	if jsonOutput {
		return printJSON(body, stdout, stderr)
	}
	var response struct {
		Token     remoteToken `json:"token"`
		Plaintext string      `json:"plaintext"`
	}
	if json.Unmarshal(body, &response) != nil || response.Plaintext == "" {
		fmt.Fprintln(stderr, "hostebin: invalid server response")
		return exitNetwork
	}
	fmt.Fprintf(stderr, "hostebin: rotated token %s for user %s\n", response.Token.ID, id)
	fmt.Fprintln(stdout, response.Plaintext)
	return exitOK
}

func runTokenRM(args []string, stderr io.Writer) int {
	var user string
	cfg, fs, ok := clientCommand("token rm", args, stderr, func(fs *flag.FlagSet) {
		fs.StringVar(&user, "user", "", "user name or id")
	})
	if !ok || fs.NArg() != 0 {
		return exitUsage
	}
	id, err := resolveTargetUserID(cfg, user)
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitNetwork
	}
	endpoint := cfg.Server + "/api/v1/users/" + url.PathEscape(id) + "/token"
	body, status, reqErr := request(http.MethodDelete, endpoint, cfg.Token)
	if !successful(body, status, reqErr, stderr) {
		return exitNetwork
	}
	return exitOK
}

func runWhoami(args []string, stdout, stderr io.Writer) int {
	var jsonOutput bool
	cfg, fs, ok := clientCommand("whoami", args, stderr, func(fs *flag.FlagSet) {
		boolVar(fs, &jsonOutput, "json", "print JSON")
	})
	if !ok {
		return exitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: hostebin whoami [--json]")
		return exitUsage
	}
	body, status, err := request(http.MethodGet, cfg.Server+"/api/v1/whoami", cfg.Token)
	if !successful(body, status, err, stderr) {
		return exitNetwork
	}
	if jsonOutput {
		return printJSON(body, stdout, stderr)
	}
	var p remotePrincipal
	if json.Unmarshal(body, &p) != nil {
		fmt.Fprintln(stderr, "hostebin: invalid server response")
		return exitNetwork
	}
	fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\n", p.ID, p.Name, p.Role, p.TokenID, p.TokenLabel)
	return exitOK
}

// clientCommand assembles a management subcommand: shared client flags, then
// whatever register adds, parsed and resolved into a ready-to-use Config. It
// returns the flag set so callers can inspect the remaining positional args.
func clientCommand(name string, args []string, stderr io.Writer, register func(*flag.FlagSet)) (*Config, *flag.FlagSet, bool) {
	cfg, err := NewConfig()
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return nil, nil, false
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg.registerClientFlags(fs)
	if register != nil {
		register(fs)
	}
	if parseConfig(fs, args) != nil {
		return nil, nil, false
	}
	if err := resolveClientConfig(cfg); err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return nil, nil, false
	}
	return cfg, fs, true
}

func successful(body []byte, status int, err error, stderr io.Writer) bool {
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return false
	}
	if status < 200 || status >= 300 {
		fmt.Fprintf(stderr, "hostebin: server returned HTTP %d: %s\n", status, strings.TrimSpace(string(body)))
		return false
	}
	return true
}

func printJSON(body []byte, stdout, stderr io.Writer) int {
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

func getJSON(cfg *Config, path string, dst any) error {
	body, status, err := request(http.MethodGet, cfg.Server+path, cfg.Token)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("server returned HTTP %d: %s", status, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("invalid server response: %w", err)
	}
	return nil
}

// resolveTargetUserID resolves target to a user ID, defaulting to the caller's
// own account when target is empty.
func resolveTargetUserID(cfg *Config, target string) (string, error) {
	if target != "" {
		return resolveUserID(cfg, target)
	}
	principal, err := self(cfg)
	if err != nil {
		return "", err
	}
	return principal.ID, nil
}

// resolveUserID maps a name or ID to a user ID. The caller's own identity is
// checked first because listing users is admin-only: without it a regular user
// could not name themselves in `token new --user <me>`.
func resolveUserID(cfg *Config, target string) (string, error) {
	if principal, err := self(cfg); err == nil {
		if target == principal.ID || strings.EqualFold(target, principal.Name) {
			return principal.ID, nil
		}
	}
	if strings.HasPrefix(target, "u_") {
		return target, nil
	}
	var all []remoteUser
	if err := getJSON(cfg, "/api/v1/users", &all); err != nil {
		return "", err
	}
	for _, user := range all {
		if strings.EqualFold(user.Name, target) {
			return user.ID, nil
		}
	}
	return "", fmt.Errorf("user %q not found", target)
}

func self(cfg *Config) (remotePrincipal, error) {
	var principal remotePrincipal
	if err := getJSON(cfg, "/api/v1/whoami", &principal); err != nil {
		return remotePrincipal{}, err
	}
	if principal.ID == "" {
		return remotePrincipal{}, fmt.Errorf("invalid server response")
	}
	return principal, nil
}

// takeLeadingArg splits off a positional argument written before the flags, so
// that `user rm bob --bundles delete` parses as well as the flags-first form.
func takeLeadingArg(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

// commandTarget resolves the single positional argument a command takes from
// either side of the flags. It reports false when none or more than one was
// given, which is always a usage error.
func commandTarget(leading string, fs *flag.FlagSet) (string, bool) {
	switch {
	case leading != "" && fs.NArg() == 0:
		return leading, true
	case leading == "" && fs.NArg() == 1:
		return fs.Arg(0), true
	default:
		return "", false
	}
}
