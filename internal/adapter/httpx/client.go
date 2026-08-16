// Package httpx provides a shared HTTP client with cookie-jar plumbing used by
// the platform adapters. It holds only the generic logic; each adapter keeps
// its own host list, cookie domain, and preferred key order as thin wrappers.
package httpx

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Client wraps *http.Client with a cookie jar.
type Client struct {
	HTTP *http.Client
	Jar  http.CookieJar
}

// New returns a client with a 30s timeout and a fresh cookie jar.
func New() *Client {
	jar, _ := cookiejar.New(nil) // never fails for nil options
	return &Client{
		HTTP: &http.Client{Timeout: 30 * time.Second, Jar: jar},
		Jar:  jar,
	}
}

// SetCookieHeader applies a "k=v; k2=v2" header to the jar across hosts.
// domain is the optional cookie Domain (e.g. ".bilibili.com"); empty = host-only.
func (c *Client) SetCookieHeader(header, domain string, hosts ...string) {
	if strings.TrimSpace(header) == "" {
		return
	}
	var cks []*http.Cookie
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		k, v, ok := strings.Cut(part, "=")
		if !ok || k == "" {
			continue
		}
		ck := &http.Cookie{Name: strings.TrimSpace(k), Value: strings.TrimSpace(v), Path: "/"}
		if domain != "" {
			ck.Domain = domain
		}
		cks = append(cks, ck)
	}
	for _, h := range hosts {
		if u, err := url.Parse(h); err == nil {
			c.Jar.SetCookies(u, cks)
		}
	}
}

// CookieString serializes jar cookies across hosts. prefer keys come first;
// remaining keys are sorted for determinism.
func (c *Client) CookieString(hosts, prefer []string) string {
	seen := map[string]string{}
	for _, h := range hosts {
		u, err := url.Parse(h)
		if err != nil {
			continue
		}
		for _, ck := range c.Jar.Cookies(u) {
			if ck.Value != "" {
				seen[ck.Name] = ck.Value
			}
		}
	}
	parts := make([]string, 0, len(seen))
	used := map[string]bool{}
	for _, k := range prefer {
		if v, ok := seen[k]; ok {
			parts = append(parts, k+"="+v)
			used[k] = true
		}
	}
	rest := make([]string, 0, len(seen)-len(parts))
	for k, v := range seen {
		if !used[k] {
			rest = append(rest, k+"="+v)
		}
	}
	sort.Strings(rest)
	parts = append(parts, rest...)
	return strings.Join(parts, "; ")
}

// CookieValue returns the first cookie value matching any name across hosts.
func (c *Client) CookieValue(hosts, names []string) string {
	for _, h := range hosts {
		u, err := url.Parse(h)
		if err != nil {
			continue
		}
		for _, ck := range c.Jar.Cookies(u) {
			for _, n := range names {
				if ck.Name == n {
					return ck.Value
				}
			}
		}
	}
	return ""
}
