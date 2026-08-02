package cli

import (
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// setUserConfigRoot points os.UserConfigDir at root for the duration of the
// test. Each platform reads a different variable, so setting XDG_CONFIG_HOME
// alone would silently leave macOS and Windows writing to the real user
// configuration directory.
func setUserConfigRoot(t *testing.T, root string) {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", root)
	case "darwin", "ios":
		t.Setenv("HOME", root)
	default:
		t.Setenv("XDG_CONFIG_HOME", root)
	}
}

func TestNewConfigUsesGlobalDirectoriesAndAutogeneratesJSON(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataRoot)
	// os.UserConfigDir reads a different variable per platform, so redirect the
	// one that applies here and derive the expectation from the same rule.
	setUserConfigRoot(t, t.TempDir())
	configRoot, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := NewConfig()
	if err != nil {
		t.Fatal(err)
	}
	wantConfig := filepath.Join(configRoot, "hostebin", "config.json")
	wantData := filepath.Join(dataRoot, "hostebin")
	if cfg.ConfigFile != wantConfig {
		t.Fatalf("ConfigFile = %q, want %q", cfg.ConfigFile, wantConfig)
	}
	if cfg.Data != wantData {
		t.Fatalf("Data = %q, want %q", cfg.Data, wantData)
	}

	info, err := os.Stat(wantConfig)
	if err != nil {
		t.Fatal(err)
	}
	// Windows has no POSIX permission bits; os.Stat synthesizes 0666 for any
	// writable file, so the 0600 guarantee is only meaningful elsewhere.
	if runtime.GOOS != "windows" {
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("config mode = %o, want 600", got)
		}
	}
	data, err := os.ReadFile(wantConfig)
	if err != nil {
		t.Fatal(err)
	}
	var generated map[string]any
	if err := json.Unmarshal(data, &generated); err != nil {
		t.Fatalf("generated config is not JSON: %v", err)
	}
	if generated["data"] != wantData {
		t.Fatalf("generated data = %#v, want %q", generated["data"], wantData)
	}
	if generated["max-upload"] != "32MiB" || generated["max-files"] != float64(64) {
		t.Fatalf("generated limits = %#v, %#v", generated["max-upload"], generated["max-files"])
	}
	if generated["host"] != "" || generated["port"] != float64(8080) {
		t.Fatalf("generated HTTP listener = %#v, %#v", generated["host"], generated["port"])
	}
	if _, ok := generated["addr"]; ok {
		t.Fatal("generated config contains removed addr option")
	}
}

func TestConfigPrecedenceCLIThenEnvironmentThenJSON(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.json")
	contents := []byte(`{
  "host": "file-host",
  "port": 8081,
  "base-url": "https://from-file.example",
  "max-files": 12,
  "max-upload": "2MiB",
  "tailscale": true
}`)
	if err := os.WriteFile(configFile, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOSTEBIN_HOST", "env-host")
	t.Setenv("HOSTEBIN_PORT", "8082")
	t.Setenv("HOSTEBIN_MAX_FILES", "23")

	cfg, err := NewConfig(WithConfigFile(configFile))
	if err != nil {
		t.Fatal(err)
	}
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	cfg.registerServeFlags(fs)
	if err := parseConfig(fs, []string{"--host", "cli-host", "--max-upload", "4MiB", "--tailscale=false"}); err != nil {
		t.Fatal(err)
	}

	if cfg.HTTPHost != "cli-host" {
		t.Fatalf("HTTPHost = %q, want CLI value", cfg.HTTPHost)
	}
	if cfg.HTTPPort != 8082 {
		t.Fatalf("HTTPPort = %d, want environment value", cfg.HTTPPort)
	}
	if cfg.MaxFiles != 23 {
		t.Fatalf("MaxFiles = %d, want environment value", cfg.MaxFiles)
	}
	if cfg.MaxUpload != "4MiB" {
		t.Fatalf("MaxUpload = %q, want CLI value", cfg.MaxUpload)
	}
	if cfg.BaseURL != "https://from-file.example" {
		t.Fatalf("BaseURL = %q, want config value", cfg.BaseURL)
	}
	if cfg.Tailscale {
		t.Fatal("Tailscale = true, want explicit CLI false")
	}
}

func TestClientConfigEnvironmentAndCLIPrecedence(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configFile, []byte(`{"server":"https://from-file.example","token":"file-token"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOSTEBIN_SERVER", "https://from-env.example/")

	cfg, err := NewConfig(WithConfigFile(configFile))
	if err != nil {
		t.Fatal(err)
	}
	fs := flag.NewFlagSet("client", flag.ContinueOnError)
	cfg.registerClientFlags(fs)
	if err := parseConfig(fs, []string{"--token", "cli-token"}); err != nil {
		t.Fatal(err)
	}
	if err := resolveClientConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Server != "https://from-env.example" {
		t.Fatalf("Server = %q, want environment value", cfg.Server)
	}
	if cfg.Token != "cli-token" {
		t.Fatalf("Token = %q, want CLI value", cfg.Token)
	}
}

func TestNewConfigOptionsCanDisableAutogeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	cfg, err := NewConfig(WithConfigFile(path), WithDataDir("/custom/data"), WithConfigAutogeneration(false))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Data != "/custom/data" {
		t.Fatalf("Data = %q", cfg.Data)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config stat error = %v, want not-exist", err)
	}
}

func TestConfigRejectsUnknownJSONKeys(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configFile, []byte(`{"max-fiels":64}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := NewConfig(WithConfigFile(configFile))
	if err != nil {
		t.Fatal(err)
	}
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfg.registerServeFlags(fs)
	if err := parseConfig(fs, nil); err == nil {
		t.Fatal("parseConfig succeeded with an unknown JSON key")
	}
}

func TestHTTPListenAddr(t *testing.T) {
	tests := []struct {
		name string
		host string
		port int
		want string
		err  bool
	}{
		{name: "all interfaces", port: 8080, want: ":8080"},
		{name: "IPv4", host: "127.0.0.1", port: 9000, want: "127.0.0.1:9000"},
		{name: "IPv6", host: "::1", port: 9000, want: "[::1]:9000"},
		{name: "disabled", host: "127.0.0.1", port: 0, want: ""},
		{name: "negative", port: -1, err: true},
		{name: "too large", port: 65536, err: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := httpListenAddr(tt.host, tt.port)
			if (err != nil) != tt.err {
				t.Fatalf("error = %v, want error %v", err, tt.err)
			}
			if got != tt.want {
				t.Fatalf("address = %q, want %q", got, tt.want)
			}
		})
	}
}
