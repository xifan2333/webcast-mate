package bilibili

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const (
	ua = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"

	urlQRGenerate = "https://passport.bilibili.com/x/passport-login/web/qrcode/generate"
	urlQRPoll     = "https://passport.bilibili.com/x/passport-login/web/qrcode/poll"
	urlNav        = "https://api.bilibili.com/x/web-interface/nav"
	urlStartLive  = "https://api.live.bilibili.com/room/v1/Room/startLive"
	urlStopLive   = "https://api.live.bilibili.com/room/v1/Room/stopLive"
	urlUpdateRoom = "https://api.live.bilibili.com/room/v1/Room/update"
	urlFaceAuth   = "https://api.live.bilibili.com/xlive/app-blink/v1/preLive/IsUserIdentifiedByFaceAuth"
	urlRoomInfo   = "https://api.live.bilibili.com/room/v1/Room/get_info"
)

type Client struct {
	http *http.Client
	jar  http.CookieJar
}

func NewClient() (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Client{
		jar: jar,
		http: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
	}, nil
}

func (c *Client) setCookieHeader(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	// apply to main bilibili domains
	hosts := []string{
		"https://passport.bilibili.com",
		"https://api.bilibili.com",
		"https://api.live.bilibili.com",
		"https://bilibili.com",
	}
	for _, h := range hosts {
		u, err := url.Parse(h)
		if err != nil {
			continue
		}
		var cookies []*http.Cookie
		for _, part := range strings.Split(raw, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			k, v, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			cookies = append(cookies, &http.Cookie{
				Name:   strings.TrimSpace(k),
				Value:  strings.TrimSpace(v),
				Path:   "/",
				Domain: ".bilibili.com",
			})
		}
		c.jar.SetCookies(u, cookies)
	}
	return nil
}

func (c *Client) cookieString() string {
	u, _ := url.Parse("https://api.bilibili.com")
	var parts []string
	seen := map[string]bool{}
	for _, ck := range c.jar.Cookies(u) {
		if seen[ck.Name] {
			continue
		}
		seen[ck.Name] = true
		parts = append(parts, ck.Name+"="+ck.Value)
	}
	// also passport jar
	u2, _ := url.Parse("https://passport.bilibili.com")
	for _, ck := range c.jar.Cookies(u2) {
		if seen[ck.Name] {
			continue
		}
		seen[ck.Name] = true
		parts = append(parts, ck.Name+"="+ck.Value)
	}
	return strings.Join(parts, "; ")
}

func (c *Client) csrf() string {
	u, _ := url.Parse("https://api.bilibili.com")
	for _, ck := range c.jar.Cookies(u) {
		if ck.Name == "bili_jct" {
			return ck.Value
		}
	}
	u2, _ := url.Parse("https://passport.bilibili.com")
	for _, ck := range c.jar.Cookies(u2) {
		if ck.Name == "bili_jct" {
			return ck.Value
		}
	}
	// parse from combined
	for _, part := range strings.Split(c.cookieString(), ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.TrimSpace(k) == "bili_jct" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (c *Client) doJSON(method, rawURL string, form url.Values, headers map[string]string) ([]byte, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	}
	req.Header.Set("Origin", "https://link.bilibili.com")
	req.Header.Set("Referer", "https://link.bilibili.com/p/center/index")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return b, fmt.Errorf("http %d", resp.StatusCode)
	}
	return b, nil
}
