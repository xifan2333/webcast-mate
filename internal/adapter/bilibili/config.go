package bilibili

import (
	"fmt"

	"github.com/xifan2333/webcast-mate/internal/appcfg"
)

// OpenConfig is start-time prefs for bilibili (same name as other adapters).
type OpenConfig struct {
	RoomID string
	Area   string
	Title  string
}

func fromPlatform(p appcfg.Platform) *OpenConfig {
	c := &OpenConfig{
		RoomID: p.RoomID,
		Area:   p.Area,
		Title:  p.Title,
	}
	if c.Area == "" {
		c.Area = "21"
	}
	return c
}

func (c *OpenConfig) toPlatform(prev appcfg.Platform) appcfg.Platform {
	p := prev
	p.RoomID = c.RoomID
	p.Area = c.Area
	p.Title = c.Title
	return p
}

func (c *OpenConfig) ValidateForStart() error {
	if c == nil || c.RoomID == "" {
		return fmt.Errorf("%w: room_id empty (%s)", ErrNotConfigured, appcfg.Path())
	}
	if c.Area == "" {
		return fmt.Errorf("%w: area empty", ErrNotConfigured)
	}
	return nil
}
