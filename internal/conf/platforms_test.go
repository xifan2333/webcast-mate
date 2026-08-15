package conf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpsertPreservesBitrate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "platforms.conf")
	raw := "[bilibili]\nserver = old\nkey = oldk\nvideo_bitrate = 3200\naudio_bitrate = 128\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	f.UpsertServerKey("bilibili", "rtmp://new/", "newkey", "4000", "128")
	if f.Sections["bilibili"].VideoBitrate != "3200" {
		t.Fatalf("bitrate overwritten: %q", f.Sections["bilibili"].VideoBitrate)
	}
	if err := f.Write(path); err != nil {
		t.Fatal(err)
	}
	f2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s := f2.Sections["bilibili"]
	if s.Server != "rtmp://new/" || s.Key != "newkey" || s.VideoBitrate != "3200" {
		t.Fatalf("%+v", s)
	}
}

func TestUpsertCreatesSection(t *testing.T) {
	f := &File{Sections: map[string]*Section{}}
	f.UpsertServerKey("douyin", "rtmp://d/", "k", "4000", "128")
	if f.Sections["douyin"].VideoBitrate != "4000" {
		t.Fatal(f.Sections["douyin"])
	}
}
