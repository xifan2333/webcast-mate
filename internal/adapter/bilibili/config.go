package bilibili

import (
	"fmt"

	"github.com/xifan2333/webcast-mate/internal/appcfg"
)

// Config is the in-memory open prefs (from appcfg platforms.bilibili).
type Config struct {
	RoomID string
	AreaV2 string
	Title  string
	Cover  string
}

func fromPlatform(p appcfg.Platform) *Config {
	c := &Config{
		RoomID: p.RoomID,
		AreaV2: p.AreaV2,
		Title:  p.Title,
		Cover:  p.Cover,
	}
	if c.AreaV2 == "" {
		c.AreaV2 = "21"
	}
	return c
}

func (c *Config) toPlatform(prev appcfg.Platform) appcfg.Platform {
	p := prev
	p.RoomID = c.RoomID
	p.AreaV2 = c.AreaV2
	p.Title = c.Title
	p.Cover = c.Cover
	return p
}

func (c *Config) ValidateForStart() error {
	if c == nil || c.RoomID == "" {
		return fmt.Errorf("%w: room_id empty (%s)", ErrNotConfigured, appcfg.Path())
	}
	if c.AreaV2 == "" {
		return fmt.Errorf("%w: area_v2 empty", ErrNotConfigured)
	}
	return nil
}
