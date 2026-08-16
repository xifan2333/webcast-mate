package secrets

import (
	"testing"
)

func TestNormalize(t *testing.T) {
	f := &File{
		Cookies: map[string]string{" a ": " 1 ", "": "x", "k": ""},
		Headers: map[string]string{"Cookie": "v", "X-Real": " y "},
		Params:  nil,
	}
	f.Normalize()
	if f.Cookies["a"] != "1" {
		t.Errorf("cookies a = %q, want 1", f.Cookies["a"])
	}
	if _, ok := f.Cookies[""]; ok {
		t.Errorf("empty cookie key not dropped")
	}
	if _, ok := f.Cookies["k"]; ok {
		t.Errorf("empty cookie value not dropped")
	}
	if _, ok := f.Headers["Cookie"]; ok {
		t.Errorf("Cookie header not dropped")
	}
	if f.Headers["X-Real"] != "y" {
		t.Errorf("headers X-Real = %q, want y", f.Headers["X-Real"])
	}
	if f.Params == nil {
		t.Errorf("params should be non-nil after Normalize")
	}
	if f.Version != Version {
		t.Errorf("version = %d, want %d", f.Version, Version)
	}
}

func TestCookieHeaderSorted(t *testing.T) {
	f := &File{Cookies: map[string]string{"b": "2", "a": "1"}}
	if got := f.CookieHeader(); got != "a=1; b=2" {
		t.Errorf("CookieHeader = %q, want %q", got, "a=1; b=2")
	}
	if f := (*File)(nil); f.CookieHeader() != "" {
		t.Errorf("nil CookieHeader should be empty")
	}
}

func TestParseCookieHeader(t *testing.T) {
	got := ParseCookieHeader("a=1; b=2 ; c; =d")
	if got["a"] != "1" || got["b"] != "2" {
		t.Errorf("ParseCookieHeader = %#v", got)
	}
	if _, ok := got["c"]; ok {
		t.Errorf("malformed c should be dropped")
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestSetCookieHeaderMerge(t *testing.T) {
	f := &File{Cookies: map[string]string{"a": "1"}}
	f.SetCookieHeader("b=2; a=3")
	if f.Cookies["a"] != "3" || f.Cookies["b"] != "2" {
		t.Errorf("SetCookieHeader merge = %#v", f.Cookies)
	}
}

func TestAuthMapsCopies(t *testing.T) {
	f := &File{Cookies: map[string]string{"a": "1"}, Headers: map[string]string{"x": "y"}, Params: map[string]string{"p": "q"}}
	c, h, p := f.AuthMaps()
	c["a"] = "MUTATED"
	h["x"] = "MUTATED"
	p["p"] = "MUTATED"
	if f.Cookies["a"] != "1" || f.Headers["x"] != "y" || f.Params["p"] != "q" {
		t.Errorf("AuthMaps returned aliased maps")
	}
}

func TestHasAuth(t *testing.T) {
	if (&File{}).HasAuth() {
		t.Errorf("empty file should have no auth")
	}
	if (&File{Cookies: map[string]string{"a": "1"}}).HasAuth() != true {
		t.Errorf("file with cookie should have auth")
	}
}
