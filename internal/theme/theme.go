// Package theme resolves the stylesheet that colours every page hostebin
// renders, and every uploaded page that links /.hostebin/theme.css.
package theme

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Default is the built-in palette. Every theme is applied on top of it.
//
//go:embed default.css
var Default []byte

// Source returns the current theme stylesheet. The server calls it on every
// request, so a theme edited or switched on disk applies without a restart.
type Source func() ([]byte, error)

// Resolve turns a --theme value into a Source and a name for logs. An empty
// value follows the active Omarchy theme when there is one and falls back to
// the default otherwise; "default" and "omarchy" force either; anything else
// is the path of a CSS file layered over the default.
func Resolve(spec string) (Source, string, error) {
	switch spec {
	case "default":
		return func() ([]byte, error) { return Default, nil }, "default", nil
	case "", "omarchy":
		path, err := omarchyColors()
		if err == nil {
			_, err = os.Stat(path)
		}
		if err != nil {
			if spec == "" {
				return Resolve("default")
			}
			return nil, "", fmt.Errorf("no Omarchy theme found: %w", err)
		}
		src := func() ([]byte, error) {
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			return fromOmarchy(raw)
		}
		return checked(src, "omarchy")
	default:
		src := func() ([]byte, error) {
			css, err := os.ReadFile(spec)
			if err != nil {
				return nil, err
			}
			return append(append(append([]byte{}, Default...), '\n'), css...), nil
		}
		return checked(src, spec)
	}
}

func checked(src Source, name string) (Source, string, error) {
	if _, err := src(); err != nil {
		return nil, "", fmt.Errorf("theme %s: %w", name, err)
	}
	return src, name, nil
}

func omarchyColors() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", errors.New("no home directory")
	}
	return filepath.Join(home, ".local", "state", "omarchy", "current", "theme", "colors.toml"), nil
}
