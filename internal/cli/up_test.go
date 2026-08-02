package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"net/http/httptest"

	"github.com/hostebin/hostebin/internal/server"
	"github.com/hostebin/hostebin/internal/store"
)

func cliTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, _ := store.New(t.TempDir())
	app, err := server.New(server.Config{Store: st, Token: "cli-token"})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(app.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestUpStdoutAndJSON(t *testing.T) {
	ts := cliTestServer(t)
	t.Setenv("HOSTEBIN_URL", ts.URL)
	t.Setenv("HOSTEBIN_TOKEN", "cli-token")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
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
	t.Setenv("HOSTEBIN_URL", ts.URL)
	t.Setenv("HOSTEBIN_TOKEN", "cli-token")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
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
