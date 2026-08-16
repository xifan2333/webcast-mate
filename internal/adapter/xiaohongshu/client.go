package xiaohongshu

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
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
type Client struct {
	http        *http.Client
	A1          string
	WebID       string
	AccessToken string
	DeviceID    string
	Subsystem   string
	UserID      string
	UserName    string
	extraCookie map[string]string
	// lastRoom from pre
	RoomID  string
	PushURL string
}

func NewClient() *Client {
	return &Client{
		http:        &http.Client{Timeout: 30 * time.Second},
		Subsystem:   "robs",
		DeviceID:    randomMAC(),
		extraCookie: map[string]string{},
	}
}

func randomMAC() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	b[0] |= 0x02 // local
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
}

// CookieHeader is secrets/stdout: cookie-like k=v list including AT + device-id + room.
// Wire HTTP Cookie uses wireCookieHeader() (browser cookies only).
func (c *Client) CookieHeader() string {
	c.ensureIdentity()
	parts := make([]string, 0, 12)
	if c.AccessToken != "" {
		parts = append(parts, "access-token="+c.AccessToken, "auth="+c.AccessToken)
	}
	if c.DeviceID != "" {
		parts = append(parts, "device-id="+c.DeviceID)
	}
	if c.RoomID != "" {
		parts = append(parts, "xhs-room-id="+c.RoomID)
	}
	parts = append(parts, c.wireCookieHeader())
	return strings.Join(parts, "; ")
}

func (c *Client) wireCookieHeader() string {
	c.ensureIdentity()
	parts := []string{
		"xsecappid=" + xsecAppLive,
		"a1=" + c.A1,
		"webId=" + c.WebID,
	}
	for _, k := range []string{"acw_tc", "websectiga", "sec_poison_id", "gid", "web_session"} {
		if v := c.extraCookie[k]; v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	for k, v := range c.extraCookie {
		switch k {
		case "xsecappid", "a1", "webId", "acw_tc", "websectiga", "sec_poison_id", "gid", "web_session",
			"access-token", "auth", "device-id", "xhs-room-id":
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
	for k, v := range c.extraCookie {
		if v != "" {
			m[k] = v
		}
	}
	return m
}

// SessionBlob is in-memory session; secrets store CookieHeader() string only.
type SessionBlob struct {
	AccessToken string
	DeviceID    string
	A1          string
	WebID       string
	CookieExtra map[string]string
	UserID      string
	UserName    string
	RoomID      string
	LoginAt     time.Time
}

func (c *Client) applySession(s *SessionBlob) {
	if s == nil {
		return
	}
	c.AccessToken = s.AccessToken
	if s.DeviceID != "" {
		c.DeviceID = s.DeviceID
	}
	if s.A1 != "" {
		c.A1 = s.A1
	}
	if s.WebID != "" {
		c.WebID = s.WebID
	}
	if s.CookieExtra != nil {
		c.extraCookie = s.CookieExtra
	}
	c.UserID = s.UserID
	c.UserName = s.UserName
	c.RoomID = s.RoomID
}

func (c *Client) sessionBlob() *SessionBlob {
	return &SessionBlob{
		AccessToken: c.AccessToken,
		DeviceID:    c.DeviceID,
		A1:          c.A1,
		WebID:       c.WebID,
		CookieExtra: c.extraCookie,
		UserID:      c.UserID,
		UserName:    c.UserName,
		RoomID:      c.RoomID,
		LoginAt:     time.Now().UTC(),
	}
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

	// CAS needs x-s; redobs start/stop often work without, but we sign when a1 present.
	needSign := opts.sign || strings.Contains(host, "customer.xiaohongshu")
	var signHS map[string]string
	if needSign {
		// override global xsec for live-helper
		prev := xsecAppID
		xsecAppID = xsecAppLive
		var err error
		signHS, err = SignHeaders(method, path, c.cookieMap(), signPayload)
		xsecAppID = prev
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
	req.Header.Set("Cookie", c.wireCookieHeader())
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
		return nil, err
	}
	defer resp.Body.Close()
	// absorb set-cookie extras
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
			if c.extraCookie == nil {
				c.extraCookie = map[string]string{}
			}
			c.extraCookie[k] = v
		}
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("HTTP %d json: %w (%s)", resp.StatusCode, err, truncate(string(raw), 160))
	}
	out["_http"] = float64(resp.StatusCode)
	return out, nil
}

type doOpts struct {
	sign       bool
	redobs     bool
	originRobs bool
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func anyString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%.0f", t)
	case json.Number:
		return t.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func anyInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		var n int
		fmt.Sscanf(t, "%d", &n)
		return n
	default:
		return 0
	}
}

func bizOK(m map[string]any) bool {
	if m == nil {
		return false
	}
	if s, ok := m["success"].(bool); ok && s {
		return true
	}
	if anyInt(m["code"]) == 0 && m["code"] != nil {
		return true
	}
	if anyInt(m["result"]) == 0 && m["result"] != nil {
		return true
	}
	return false
}

// md5 hex helper used if GenerateWebID missing edge
func md5HexStr(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}
