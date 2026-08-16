package httpx

import "testing"

func TestSetAndReadCookie(t *testing.T) {
	c := New()
	c.SetCookieHeader("a=1; b=2", ".example.com", "https://api.example.com", "https://www.example.com")
	got := c.CookieString([]string{"https://api.example.com"}, nil)
	if got != "a=1; b=2" {
		t.Fatalf("CookieString = %q, want %q", got, "a=1; b=2")
	}
}

func TestCookiePreferOrder(t *testing.T) {
	c := New()
	c.SetCookieHeader("z=26; sessionid=abc; a=1", "", "https://x.example.com")
	got := c.CookieString([]string{"https://x.example.com"}, []string{"sessionid"})
	if got != "sessionid=abc; a=1; z=26" {
		t.Fatalf("CookieString = %q, want %q", got, "sessionid=abc; a=1; z=26")
	}
}

func TestCookieValue(t *testing.T) {
	c := New()
	c.SetCookieHeader("csrf=TOKEN", "", "https://api.example.com")
	if got := c.CookieValue([]string{"https://api.example.com"}, []string{"csrf"}); got != "TOKEN" {
		t.Fatalf("CookieValue = %q, want TOKEN", got)
	}
	if got := c.CookieValue([]string{"https://api.example.com"}, []string{"missing"}); got != "" {
		t.Fatalf("CookieValue missing = %q, want empty", got)
	}
}

func TestSetCookieHeaderEmpty(t *testing.T) {
	c := New()
	c.SetCookieHeader("", ".example.com", "https://api.example.com")
	if got := c.CookieString([]string{"https://api.example.com"}, nil); got != "" {
		t.Fatalf("empty header should leave empty jar, got %q", got)
	}
}
