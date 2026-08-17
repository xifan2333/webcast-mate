// Package secrets stores request auth material separately from config.yaml.
//
//	~/.config/webcastmate/secrets/<platform>.json  (0600)
//
// Layout mirrors requests.Session (Python):
//
//	cookies  → Cookie jar / Cookie header
//	headers  → fixed request headers (never a "Cookie" key)
//	params   → default query string (requests params=)
package secrets

import (
	"os"
	"sort"
	"strings"
	"time"

	"github.com/xifan2333/webcast-mate/internal/appdir"
	"github.com/xifan2333/webcast-mate/internal/platform"
)

const Version = 2

// File is per-platform secret material (auth only).
type File struct {
	Version  int               `json:"version"`
	UserID   string            `json:"user_id,omitempty"`
	UserName string            `json:"user_name,omitempty"`
	LoginAt  time.Time         `json:"login_at,omitempty"`
	Cookies  map[string]string `json:"cookies,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Params   map[string]string `json:"params,omitempty"`
}

// Normalize ensures maps exist and drops empty keys.
func (f *File) Normalize() {
	if f == nil {
		return
	}
	if f.Cookies == nil {
		f.Cookies = map[string]string{}
	}
	if f.Headers == nil {
		f.Headers = map[string]string{}
	}
	if f.Params == nil {
		f.Params = map[string]string{}
	}
	scrubMap(f.Cookies)
	scrubMap(f.Headers)
	scrubMap(f.Params)
	delete(f.Headers, "Cookie")
	delete(f.Headers, "cookie")
	if f.Version == 0 {
		f.Version = Version
	}
}

func scrubMap(m map[string]string) {
	for k, v := range m {
		k2 := strings.TrimSpace(k)
		v2 := strings.TrimSpace(v)
		if k2 == "" || v2 == "" {
			delete(m, k)
			continue
		}
		if k2 != k || v2 != v {
			delete(m, k)
			m[k2] = v2
		}
	}
}

// HasAuth reports whether any auth material is present.
func (f *File) HasAuth() bool {
	if f == nil {
		return false
	}
	f.Normalize()
	return len(f.Cookies) > 0 || len(f.Headers) > 0 || len(f.Params) > 0
}

// CookieHeader builds requests-style Cookie header from cookies map.
func (f *File) CookieHeader() string {
	if f == nil {
		return ""
	}
	f.Normalize()
	if len(f.Cookies) == 0 {
		return ""
	}
	keys := make([]string, 0, len(f.Cookies))
	for k := range f.Cookies {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+f.Cookies[k])
	}
	return strings.Join(parts, "; ")
}

// SetCookieHeader merges a Cookie header string into cookies map.
// Used when capturing jar output after HTTP round-trips.
func (f *File) SetCookieHeader(header string) {
	if f.Cookies == nil {
		f.Cookies = map[string]string{}
	}
	for k, v := range ParseCookieHeader(header) {
		f.Cookies[k] = v
	}
}

// ParseCookieHeader splits "k=v; k2=v2".
func ParseCookieHeader(header string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}

// Load reads v2 secrets only.
func Load(id platform.ID) (*File, error) {
	path, err := appdir.SecretPath(string(id))
	if err != nil {
		return nil, err
	}
	var f File
	if err := appdir.ReadJSON(path, &f); err != nil {
		return nil, err
	}
	f.Normalize()
	if f.Version != Version {
		return nil, os.ErrInvalid
	}
	if !f.HasAuth() {
		return nil, os.ErrNotExist
	}
	return &f, nil
}

// Save writes v2 buckets only.
func Save(id platform.ID, f *File) error {
	if f == nil {
		return os.ErrInvalid
	}
	f.Normalize()
	f.Version = Version
	path, err := appdir.SecretPath(string(id))
	if err != nil {
		return err
	}
	out := struct {
		Version  int               `json:"version"`
		UserID   string            `json:"user_id,omitempty"`
		UserName string            `json:"user_name,omitempty"`
		LoginAt  time.Time         `json:"login_at,omitempty"`
		Cookies  map[string]string `json:"cookies,omitempty"`
		Headers  map[string]string `json:"headers,omitempty"`
		Params   map[string]string `json:"params,omitempty"`
	}{
		Version:  f.Version,
		UserID:   f.UserID,
		UserName: f.UserName,
		LoginAt:  f.LoginAt,
		Cookies:  nonEmptyMap(f.Cookies),
		Headers:  nonEmptyMap(f.Headers),
		Params:   nonEmptyMap(f.Params),
	}
	return appdir.WriteJSON(path, out)
}

func nonEmptyMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	return m
}

// AuthMaps returns shallow copies of the three buckets for stdout JSON.
func (f *File) AuthMaps() (cookies, headers, params map[string]string) {
	if f == nil {
		return nil, nil, nil
	}
	f.Normalize()
	return copyMap(f.Cookies), copyMap(f.Headers), copyMap(f.Params)
}

func copyMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func Clear(id platform.ID) error {
	path, err := appdir.SecretPath(string(id))
	if err != nil {
		return err
	}
	return appdir.Remove(path)
}
