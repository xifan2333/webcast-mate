package bilibili

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/xifan2333/webcast-mate/internal/session"
	"gopkg.in/yaml.v3"
)

// Config is XDG bilibili settings.
//
//	~/.config/webcast-mate/bilibili/config.yaml
type Config struct {
	RoomID string `yaml:"room_id"`
	AreaV2 string `yaml:"area_v2"`
	Title  string `yaml:"title,omitempty"`
	// Cover is optional: local image path or https://*.hdslb.com/... URL.
	Cover string `yaml:"cover,omitempty"`
}

func configPath() (string, error) {
	d, err := session.PlatformDir("bilibili")
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.yaml"), nil
}

// LoadConfigFile reads yaml; missing file → empty config (no error).
func LoadConfigFile() (*Config, string, error) {
	path, err := configPath()
	if err != nil {
		return nil, "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{AreaV2: "21"}, path, nil
		}
		return nil, path, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, path, err
	}
	if c.AreaV2 == "" {
		c.AreaV2 = "21"
	}
	return &c, path, nil
}

// SaveConfig writes config.yaml.
func SaveConfig(c *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// ValidateForStart ensures required fields for non-interactive start.
func (c *Config) ValidateForStart() error {
	if c == nil || c.RoomID == "" {
		path, _ := configPath()
		return fmt.Errorf("%w: room_id empty (edit %s or run without -y)", ErrNotConfigured, path)
	}
	if c.AreaV2 == "" {
		return fmt.Errorf("%w: area_v2 empty", ErrNotConfigured)
	}
	return nil
}
