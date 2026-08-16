// Package secrets stores login cookies separately from config.yaml.
//
//	~/.config/webcast-mate/secrets/<platform>.json  (0600)
package secrets

import (
	"os"
	"time"

	"github.com/xifan2333/webcast-mate/internal/appdir"
)

// File is per-platform secret material.
type File struct {
	Cookie   string    `json:"cookie"`
	UserID   string    `json:"user_id,omitempty"`
	UserName string    `json:"user_name,omitempty"`
	LoginAt  time.Time `json:"login_at,omitempty"`
}

func Load(platform string) (*File, error) {
	path, err := appdir.SecretPath(platform)
	if err != nil {
		return nil, err
	}
	var f File
	if err := appdir.ReadJSON(path, &f); err != nil {
		return nil, err
	}
	if f.Cookie == "" {
		return nil, os.ErrNotExist
	}
	return &f, nil
}

func Save(platform string, f *File) error {
	path, err := appdir.SecretPath(platform)
	if err != nil {
		return err
	}
	return appdir.WriteJSON(path, f)
}

func Clear(platform string) error {
	path, err := appdir.SecretPath(platform)
	if err != nil {
		return err
	}
	return appdir.Remove(path)
}
