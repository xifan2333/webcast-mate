package appcfg

import (
	"os"

	"github.com/xifan2333/webcast-mate/internal/appdir"
	"github.com/xifan2333/webcast-mate/internal/platform"
	"gopkg.in/yaml.v3"
)

// File is the single user-facing config (no cookies, no bitrate — bitrate is
// owned by the streaming layer).
type File struct {
	Platforms map[string]Platform `yaml:"platforms"`
}

// Platform holds durable open preferences for one platform.
type Platform struct {
	RoomID string `yaml:"room_id,omitempty"`
	Area   string `yaml:"area,omitempty"` // partition: bili leaf id / dy base|leaf / xhs leaf id
	Title  string `yaml:"title,omitempty"`
}

func defaultFile() *File {
	return &File{
		Platforms: map[string]Platform{
			"bilibili": {Area: "21"},
		},
	}
}

// Load reads config.yaml; missing → defaults (not an error).
func Load() (*File, error) {
	path, err := appdir.ConfigPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultFile(), nil
		}
		return nil, err
	}
	f := defaultFile()
	if err := yaml.Unmarshal(b, f); err != nil {
		return nil, err
	}
	if f.Platforms == nil {
		f.Platforms = map[string]Platform{}
	}
	return f, nil
}

// Save writes config.yaml (0600).
func Save(f *File) error {
	path, err := appdir.ConfigPath()
	if err != nil {
		return err
	}
	b, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// GetPlatform returns platform block or empty.
func (f *File) GetPlatform(id platform.ID) Platform {
	if f == nil || f.Platforms == nil {
		return Platform{}
	}
	return f.Platforms[string(id)]
}

// SetPlatform upserts and saves.
func (f *File) SetPlatform(id platform.ID, p Platform) error {
	if f.Platforms == nil {
		f.Platforms = map[string]Platform{}
	}
	f.Platforms[string(id)] = p
	return Save(f)
}

// Path returns config.yaml path for messages.
func Path() string {
	p, err := appdir.ConfigPath()
	if err != nil {
		return "$XDG_CONFIG_HOME/webcastmate/config.yaml"
	}
	return p
}

// EnsureExists writes default file if missing (so users can edit).
func EnsureExists() error {
	path, err := appdir.ConfigPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return Save(defaultFile())
}
