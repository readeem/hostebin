package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/peterbourgon/ff/v3"
)

// Config is the complete persistent configuration surface. JSON keys match
// their command-line flag names so ff can apply config, environment, and CLI
// values without maintaining three separate mappings.
type Config struct {
	ConfigFile string `json:"-"`

	Server string `json:"server"`
	Token  string `json:"token"`

	HTTPHost     string `json:"host"`
	HTTPPort     int    `json:"port"`
	Data         string `json:"data"`
	BaseURL      string `json:"base-url"`
	TLSAddr      string `json:"tls-addr"`
	TLSCert      string `json:"tls-cert"`
	TLSKey       string `json:"tls-key"`
	ACMEDomain   string `json:"acme-domain"`
	ACMEEmail    string `json:"acme-email"`
	Tailscale    bool   `json:"tailscale"`
	Funnel       bool   `json:"funnel"`
	TSHostname   string `json:"ts-hostname"`
	TSAuthKey    string `json:"ts-auth-key"`
	MaxUpload    string `json:"max-upload"`
	MaxFiles     int    `json:"max-files"`
	DefaultTTL   string `json:"default-ttl"`
	CSP          string `json:"csp"`
	autogenerate bool   `json:"-"`
}

// ConfigOpt customizes construction of the global configuration. Keeping path
// policy here makes Config reusable and makes tests independent of the real
// user directories.
type ConfigOpt func(*Config)

func WithConfigFile(path string) ConfigOpt {
	return func(cfg *Config) { cfg.ConfigFile = path }
}

func WithDataDir(path string) ConfigOpt {
	return func(cfg *Config) { cfg.Data = path }
}

func WithConfigAutogeneration(enabled bool) ConfigOpt {
	return func(cfg *Config) { cfg.autogenerate = enabled }
}

// NewConfig returns defaults rooted in the current user's global directories.
// The default JSON file is generated on first use unless an option disables it.
func NewConfig(opts ...ConfigOpt) (*Config, error) {
	cfg := &Config{
		HTTPPort:     8080,
		TLSAddr:      ":8443",
		TSHostname:   "hostebin",
		MaxUpload:    "32MiB",
		MaxFiles:     64,
		DefaultTTL:   "never",
		autogenerate: true,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.ConfigFile == "" {
		configFile, err := defaultConfigFile()
		if err != nil {
			return nil, fmt.Errorf("resolve config directory: %w", err)
		}
		cfg.ConfigFile = configFile
	}
	if cfg.Data == "" {
		dataDir, err := defaultDataDir()
		if err != nil {
			return nil, fmt.Errorf("resolve data directory: %w", err)
		}
		cfg.Data = dataDir
	}
	if cfg.autogenerate {
		if err := generateConfigFile(cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func defaultConfigFile() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "hostebin", "config.json"), nil
}

func defaultDataDir() (string, error) {
	if root := os.Getenv("XDG_DATA_HOME"); root != "" {
		return filepath.Join(root, "hostebin"), nil
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd" || runtime.GOOS == "netbsd" {
		root, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(root, ".local", "share", "hostebin"), nil
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "hostebin", "data"), nil
}

func generateConfigFile(cfg *Config) error {
	if _, err := os.Stat(cfg.ConfigFile); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect %s: %w", cfg.ConfigFile, err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.ConfigFile), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	f, err := os.OpenFile(cfg.ConfigFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) { // another process won the first-run race
		return nil
	}
	if err != nil {
		return fmt.Errorf("create %s: %w", cfg.ConfigFile, err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	writeErr := enc.Encode(cfg)
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(cfg.ConfigFile)
		return fmt.Errorf("write %s: %w", cfg.ConfigFile, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", cfg.ConfigFile, closeErr)
	}
	return nil
}

func (cfg *Config) registerConfigFlag(fs *flag.FlagSet) {
	fs.StringVar(&cfg.ConfigFile, "config", cfg.ConfigFile, "JSON configuration file")
}

func (cfg *Config) registerClientFlags(fs *flag.FlagSet) {
	cfg.registerConfigFlag(fs)
	fs.StringVar(&cfg.Server, "server", cfg.Server, "hostebin server URL")
	fs.StringVar(&cfg.Token, "token", cfg.Token, "upload bearer token")
}

func (cfg *Config) registerServeFlags(fs *flag.FlagSet) {
	cfg.registerConfigFlag(fs)
	fs.StringVar(&cfg.HTTPHost, "host", cfg.HTTPHost, "plain HTTP listen host")
	fs.IntVar(&cfg.HTTPPort, "port", cfg.HTTPPort, "plain HTTP listen port; 0 disables it")
	fs.StringVar(&cfg.Data, "data", cfg.Data, "data directory")
	fs.StringVar(&cfg.BaseURL, "base-url", cfg.BaseURL, "public base URL override")
	fs.StringVar(&cfg.TLSAddr, "tls-addr", cfg.TLSAddr, "certificate TLS listen address")
	fs.StringVar(&cfg.TLSCert, "tls-cert", cfg.TLSCert, "TLS certificate file")
	fs.StringVar(&cfg.TLSKey, "tls-key", cfg.TLSKey, "TLS private key file")
	fs.StringVar(&cfg.ACMEDomain, "acme-domain", cfg.ACMEDomain, "Let's Encrypt domain")
	fs.StringVar(&cfg.ACMEEmail, "acme-email", cfg.ACMEEmail, "Let's Encrypt account email")
	boolVar(fs, &cfg.Tailscale, "tailscale", "enable embedded Tailscale")
	boolVar(fs, &cfg.Funnel, "funnel", "enable Tailscale Funnel")
	fs.StringVar(&cfg.TSHostname, "ts-hostname", cfg.TSHostname, "Tailscale node hostname")
	fs.StringVar(&cfg.TSAuthKey, "ts-auth-key", cfg.TSAuthKey, "Tailscale auth key")
	fs.StringVar(&cfg.Token, "token", cfg.Token, "upload bearer token")
	fs.StringVar(&cfg.MaxUpload, "max-upload", cfg.MaxUpload, "maximum upload size")
	fs.IntVar(&cfg.MaxFiles, "max-files", cfg.MaxFiles, "maximum files per bundle")
	fs.StringVar(&cfg.DefaultTTL, "default-ttl", cfg.DefaultTTL, "default expiry duration or never")
	fs.StringVar(&cfg.CSP, "csp", cfg.CSP, "Content-Security-Policy value; off disables it")
}

// parseConfig applies ff's native precedence: CLI, environment, then JSON.
func parseConfig(fs *flag.FlagSet, args []string) error {
	err := ff.Parse(fs, args,
		ff.WithEnvVarPrefix("HOSTEBIN"),
		ff.WithConfigFileFlag("config"),
		ff.WithConfigFileParser(configJSONParser),
		ff.WithIgnoreUndefined(true),
	)
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(fs.Output(), "hostebin:", err)
	}
	return err
}

func configJSONParser(r io.Reader, set func(name, value string) error) error {
	return ff.JSONParser(r, func(name, value string) error {
		if _, ok := configKeys[name]; !ok {
			return fmt.Errorf("unknown config key %q", name)
		}
		return set(name, value)
	})
}

var configKeys = map[string]struct{}{
	"server": {}, "token": {},
	"host": {}, "port": {}, "data": {}, "base-url": {},
	"tls-addr": {}, "tls-cert": {}, "tls-key": {},
	"acme-domain": {}, "acme-email": {},
	"tailscale": {}, "funnel": {}, "ts-hostname": {}, "ts-auth-key": {},
	"max-upload": {}, "max-files": {}, "default-ttl": {}, "csp": {},
}

func resolveClientConfig(cfg *Config) error {
	cfg.Server = strings.TrimRight(cfg.Server, "/")
	if cfg.Server == "" {
		return errors.New("server URL is required (--server, HOSTEBIN_SERVER, or config.json)")
	}
	if cfg.Token == "" {
		return errors.New("token is required (--token, HOSTEBIN_TOKEN, or config.json)")
	}
	return nil
}

type flexibleBoolValue struct{ target *bool }

func boolVar(fs *flag.FlagSet, target *bool, name, usage string) {
	fs.Var(flexibleBoolValue{target}, name, usage)
}

func (v flexibleBoolValue) String() string {
	if v.target == nil {
		return "false"
	}
	return strconv.FormatBool(*v.target)
}
func (v flexibleBoolValue) IsBoolFlag() bool { return true }
func (v flexibleBoolValue) Set(raw string) error {
	value, err := parseBool(raw)
	if err != nil {
		return err
	}
	*v.target = value
	return nil
}

func parseBool(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", raw)
	}
}
