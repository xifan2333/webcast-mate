package session

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Dir returns ~/.config/webcast-mate (XDG).
func Dir() (string, error) {
	if runtime.GOOS == "windows" {
		if app := os.Getenv("AppData"); app != "" {
			return filepath.Join(app, "webcast-mate"), nil
		}
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "webcast-mate"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "webcast-mate"), nil
}

// PlatformDir is <Dir>/<platform>/.
func PlatformDir(platform string) (string, error) {
	root, err := Dir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(root, platform)
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", fmt.Errorf("session dir: %w", err)
	}
	return d, nil
}

// EnsureRoot creates the config root.
func EnsureRoot() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	return d, nil
}
