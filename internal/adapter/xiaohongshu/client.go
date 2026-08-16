package xiaohongshu

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	edithHost = "https://edith.xiaohongshu.com"
	wwwHost   = "https://www.xiaohongshu.com"
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"
)

// Client is the web/spectrum HTTP client (QR login + OBS push_url).
// Cookie model: a1/webId/web_session for sign + sticky edge cookies (acw_tc)
// echoed on every request. Without acw_tc echo, status after confirm resets.
type Client struct {
	http       *http.Client
	A1         string
	WebID      string
	WebSession string
	UserID     string
	// XsecAppID: xhs-pc-web for edith QR; spectrum for zhibo/obs.
	XsecAppID string
	// Extra cookies from Set-Cookie (acw_tc, websectiga, sec_poison_id, …).
	extra map[string]string
}

func NewClient() *Client {
	return &Client{
		http:      &http.Client{Timeout: 25 * time.Second},
		XsecAppID: "xhs-pc-web",
		extra:     map[string]string{},
	}
}

func (c *Client) cookieMap() map[string]string {
	m := map[string]string{"xsecappid": c.XsecAppID}
	if c.A1 != "" {
		m["a1"] = c.A1
	}
	if c.WebID != "" {
		m["webId"] = c.WebID
	}
	if c.WebSession != "" {
		m["web_session"] = c.WebSession
	}
	for k, v := range c.extra {
		if v != "" {
			m[k] = v
		}
	}
	return m
}

func (c *Client) cookieHeader() string {
	// Stable core order + extras (acw_tc first — WAF often wants it early).
	parts := make([]string, 0, 8)
	if v := c.extra["acw_tc"]; v != "" {
		parts = append(parts, "acw_tc="+v)
	}
	if c.A1 != "" {
		parts = append(parts, "a1="+c.A1)
	}
	if c.WebID != "" {
		parts = append(parts, "webId="+c.WebID)
	}
	if c.WebSession != "" {
		parts = append(parts, "web_session="+c.WebSession)
	}
	parts = append(parts, "xsecappid="+c.XsecAppID)
	for k, v := range c.extra {
		if k == "acw_tc" || v == "" {
			continue
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}

// do signed JSON request. host is full origin (edithHost or wwwHost).
// For GET, pass query; sign payload is that query map (keys sorted in SignXS).
// For POST, pass payload object/raw JSON.
func (c *Client) do(method, host, uri string, payload any, query url.Values) ([]byte, http.Header, error) {
	var bodyJSON []byte
	var err error
	var signPayload any = payload

	if method == http.MethodGet && query != nil {
		m := map[string]any{}
		for k, vs := range query {
			if len(vs) > 0 {
				m[k] = vs[0]
			}
		}
		signPayload = m
	}
	if payload != nil && method != http.MethodGet {
		switch t := payload.(type) {
		case json.RawMessage:
			bodyJSON = []byte(t)
			signPayload = json.RawMessage(bodyJSON)
		case []byte:
			bodyJSON = t
			signPayload = json.RawMessage(bodyJSON)
		default:
			bodyJSON, err = json.Marshal(payload)
			if err != nil {
				return nil, nil, err
			}
			signPayload = json.RawMessage(bodyJSON)
		}
	}

	cm := c.cookieMap()
	hs, err := SignHeaders(method, uri, cm, signPayload)
	if err != nil {
		return nil, nil, err
	}

	full := host + uri
	if query != nil && len(query) > 0 {
		// Encode in the same key order SignXS uses (alphabetical) so x-s matches wire URL.
		full += "?" + encodeQuerySorted(query)
	}

	var body io.Reader
	if bodyJSON != nil {
		body = bytes.NewReader(bodyJSON)
	}
	req, err := http.NewRequest(method, full, body)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", "https://www.xiaohongshu.com")
	req.Header.Set("Referer", "https://www.xiaohongshu.com/")
	req.Header.Set("Cookie", c.cookieHeader())
	if bodyJSON != nil {
		req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	}
	for k, v := range hs {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	// Absorb Set-Cookie into fields (web_session on login success).
	prevSession := c.WebSession
	c.applySetCookie(resp.Header)
	// XHS often returns HTTP 461/471 on captcha/WAF edges while body is still
	// valid JSON (code=0). Treat business JSON as success; only hard-fail 4xx/5xx
	// without a parseable success payload.
	if resp.StatusCode >= 400 {
		if bizOK(data) {
			return data, resp.Header, nil
		}
		// session upgraded via Set-Cookie alone (login confirm path)
		if c.WebSession != "" && c.WebSession != prevSession {
			return data, resp.Header, nil
		}
		return data, resp.Header, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	return data, resp.Header, nil
}

func bizOK(data []byte) bool {
	var top map[string]any
	if json.Unmarshal(data, &top) != nil {
		return false
	}
	if s, ok := top["success"].(bool); ok && s {
		return true
	}
	switch c := top["code"].(type) {
	case float64:
		return c == 0
	case int:
		return c == 0
	}
	return false
}

func encodeQuerySorted(q url.Values) string {
	// mirror url.Values.Encode (sorted keys)
	return q.Encode()
}

func (c *Client) applySetCookie(h http.Header) {
	// Go joins Set-Cookie; parse each.
	for _, line := range h.Values("Set-Cookie") {
		// name=value; Path=...
		part := strings.SplitN(line, ";", 2)[0]
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		switch k {
		case "web_session":
			if v != "" {
				c.WebSession = v
			}
		case "a1":
			if v != "" {
				c.A1 = v
				c.WebID = GenerateWebID(c.A1)
			}
		case "webId":
			if v != "" {
				c.WebID = v
			}
		}
	}
	if os.Getenv("WEBCAST_MATE_DEBUG") != "" {
		names := make([]string, 0)
		for _, line := range h.Values("Set-Cookie") {
			part := strings.SplitN(line, ";", 2)[0]
			if k, _, ok := strings.Cut(part, "="); ok {
				names = append(names, strings.TrimSpace(k))
			}
		}
		if len(names) > 0 {
			fmt.Fprintf(os.Stderr, "xiaohongshu: Set-Cookie names: %v session_len=%d\n", names, len(c.WebSession))
		}
	}
}

func (c *Client) doEdith(method, uri string, payload any) ([]byte, error) {
	c.XsecAppID = "xhs-pc-web"
	b, _, err := c.do(method, edithHost, uri, payload, nil)
	return b, err
}

func (c *Client) doEdithGET(uri string, query url.Values) ([]byte, error) {
	c.XsecAppID = "xhs-pc-web"
	b, _, err := c.do(http.MethodGet, edithHost, uri, nil, query)
	return b, err
}

func (c *Client) doSpectrumGET(uri string, query url.Values) ([]byte, error) {
	c.XsecAppID = "spectrum"
	b, _, err := c.do(http.MethodGet, wwwHost, uri, nil, query)
	return b, err
}

// DebugGET spectrum host.
func (c *Client) DebugGET(uri string, query url.Values) ([]byte, error) {
	return c.doSpectrumGET(uri, query)
}

// DebugEdithGET edith host.
func (c *Client) DebugEdithGET(uri string, query url.Values) ([]byte, error) {
	return c.doEdithGET(uri, query)
}

// DebugEdithPOST edith host.
func (c *Client) DebugEdithPOST(uri string, payload any) ([]byte, error) {
	return c.doEdith(http.MethodPost, uri, payload)
}

func randomPubKeyB64() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// EnsureWebGuest: a1 + activate → tourist web_session (needed before QR create).
func (c *Client) EnsureWebGuest() error {
	if c.A1 != "" && c.WebSession != "" {
		return nil
	}
	if c.A1 == "" {
		c.A1 = GenerateA1()
	}
	if c.WebID == "" {
		c.WebID = GenerateWebID(c.A1)
	}
	c.XsecAppID = "xhs-pc-web"
	uri := "/api/sns/web/v1/login/activate"
	// Official web posts {} or client_public_key; both work. Prefer {}.
	b, err := c.doEdith(http.MethodPost, uri, json.RawMessage(`{}`))
	if err != nil {
		// fallback key form
		b, err = c.doEdith(http.MethodPost, uri, map[string]any{
			"client_public_key_base64": randomPubKeyB64(),
		})
		if err != nil {
			return err
		}
	}
	var act struct {
		Code    int  `json:"code"`
		Success bool `json:"success"`
		Data    struct {
			UserID  string `json:"user_id"`
			Session string `json:"session"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &act); err != nil {
		return err
	}
	if !act.Success || act.Code != 0 || act.Data.Session == "" {
		return fmt.Errorf("activate failed: %s", truncate(string(b), 200))
	}
	c.WebSession = act.Data.Session
	c.UserID = act.Data.UserID
	return nil
}
