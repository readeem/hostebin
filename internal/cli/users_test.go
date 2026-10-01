package cli

import (
	"bytes"
	"flag"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandTargetAcceptsEitherArgumentPosition(t *testing.T) {
	parse := func(args []string) (string, bool) {
		leading, rest := takeLeadingArg(args)
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.SetOutput(&bytes.Buffer{})
		var flagValue string
		fs.StringVar(&flagValue, "bundles", "", "")
		if err := fs.Parse(rest); err != nil {
			return "", false
		}
		return commandTarget(leading, fs)
	}
	for _, tc := range []struct {
		args []string
		want string
		ok   bool
	}{
		{args: []string{"bob", "--bundles", "delete"}, want: "bob", ok: true},
		{args: []string{"--bundles", "delete", "bob"}, want: "bob", ok: true},
		{args: []string{"bob"}, want: "bob", ok: true},
		{args: nil},
		{args: []string{"--bundles", "delete"}},
		{args: []string{"bob", "extra"}},
	} {
		got, ok := parse(tc.args)
		if got != tc.want || ok != tc.ok {
			t.Errorf("commandTarget(%q) = %q, %v; want %q, %v", tc.args, got, ok, tc.want, tc.ok)
		}
	}
}

func TestUserLifecycleCommands(t *testing.T) {
	ts := cliTestServer(t)
	t.Setenv("HOSTEBIN_SERVER", ts.URL)
	t.Setenv("HOSTEBIN_TOKEN", "cli-token")
	setUserConfigRoot(t, t.TempDir())

	var stdout, stderr bytes.Buffer
	run := func(args ...string) int {
		stdout.Reset()
		stderr.Reset()
		return Run(args, strings.NewReader(""), &stdout, &stderr)
	}

	if code := run("user", "add", "carol"); code != exitOK {
		t.Fatalf("user add = %d, %s", code, stderr.String())
	}
	// Targeting by name must work with the flag on either side of it.
	if code := run("user", "disable", "carol"); code != exitOK {
		t.Fatalf("user disable = %d, %s", code, stderr.String())
	}
	if code := run("user", "enable", "carol"); code != exitOK {
		t.Fatalf("user enable = %d, %s", code, stderr.String())
	}
	if code := run("user", "ls"); code != exitOK || !strings.Contains(stdout.String(), "\tcarol\tuser\tenabled\t") {
		t.Fatalf("user ls = %d, %q", code, stdout.String())
	}
	if code := run("user", "rm", "--bundles", "delete", "carol"); code != exitOK {
		t.Fatalf("user rm = %d, %s", code, stderr.String())
	}
	if code := run("user", "ls"); code != exitOK || strings.Contains(stdout.String(), "carol") {
		t.Fatalf("user ls after rm = %d, %q", code, stdout.String())
	}
	if code := run("user", "rm"); code != exitUsage {
		t.Fatalf("user rm without target = %d", code)
	}
}

func TestThemeSetAndRemove(t *testing.T) {
	ts := cliTestServer(t)
	t.Setenv("HOSTEBIN_SERVER", ts.URL)
	t.Setenv("HOSTEBIN_TOKEN", "cli-token")
	setUserConfigRoot(t, t.TempDir())
	file := filepath.Join(t.TempDir(), "brand.css")
	if err := os.WriteFile(file, []byte(":root { --accent: #00ff00; }"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	run := func(args ...string) int {
		stdout.Reset()
		stderr.Reset()
		return Run(args, strings.NewReader(""), &stdout, &stderr)
	}
	if code := run("up", "-n", "a.md", "-"); code != exitOK {
		t.Fatalf("up = %d, %s", code, stderr.String())
	}
	themeURL := strings.TrimSpace(stdout.String()) + ".hostebin/theme.css"
	served := func() string {
		resp, err := http.Get(themeURL)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return string(body)
	}

	if code := run("theme", "set", file); code != exitOK {
		t.Fatalf("theme set = %d, %s", code, stderr.String())
	}
	if !strings.Contains(served(), "#00ff00") {
		t.Fatal("theme set did not reach the owner's bundles")
	}
	if code := run("theme", "rm"); code != exitOK {
		t.Fatalf("theme rm = %d, %s", code, stderr.String())
	}
	if strings.Contains(served(), "#00ff00") {
		t.Fatal("theme rm left the theme in place")
	}
}
