package douyin

import "testing"

func TestSplitPushURL(t *testing.T) {
	s, k := splitPushURL("rtmp://push-rtmp-l1.douyincdn.com/third/stream-123?sign=abc&expire=1")
	if s != "rtmp://push-rtmp-l1.douyincdn.com/third" {
		t.Fatalf("server %q", s)
	}
	if k != "stream-123?sign=abc&expire=1" {
		t.Fatalf("key %q", k)
	}
}
