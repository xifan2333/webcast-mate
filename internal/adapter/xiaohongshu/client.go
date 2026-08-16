package xiaohongshu

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/conv"
	"github.com/xifan2333/webcast-mate/internal/secrets"
)

const (
	hostCustomer = "https://customer.xiaohongshu.com"
	hostRobs     = "https://robs.xiaohongshu.com"
	hostRedobs   = "https://redobs.xiaohongshu.com"
	serviceRobs  = "https://robs.xiaohongshu.com"
	xsecAppLive  = "live-helper"
	userAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) live-helper/4.4.0 Chrome/118.0.5993.159 Electron/27.3.2 Safari/537.36 env/production platform/win32 appname/xhs-live win_version/windows"
	xyCommon     = "platform=pc&build=4040000&version=4.4.0&isWin7=false&systemVersion=10.0.19045&cpuModel=Intel(R)+Core(TM)+i5-2520M+CPU+@+2.50GHz&gpu=ANGLE+(Intel,+Intel(R)+HD+Graphics+3000+Direct3D9Ex+vs_3_0+ps_3_0,+igdumd64.dll)"
)

// Client is live-helper 4.4.0 HTTP client (CAS + robs + redobs).
//
// Auth material lives in secrets.File.Cookie as a cookie-like string
// (same schema as bilibili/douyin). Format:
//
//	access-token=AT-…; device-id=…; a1=…; webId=…[; acw_tc=…]
//
// stdout JSON "cookie" stays empty — xhs danmaku uses browser cookies, not helper AT.
type Client struct {
	http *http.Client

	AccessToken string
	DeviceID    string
	A1          string
	WebID       string
	Subsystem   string
	UserID      string
	UserName    string
	extra       map[string]string // sticky browser cookies from Set-Cookie

	// last pre
	RoomID  string
	PushURL string
}

func NewClient() *Client {
	return &Client{
		http:      &http.Client{Timeout: 30 * time.Second},
		Subsystem: "robs",
		DeviceID:  randomMAC(),
		extra:     map[string]string{},
	}
}

func randomMAC() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	b[0] |= 0x02
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", b[0], b[1], b[2], b[3], b[4], b[5])
}

func (c *Client) ensureIdentity() {
	if c.A1 == "" {
		c.A1 = GenerateA1()
	}
	if c.WebID == "" {
		c.WebID = GenerateWebID(c.A1)
	}
	if c.DeviceID == "" {
		c.DeviceID = randomMAC()
	}
	if c.Subsystem == "" {
		c.Subsystem = "robs"
	}
	if c.extra == nil {
		c.extra = map[string]string{}
	}
}

// LoadSecrets applies secrets.File into the client (unified schema).
func (c *Client) LoadSecrets(f *secrets.File) {
	if f == nil {
		return
	}
	f.Normalize()
	c.UserID = f.UserID
	c.UserName = f.UserName
	if v := f.Headers["access-token"]; v != "" {
		c.AccessToken = v
	}
	if v := f.Params["device-id"]; v != "" {
		c.DeviceID = v
	}
	if v := f.Cookies["a1"]; v != "" {
		c.A1 = v
	}
	if v := f.Cookies["webId"]; v != "" {
		c.WebID = v
	}
	if c.extra == nil {
		c.extra = map[string]string{}
	}
	for k, v := range f.Cookies {
		if k == "a1" || k == "webId" {
			continue
		}
		c.extra[k] = v
	}
}

// SecretsFile builds the unified secrets.File for this client.
func (c *Client) SecretsFile() *secrets.File {
	c.ensureIdentity()
	f := &secrets.File{
		Version:  secrets.Version,
		UserID:   c.UserID,
		UserName: c.UserName,
		LoginAt:  time.Now().UTC(),
		Cookies:  map[string]string{},
		Headers:  map[string]string{},
		Params:   map[string]string{},
	}
	if c.AccessToken != "" {
		f.Headers["access-token"] = c.AccessToken
	}
	if c.DeviceID != "" {
		f.Params["device-id"] = c.DeviceID
	}
	if c.A1 != "" {
		f.Cookies["a1"] = c.A1
	}
	if c.WebID != "" {
		f.Cookies["webId"] = c.WebID
	}
	for k, v := range c.extra {
		if v == "" {
			continue
		}
		switch k {
		case "access-token", "auth", "device-id", "a1", "webId", "xsecappid":
			continue
		default:
			f.Cookies[k] = v
		}
	}
	f.Normalize()
	return f
}

// cookieString is what we persist in secrets.File.Cookie (open-live auth only).
func (c *Client) cookieString() string {
	c.ensureIdentity()
	parts := make([]string, 0, 10)
	if c.AccessToken != "" {
		parts = append(parts, "access-token="+c.AccessToken)
	}
	if c.DeviceID != "" {
		parts = append(parts, "device-id="+c.DeviceID)
	}
	parts = append(parts, "a1="+c.A1, "webId="+c.WebID)
	for _, k := range []string{"acw_tc", "websectiga", "sec_poison_id", "gid", "web_session"} {
		if v := c.extra[k]; v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	for k, v := range c.extra {
		switch k {
		case "acw_tc", "websectiga", "sec_poison_id", "gid", "web_session",
			"access-token", "auth", "device-id", "a1", "webId", "xsecappid":
			continue
		}
		if v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	return strings.Join(parts, "; ")
}

func (c *Client) applyCookieString(header string) {
	if header == "" {
		return
	}
	// legacy: JSON blob once shipped in Cookie field
	if header[0] == '{' {
		var m map[string]any
		if json.Unmarshal([]byte(header), &m) == nil {
			c.AccessToken = conv.AnyString(m["access_token"])
			if c.AccessToken == "" {
				c.AccessToken = conv.AnyString(m["access-token"])
			}
			if d := conv.AnyString(m["device_id"]); d != "" {
				c.DeviceID = d
			}
			if a := conv.AnyString(m["a1"]); a != "" {
				c.A1 = a
			}
			if w := conv.AnyString(m["web_id"]); w != "" {
				c.WebID = w
			}
			if w := conv.AnyString(m["webId"]); w != "" {
				c.WebID = w
			}
			if ex, ok := m["cookie_extra"].(map[string]any); ok {
				for k, v := range ex {
					if s := conv.AnyString(v); s != "" {
						c.extra[k] = s
					}
				}
			}
			if uid := conv.AnyString(m["user_id"]); uid != "" && c.UserID == "" {
				c.UserID = uid
			}
			if name := conv.AnyString(m["user_name"]); name != "" && c.UserName == "" {
				c.UserName = name
			}
			return
		}
	}
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if v == "" {
			continue
		}
		switch strings.ToLower(k) {
		case "access-token", "auth":
			c.AccessToken = v
		case "device-id":
			c.DeviceID = v
		case "a1":
			c.A1 = v
		case "webid":
			c.WebID = v
		case "xsecappid":
			// fixed for helper
		default:
			c.extra[k] = v
		}
	}
}

func (c *Client) wireCookie() string {
	c.ensureIdentity()
	parts := []string{
		"xsecappid=" + xsecAppLive,
		"a1=" + c.A1,
		"webId=" + c.WebID,
	}
	for _, k := range []string{"acw_tc", "websectiga", "sec_poison_id", "gid", "web_session"} {
		if v := c.extra[k]; v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	for k, v := range c.extra {
		switch k {
		case "acw_tc", "websectiga", "sec_poison_id", "gid", "web_session",
			"access-token", "auth", "device-id", "a1", "webId", "xsecappid":
			continue
		}
		if v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	return strings.Join(parts, "; ")
}

func (c *Client) cookieMap() map[string]string {
	c.ensureIdentity()
	m := map[string]string{
		"xsecappid": xsecAppLive,
		"a1":        c.A1,
		"webId":     c.WebID,
	}
	for k, v := range c.extra {
		if v != "" {
			m[k] = v
		}
	}
	return m
}

type doOpts struct {
	sign       bool
	redobs     bool
	originRobs bool
}

func (c *Client) do(
	method, host, path string,
	jsonBody any,
	query url.Values,
	opts doOpts,
) (map[string]any, error) {
	c.ensureIdentity()

	var bodyBytes []byte
	var signPayload any
	if jsonBody != nil {
		var err error
		bodyBytes, err = json.Marshal(jsonBody)
		if err != nil {
			return nil, err
		}
		signPayload = json.RawMessage(bodyBytes)
	}
	if method == http.MethodGet && query != nil {
		m := map[string]any{}
		for k, vs := range query {
			if len(vs) > 0 {
				m[k] = vs[0]
			}
		}
		signPayload = m
	}

	needSign := opts.sign || strings.Contains(host, "customer.xiaohongshu")
	var signHS map[string]string
	if needSign {
		var err error
		signHS, err = SignHeaders(method, path, xsecAppLive, c.cookieMap(), signPayload)
		if err != nil {
			return nil, fmt.Errorf("sign: %w", err)
		}
	}

	full := host + path
	if query != nil && len(query) > 0 {
		full += "?" + query.Encode()
	}
	var rdr io.Reader
	if bodyBytes != nil {
		rdr = bytes.NewReader(bodyBytes)
	}
	req, err := http.NewRequest(method, full, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Cookie", c.wireCookie())
	req.Header.Set("device-id", c.DeviceID)
	req.Header.Set("subsystem", c.Subsystem)
	if opts.originRobs {
		req.Header.Set("Origin", hostRobs)
		req.Header.Set("Referer", hostRobs+"/")
	} else {
		req.Header.Set("Origin", "https://www.xiaohongshu.com")
		req.Header.Set("Referer", "https://www.xiaohongshu.com/")
	}
	if c.AccessToken != "" {
		req.Header.Set("auth", c.AccessToken)
		req.Header.Set("access-token", c.AccessToken)
	}
	if bodyBytes != nil {
		req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	}
	if opts.redobs {
		req.Header.Set("xy-common-params", xyCommon)
		req.Header.Set("xy-platform-info", xyCommon)
	}
	for k, v := range signHS {
		req.Header.Set(k, v)
	}
	if req.Header.Get("x-t") == "" {
		req.Header.Set("x-t", fmt.Sprintf("%d", time.Now().UnixMilli()))
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", adapter.ErrNetwork, err)
	}
	defer resp.Body.Close()
	for _, sc := range resp.Header.Values("Set-Cookie") {
		part := strings.SplitN(sc, ";", 2)[0]
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "a1":
			c.A1 = v
			c.WebID = GenerateWebID(c.A1)
		case "webId":
			c.WebID = v
		case "web_session", "acw_tc", "websectiga", "sec_poison_id", "gid":
			c.extra[k] = v
		}
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("HTTP %d json: %w (%s)", resp.StatusCode, err, conv.Truncate(string(raw), 160))
	}
	out["_http"] = float64(resp.StatusCode)
	return out, nil
}

func bizOK(m map[string]any) bool {
	if m == nil {
		return false
	}
	if s, ok := m["success"].(bool); ok && s {
		return true
	}
	if m["code"] != nil && conv.AnyInt(m["code"]) == 0 {
		return true
	}
	if m["result"] != nil && conv.AnyInt(m["result"]) == 0 {
		return true
	}
	return false
}
