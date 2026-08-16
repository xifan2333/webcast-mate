package douyin

import "testing"

func TestSplitPushURL(t *testing.T) {
	cases := []struct {
		in          string
		server, key string
	}{
		{"rtmp://host/app/key", "rtmp://host/app", "key"},
		{"rtmp://host/app", "rtmp://host/app", ""},
		{"rtmp://host", "rtmp://host", ""},
		{"rtmps://host/app/key", "rtmps://host/app", "key"},
		{"http://host/app/key", "http://host/app/key", ""},
		{"", "", ""},
		{"  rtmp://host/app/key  ", "rtmp://host/app", "key"},
	}
	for _, c := range cases {
		s, k := splitPushURL(c.in)
		if s != c.server || k != c.key {
			t.Errorf("splitPushURL(%q) = (%q, %q), want (%q, %q)", c.in, s, k, c.server, c.key)
		}
	}
}
