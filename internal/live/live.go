// Package live is the single source of truth for capture/push targets.
//
//	~/.config/webcast-mate/live.json
//
// Only platforms currently live appear here. Capture reads this file only.
package live

import (
	"os"
	"time"

	"github.com/xifan2333/webcast-mate/internal/appdir"
)

// File is the whole live.json document.
type File struct {
	// Platforms keyed by platform id.
	Platforms map[string]Target `json:"platforms"`
}

// Target is one active RTMP destination.
type Target struct {
	RoomID string `json:"room_id"`
	// StreamID is platform-specific (douyin ping/anchor needs it).
	StreamID     string    `json:"stream_id,omitempty"`
	Server       string    `json:"server"`
	Key          string    `json:"key"`
	VideoBitrate int       `json:"video_bitrate"`
	AudioBitrate int       `json:"audio_bitrate"`
	StartedAt    time.Time `json:"started_at"`
}

func Load() (*File, error) {
	path, err := appdir.LivePath()
	if err != nil {
		return nil, err
	}
	var f File
	if err := appdir.ReadJSON(path, &f); err != nil {
		if os.IsNotExist(err) {
			return &File{Platforms: map[string]Target{}}, nil
		}
		return nil, err
	}
	if f.Platforms == nil {
		f.Platforms = map[string]Target{}
	}
	return &f, nil
}

func Save(f *File) error {
	path, err := appdir.LivePath()
	if err != nil {
		return err
	}
	if f.Platforms == nil {
		f.Platforms = map[string]Target{}
	}
	// empty → remove file so capture sees "nothing live"
	if len(f.Platforms) == 0 {
		return appdir.Remove(path)
	}
	return appdir.WriteJSON(path, f)
}

// Upsert adds or replaces a live target.
func Upsert(platform string, t Target) error {
	f, err := Load()
	if err != nil {
		return err
	}
	f.Platforms[platform] = t
	return Save(f)
}

// Remove drops a platform; no-op if absent.
func Remove(platform string) error {
	f, err := Load()
	if err != nil {
		return err
	}
	delete(f.Platforms, platform)
	return Save(f)
}

// Get returns a target if live.
func Get(platform string) (Target, bool) {
	f, err := Load()
	if err != nil {
		return Target{}, false
	}
	t, ok := f.Platforms[platform]
	return t, ok
}

// Path for messages.
func Path() string {
	p, err := appdir.LivePath()
	if err != nil {
		return "$XDG_CONFIG_HOME/webcast-mate/live.json"
	}
	return p
}
