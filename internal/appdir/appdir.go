// Package appdir owns the single XDG layout for webcast-mate.
//
//	~/.config/webcast-mate/
//	  config.yaml           # preferences (no secrets)
//	  secrets/<platform>.json  # cookie + login meta (0600)
//	  live.json             # active push targets for capture (0600)
package appdir

import (
	"fmt"
	"os"
	"path/filepath"
)

// Root is ~/.config/webcast-mate (Linux-only; XDG).
func Root() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "webcast-mate"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "webcast-mate"), nil
}

func EnsureRoot() (string, error) {
	d, err := Root()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", fmt.Errorf("appdir: %w", err)
	}
	return d, nil
}

func ConfigPath() (string, error) {
	r, err := EnsureRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(r, "config.yaml"), nil
}

func SecretsDir() (string, error) {
	r, err := EnsureRoot()
	if err != nil {
		return "", err
	}
	d := filepath.Join(r, "secrets")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	return d, nil
}

func SecretPath(platform string) (string, error) {
	d, err := SecretsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, platform+".json"), nil
}

func LivePath() (string, error) {
	r, err := EnsureRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(r, "live.json"), nil
}

// RunDir holds runtime pid/state (not secrets).
func RunDir() (string, error) {
	r, err := EnsureRoot()
	if err != nil {
		return "", err
	}
	d := filepath.Join(r, "run")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", fmt.Errorf("appdir run: %w", err)
	}
	return d, nil
}
