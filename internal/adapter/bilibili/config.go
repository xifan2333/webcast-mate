package bilibili

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/xifan2333/webcast-mate/internal/session"
	"gopkg.in/yaml.v3"
)

// Config is XDG bilibili settings (not CLI flags).
//
//	~/.config/webcast-mate/bilibili/config.yaml
type Config struct {
	// RoomID is the real live room id (not short id).
	RoomID string `yaml:"room_id"`
	// AreaV2 is the secondary partition id for startLive.
	AreaV2 string `yaml:"area_v2"`
	// Title optional; if set, update room title before start.
	Title string `yaml:"title,omitempty"`
}

func configPath() (string, error) {
	d, err := session.PlatformDir("bilibili")
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.yaml"), nil
}

// LoadConfig reads config or returns defaults + ErrNotConfigured if room missing.
func LoadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{AreaV2: "21"}, fmt.Errorf("%w: create %s (need room_id)", ErrNotConfigured, path)
		}
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.AreaV2 == "" {
		c.AreaV2 = "21" // 视频唱见 common default; user should set
	}
	if c.RoomID == "" {
		return &c, fmt.Errorf("%w: room_id empty in %s", ErrNotConfigured, path)
	}
	return &c, nil
}

// WriteExampleConfig writes a template if missing.
func WriteExampleConfig() (string, error) {
	path, err := configPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	ex := []byte(`# bilibili open settings for webcast-mate
# real room id (link center), not the short id
room_id: ""
# secondary area id (area_v2) for startLive
area_v2: "21"
# optional title applied before start
# title: "my stream"
`)
	if err := os.WriteFile(path, ex, 0o600); err != nil {
		return "", err
	}
	return path, nil
}
