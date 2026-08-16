package appcfg

import (
	"testing"

	"github.com/xifan2333/webcast-mate/internal/platform"
)

func TestBitrateMerge(t *testing.T) {
	f := defaultFile()

	// platform default from defaultFile
	if v, a := f.Bitrate(platform.Douyin); v != 4000 || a != 128 {
		t.Errorf("douyin default = %d/%d, want 4000/128", v, a)
	}
	// explicit platform override
	f.Platforms[string(platform.Douyin)] = Platform{VideoBitrate: 6000, AudioBitrate: 192}
	if v, a := f.Bitrate(platform.Douyin); v != 6000 || a != 192 {
		t.Errorf("douyin override = %d/%d, want 6000/192", v, a)
	}
	// empty platform block falls back to global defaults (3200/128)
	f.Platforms[string(platform.XiaoHongShu)] = Platform{}
	if v, a := f.Bitrate(platform.XiaoHongShu); v != 3200 || a != 128 {
		t.Errorf("xhs fallback = %d/%d, want 3200/128", v, a)
	}
	// hardcoded floor when defaults and platform are both zero
	f.Defaults = Defaults{}
	if v, a := f.Bitrate(platform.XiaoHongShu); v != 3200 || a != 128 {
		t.Errorf("xhs floor = %d/%d, want 3200/128", v, a)
	}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	f, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f == nil || f.Defaults.VideoBitrate != 3200 || f.Defaults.AudioBitrate != 128 {
		t.Fatalf("defaults = %+v", f)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	f := defaultFile()
	if err := f.SetPlatform(platform.Bilibili, Platform{RoomID: "42", Title: "hello"}); err != nil {
		t.Fatalf("SetPlatform: %v", err)
	}
	g, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p := g.GetPlatform(platform.Bilibili)
	if p.RoomID != "42" || p.Title != "hello" {
		t.Errorf("round-trip platform = %+v", p)
	}
}
