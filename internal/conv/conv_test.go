package conv

import (
	"encoding/json"
	"testing"
)

func TestAnyString(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"x", "x"},
		{float64(3), "3"},
		{int(42), "42"},
		{int64(42), "42"},
		{json.Number("7"), "7"},
		{nil, ""},
		{true, "true"},
	}
	for _, c := range cases {
		if got := AnyString(c.in); got != c.want {
			t.Errorf("AnyString(%#v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAnyInt(t *testing.T) {
	cases := []struct {
		in   any
		want int
	}{
		{float64(3.9), 3},
		{int(5), 5},
		{int64(7), 7},
		{json.Number("9"), 9},
		{"12", 12},
		{"abc", 0},
		{nil, 0},
	}
	for _, c := range cases {
		if got := AnyInt(c.in); got != c.want {
			t.Errorf("AnyInt(%#v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestAnyBool(t *testing.T) {
	cases := []struct {
		in   any
		want bool
	}{
		{true, true},
		{false, false},
		{float64(1), true},
		{float64(0), false},
		{int(1), true},
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"0", false},
		{"", false},
	}
	for _, c := range cases {
		if got := AnyBool(c.in); got != c.want {
			t.Errorf("AnyBool(%#v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		s    string
		n    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 3, "hel…"},
		{"你好世界", 2, "你好…"}, // rune-safe: must not split a rune
		{"", 3, ""},
	}
	for _, c := range cases {
		if got := Truncate(c.s, c.n); got != c.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", c.s, c.n, got, c.want)
		}
	}
}

func TestFirstString(t *testing.T) {
	m := map[string]any{"a": "x", "b": float64(9)}
	if got := FirstString(m, "b", "a"); got != "9" {
		t.Errorf("FirstString = %q, want %q", got, "9")
	}
	if got := FirstString(m, "a"); got != "x" {
		t.Errorf("FirstString = %q, want %q", got, "x")
	}
	if got := FirstString(m, "missing"); got != "" {
		t.Errorf("FirstString missing = %q, want empty", got)
	}
}

func TestFirstArray(t *testing.T) {
	arr := []any{1, 2}
	m := map[string]any{"a": arr}
	if got := FirstArray(m, "a"); len(got) != 2 {
		t.Errorf("FirstArray = %v, want len 2", got)
	}
	if got := FirstArray(m, "missing"); got != nil {
		t.Errorf("FirstArray missing = %v, want nil", got)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := FirstNonEmpty("", "  ", "x"); got != "x" {
		t.Errorf("FirstNonEmpty = %q, want x", got)
	}
	if got := FirstNonEmpty("", "  "); got != "" {
		t.Errorf("FirstNonEmpty = %q, want empty", got)
	}
}
