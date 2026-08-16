package bilibili

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/secrets"
)

const (
	ua = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"

	urlQRGenerate   = "https://passport.bilibili.com/x/passport-login/web/qrcode/generate"
	urlQRPoll       = "https://passport.bilibili.com/x/passport-login/web/qrcode/poll"
	urlNav          = "https://api.bilibili.com/x/web-interface/nav"
	urlStartLive    = "https://api.live.bilibili.com/room/v1/Room/startLive"
	urlStopLive     = "https://api.live.bilibili.com/room/v1/Room/stopLive"
	urlUpdateRoom   = "https://api.live.bilibili.com/room/v1/Room/update"
	urlFaceAuth     = "https://api.live.bilibili.com/xlive/app-blink/v1/preLive/IsUserIdentifiedByFaceAuth"
	urlRoomInfo     = "https://api.live.bilibili.com/room/v1/Room/get_info"
	urlBlinkGetInfo = "https://api.live.bilibili.com/xlive/app-blink/v1/room/GetInfo?platform=pc"
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

func (c *Client) ApplySecrets(f *secrets.File) error {
	if f == nil {
		return nil
	}
	f.Normalize()
	return c.setCookieHeader(f.CookieHeader())
}

func (c *Client) ExportSecrets(userID, userName string, loginAt time.Time) *secrets.File {
	f := &secrets.File{Version: secrets.Version, UserID: userID, UserName: userName, LoginAt: loginAt,
		Cookies: secrets.ParseCookieHeader(c.cookieString()), Headers: map[string]string{}, Params: map[string]string{}}
	f.Normalize()
	return f
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
		return nil, fmt.Errorf("%w: %v", adapter.ErrNetwork, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return b, fmt.Errorf("%w: http %d", adapter.ErrNetwork, resp.StatusCode)
	}
	return b, nil
}
