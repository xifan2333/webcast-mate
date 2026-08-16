package appcfg

import (
	"fmt"
	"os"

	"github.com/xifan2333/webcast-mate/internal/appdir"
	"gopkg.in/yaml.v3"
)

// File is the single user-facing config (no cookies).
type File struct {
	Defaults  Defaults            `yaml:"defaults"`
	Platforms map[string]Platform `yaml:"platforms"`
}

type Defaults struct {
	VideoBitrate int `yaml:"video_bitrate"`
	AudioBitrate int `yaml:"audio_bitrate"`
}

// Platform holds durable open preferences for one platform.
type Platform struct {
	RoomID       string `yaml:"room_id,omitempty"`
	AreaV2       string `yaml:"area_v2,omitempty"` // bilibili partition id (file only)
	Title        string `yaml:"title,omitempty"`
	Cover        string `yaml:"cover,omitempty"`
	VideoBitrate int    `yaml:"video_bitrate,omitempty"`
	AudioBitrate int    `yaml:"audio_bitrate,omitempty"`
}

func defaultFile() *File {
	return &File{
		Defaults: Defaults{VideoBitrate: 3200, AudioBitrate: 128},
		Platforms: map[string]Platform{
			"bilibili":    {AreaV2: "21", VideoBitrate: 3200, AudioBitrate: 128},
			"douyin":      {VideoBitrate: 4000, AudioBitrate: 128},
			"xiaohongshu": {VideoBitrate: 4000, AudioBitrate: 128},
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
	if f.Defaults.VideoBitrate == 0 {
		f.Defaults.VideoBitrate = 3200
	}
	if f.Defaults.AudioBitrate == 0 {
		f.Defaults.AudioBitrate = 128
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
func (f *File) GetPlatform(id string) Platform {
	if f == nil || f.Platforms == nil {
		return Platform{}
	}
	return f.Platforms[id]
}

// SetPlatform upserts and saves.
func (f *File) SetPlatform(id string, p Platform) error {
	if f.Platforms == nil {
		f.Platforms = map[string]Platform{}
	}
	f.Platforms[id] = p
	return Save(f)
}

// Bitrate returns effective bitrates for platform.
func (f *File) Bitrate(id string) (video, audio int) {
	video, audio = f.Defaults.VideoBitrate, f.Defaults.AudioBitrate
	p := f.GetPlatform(id)
	if p.VideoBitrate > 0 {
		video = p.VideoBitrate
	}
	if p.AudioBitrate > 0 {
		audio = p.AudioBitrate
	}
	if video == 0 {
		video = 3200
	}
	if audio == 0 {
		audio = 128
	}
	return video, audio
}

// Path returns config.yaml path for messages.
func Path() string {
	p, err := appdir.ConfigPath()
	if err != nil {
		return "$XDG_CONFIG_HOME/webcast-mate/config.yaml"
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

func ValidateBilibili(p Platform) error {
	if p.RoomID == "" {
		return fmt.Errorf("room_id empty in %s (platforms.bilibili)", Path())
	}
	if p.AreaV2 == "" {
		return fmt.Errorf("area_v2 empty in %s", Path())
	}
	return nil
}
