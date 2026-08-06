package cli

import (
	"os"
	"path/filepath"
	"testing"
)

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
