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
