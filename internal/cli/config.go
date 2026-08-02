package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type clientConfig struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

func resolveClientConfig(flagURL, flagToken string) (clientConfig, error) {
	var fileCfg clientConfig
	path, err := configPath()
	if err == nil {
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			if err := json.Unmarshal(data, &fileCfg); err != nil {
				return clientConfig{}, fmt.Errorf("read %s: %w", path, err)
			}
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return clientConfig{}, readErr
		}
	}
	cfg := fileCfg
	if value := os.Getenv("HOSTEBIN_URL"); value != "" {
		cfg.URL = value
	}
	if value := os.Getenv("HOSTEBIN_TOKEN"); value != "" {
		cfg.Token = value
	}
	if flagURL != "" {
		cfg.URL = flagURL
	}
	if flagToken != "" {
		cfg.Token = flagToken
	}
	cfg.URL = strings.TrimRight(cfg.URL, "/")
	if cfg.URL == "" {
		return cfg, errors.New("server URL is required (--server, HOSTEBIN_URL, or config.json)")
	}
	if cfg.Token == "" {
		return cfg, errors.New("token is required (--token, HOSTEBIN_TOKEN, or config.json)")
	}
	return cfg, nil
}

func configPath() (string, error) {
	if root := os.Getenv("XDG_CONFIG_HOME"); root != "" {
		return filepath.Join(root, "hostebin", "config.json"), nil
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "hostebin", "config.json"), nil
}
