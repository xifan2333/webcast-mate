package xiaohongshu

import "testing"

func TestSplitPushURL(t *testing.T) {
	s, k := SplitPushURL("rtmp://live-push.xhscdn.com/live/abc?auth=1&x=2")
	if s != "rtmp://live-push.xhscdn.com/live" {
		t.Fatalf("server %q", s)
	}
	if k != "abc?auth=1&x=2" {
		t.Fatalf("key %q", k)
	}
}

func TestExtractRoomPush(t *testing.T) {
	m := map[string]any{
		"code":    0.0,
		"success": true,
		"data": map[string]any{
			"live_info": map[string]any{
				"room_info": map[string]any{"room_id": "12345"},
				"stream_info": map[string]any{
					"push_info": map[string]any{
						"push_dispatch_config": `{"encode":{"push_url":"rtmp://live-push.xhscdn.com/live/12345?txSecret=ab"}}`,
					},
				},
			},
		},
	}
	room, push := extractRoomPush(m)
	if room != "12345" {
		t.Fatalf("room %q", room)
	}
	if push == "" || push[:4] != "rtmp" {
		t.Fatalf("push %q", push)
	}
}
