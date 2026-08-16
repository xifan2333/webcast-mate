package live

import (
	"testing"
	"time"

	"github.com/xifan2333/webcast-mate/internal/platform"
)

func TestUpsertGetRemoveRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	id := platform.Douyin
	tg := Target{
		RoomID: "123", Server: "rtmp://s/app", Key: "k",
		VideoBitrate: 4000, AudioBitrate: 128, StartedAt: time.Now().UTC(),
	}
	if err := Upsert(id, tg); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, ok := Get(id)
	if !ok {
		t.Fatalf("Get should find target")
	}
	if got.RoomID != "123" || got.Server != "rtmp://s/app" || got.Key != "k" {
		t.Fatalf("Get = %+v", got)
	}
	if err := Remove(id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := Get(id); ok {
		t.Fatalf("Get after Remove should be false")
	}
}

func TestRemoveAbsentIsNoop(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := Remove(platform.XiaoHongShu); err != nil {
		t.Fatalf("Remove absent: %v", err)
	}
}
