package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunServeExplainsWhenPortZeroDisablesTheOnlyListener(t *testing.T) {
	setUserConfigRoot(t, t.TempDir())
	configFile := filepath.Join(t.TempDir(), "config.json")
	contents, err := json.Marshal(map[string]any{
		"data": t.TempDir(),
		"port": 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	if got := runServe([]string{"--config", configFile}, &stderr); got != exitUsage {
		t.Fatalf("exit code = %d, want %d; stderr: %s", got, exitUsage, stderr.String())
	}
	for _, want := range []string{"port is 0", "HTTP listener is disabled", "set --port"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
		}
	}
}

func TestBootstrapTokenFileCompatibility(t *testing.T) {
	dataDir := t.TempDir()
	got, err := loadBootstrapToken(dataDir, "")
	if err != nil || got != "" {
		t.Fatalf("missing token = %q, %v", got, err)
	}
	if err := writeBootstrapToken(dataDir, "legacy-or-generated-token"); err != nil {
		t.Fatal(err)
	}
	got, err = loadBootstrapToken(dataDir, "")
	if err != nil || got != "legacy-or-generated-token" {
		t.Fatalf("stored token = %q, %v", got, err)
	}
	got, err = loadBootstrapToken(dataDir, "configured-token")
	if err != nil || got != "configured-token" {
		t.Fatalf("configured token = %q, %v", got, err)
	}
	info, err := os.Stat(filepath.Join(dataDir, "token"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token mode = %v", info.Mode().Perm())
	}
}
