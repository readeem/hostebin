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
