package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"net/http/httptest"

	"github.com/readeem/hostebin/internal/server"
	"github.com/readeem/hostebin/internal/store"
	"github.com/readeem/hostebin/internal/users"
	"github.com/readeem/hostebin/internal/users/filestore"
)

func cliTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	dataDir := t.TempDir()
	st, _ := store.New(dataDir)
	userStore, _ := filestore.Open(dataDir)
	t.Cleanup(func() { _ = userStore.Close() })
	userService := users.NewService(userStore)
	_, _, _ = userService.Bootstrap(t.Context(), "cli-token")
	app, err := server.New(server.Config{Store: st, Users: userService})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(app.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestUpStdoutAndJSON(t *testing.T) {
	ts := cliTestServer(t)
	t.Setenv("HOSTEBIN_SERVER", ts.URL)
	t.Setenv("HOSTEBIN_TOKEN", "cli-token")
	setUserConfigRoot(t, t.TempDir())
	file := filepath.Join(t.TempDir(), "plan.html")
	if err := os.WriteFile(file, []byte("<b>ok</b>"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"up", file}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	line := stdout.String()
	if !strings.HasPrefix(line, ts.URL+"/b/") || !strings.HasSuffix(line, "/\n") || strings.Count(line, "\n") != 1 {
		t.Fatalf("stdout = %q", line)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"up", "--json", file}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var value map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &value); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout.String(), err)
	}
}

func TestUpStdinRequiresNameAndUploads(t *testing.T) {
	ts := cliTestServer(t)
	t.Setenv("HOSTEBIN_SERVER", ts.URL)
	t.Setenv("HOSTEBIN_TOKEN", "cli-token")
	setUserConfigRoot(t, t.TempDir())
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"up", "-"}, strings.NewReader("# hi"), &stdout, &stderr); code != exitUsage {
		t.Fatalf("unnamed stdin code = %d", code)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"up", "-n", "note.md", "-"}, strings.NewReader("# hi"), &stdout, &stderr); code != exitOK {
		t.Fatalf("stdin code=%d stderr=%s", code, stderr.String())
	}
}

func TestUserAndTokenStdoutContracts(t *testing.T) {
	ts := cliTestServer(t)
	t.Setenv("HOSTEBIN_SERVER", ts.URL)
	t.Setenv("HOSTEBIN_TOKEN", "cli-token")
	setUserConfigRoot(t, t.TempDir())

	var stdout, stderr bytes.Buffer
	code := Run([]string{"user", "add", "bob", "--label", "agent", "--ttl", "1h"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("user add code=%d stderr=%s", code, stderr.String())
	}
	bobToken := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(bobToken, "hbt_") || strings.Count(stdout.String(), "\n") != 1 {
		t.Fatalf("user add stdout = %q", stdout.String())
	}

	t.Setenv("HOSTEBIN_TOKEN", bobToken)
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"whoami"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), "\tbob\tuser\t") {
		t.Fatalf("whoami code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"token", "new", "--label", "second"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.HasPrefix(stdout.String(), "hbt_") || strings.Count(stdout.String(), "\n") != 1 {
		t.Fatalf("token new code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	replacement := strings.TrimSpace(stdout.String())
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"whoami"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitNetwork {
		t.Fatalf("old token after rotation code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	t.Setenv("HOSTEBIN_TOKEN", replacement)
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"whoami"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), "\tbob\tuser\t") {
		t.Fatalf("replacement whoami code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"token", "ls"}, strings.NewReader(""), &stdout, &stderr); code != exitUsage {
		t.Fatalf("removed token ls code = %d", code)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"token", "rm"}, strings.NewReader(""), &stdout, &stderr); code != exitOK {
		t.Fatalf("token rm code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"whoami"}, strings.NewReader(""), &stdout, &stderr); code != exitNetwork {
		t.Fatalf("revoked token whoami code = %d", code)
	}
}
