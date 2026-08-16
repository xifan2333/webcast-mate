package platform

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want ID
		ok   bool
	}{
		{"bilibili", Bilibili, true},
		{"  DOUYIN ", Douyin, true},
		{"XiaoHongShu", XiaoHongShu, true},
		{"xhs", "", false}, // no aliases
		{"dy", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := Parse(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("Parse(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestIDString(t *testing.T) {
	if Bilibili.String() != "bilibili" {
		t.Errorf("Bilibili.String = %q", Bilibili.String())
	}
}

func TestAllOrder(t *testing.T) {
	want := []ID{Bilibili, Douyin, XiaoHongShu}
	if len(All) != len(want) {
		t.Fatalf("All len = %d, want %d", len(All), len(want))
	}
	for i := range want {
		if All[i] != want[i] {
			t.Errorf("All[%d] = %q, want %q", i, All[i], want[i])
		}
	}
}
