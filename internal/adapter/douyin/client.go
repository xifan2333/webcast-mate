package douyin

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	hostStreaming = "https://streamingtool.douyin.com"
	hostPC        = "https://webcast-pc.amemv.com"
	hostAPI       = "https://webcast.amemv.com"
	aid           = "2079"
	appVersion    = "12.7.3"
	userAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) webcast_mate/12.7.3 Chrome/136.0.7103.59 Electron/36.4.0-rs.18.release.webcast.71 TTElectron/36.4.0-rs.18.release.webcast.71 Safari/537.36"
)

// ROOM_STATUS
const (
	RoomPrepare = 1
	RoomLiving  = 2
	RoomPause   = 3
	RoomFinish  = 4
)

// Client is streamingtool / webcast HTTP client (cookie jar).
type Client struct {
	http     *http.Client
	jar      http.CookieJar
	DeviceID string
	IID      string
}

func NewClient() (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	c := &Client{
		http: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
		jar:      jar,
		DeviceID: envOr("WEBCAST_MATE_DY_DEVICE_ID", "1325358198338004"),
		IID:      envOr("WEBCAST_MATE_DY_IID", "1325358197522916"),
	}
	return c, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func (c *Client) commonQuery() url.Values {
	q := url.Values{}
	q.Set("ac", "wifi")
	q.Set("app_name", "webcast_mate")
	q.Set("version_code", appVersion)
	q.Set("device_platform", "windows")
	q.Set("webcast_sdk_version", "1520")
	q.Set("resolution", "1366*768")
	q.Set("os_version", "10.0.19045")
	q.Set("language", "zh")
	q.Set("aid", aid)
	q.Set("live_id", "1")
	q.Set("channel", "online")
	q.Set("device_id", c.DeviceID)
	q.Set("iid", c.IID)
	q.Set("extra_first_tag_id", "16")
	q.Set("extra_second_tag_id", "16069")
	q.Set("extra_third_tag_id", "16069163")
	q.Set("extra_encoder_core", "cpu")
	q.Set("extra_codec_name", "bytevc0")
	q.Set("extra_codec_is_ex", "0")
	q.Set("extra_use_265", "0")
	return q
}

func (c *Client) sdkParams() url.Values {
	q := url.Values{}
	q.Set("passport_jssdk_version", "2.4.13")
	q.Set("passport_jssdk_type", "normal")
	q.Set("is_from_ttaccountsdk", "1")
	q.Set("aid", aid)
	q.Set("language", "zh")
	q.Set("is_from_iesaccountsaas", "1")
	q.Set("account_sdk_source", "web")
	q.Set("device_id", c.DeviceID)
	q.Set("iid", c.IID)
	q.Set("install_id", c.IID)
	q.Set("version_code", appVersion)
	q.Set("app_name", "webcast_mate")
	q.Set("channel", "online")
	q.Set("device_platform", "windows")
	q.Set("device_type", "windows")
	q.Set("os_version", "10.0.19045")
	q.Set("fp", "verify_"+c.DeviceID)
	return q
}

func (c *Client) setCookieHeader(header string) {
	if header == "" {
		return
	}
	// Apply to all relevant hosts
	var cks []*http.Cookie
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		k, v, ok := strings.Cut(part, "=")
		if !ok || k == "" {
			continue
		}
		cks = append(cks, &http.Cookie{Name: strings.TrimSpace(k), Value: strings.TrimSpace(v), Path: "/"})
	}
	for _, h := range []string{hostStreaming, hostPC, hostAPI, "https://www.douyin.com"} {
		u, _ := url.Parse(h)
		c.jar.SetCookies(u, cks)
	}
}

func (c *Client) cookieHeader() string {
	seen := map[string]string{}
	for _, h := range []string{hostStreaming, hostPC, hostAPI} {
		u, _ := url.Parse(h)
		for _, ck := range c.jar.Cookies(u) {
			if ck.Value != "" {
				seen[ck.Name] = ck.Value
			}
		}
	}
	// stable-ish order
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	// prefer known session keys first
	prefer := []string{
		"sessionid", "sessionid_ss", "sid_tt", "sid_guard", "sid_ucp_v1", "ssid_ucp_v1",
		"uid_tt", "uid_tt_ss", "odin_tt", "passport_assist_user",
		"passport_csrf_token", "passport_csrf_token_default", "ttwid",
	}
	parts := make([]string, 0, len(seen))
	used := map[string]bool{}
	for _, k := range prefer {
		if v, ok := seen[k]; ok {
			parts = append(parts, k+"="+v)
			used[k] = true
		}
	}
	for k, v := range seen {
		if !used[k] {
			parts = append(parts, k+"="+v)
		}
	}
	return strings.Join(parts, "; ")
}

func (c *Client) csrfToken() string {
	u, _ := url.Parse(hostStreaming)
	for _, ck := range c.jar.Cookies(u) {
		if ck.Name == "passport_csrf_token" || ck.Name == "passport_csrf_token_default" {
			return ck.Value
		}
	}
	// scan all
	for _, part := range strings.Split(c.cookieHeader(), ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && (k == "passport_csrf_token" || k == "passport_csrf_token_default") {
			return v
		}
	}
	return ""
}

func (c *Client) do(method, fullURL string, body io.Reader, contentType string, extra map[string]string) ([]byte, error) {
	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "zh-CN")
	req.Header.Set("Origin", hostStreaming)
	req.Header.Set("Referer", hostStreaming+"/")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if t := c.csrfToken(); t != "" {
		req.Header.Set("x-tt-passport-csrf-token", t)
	}
	for k, v := range extra {
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
		return b, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(b), 200))
	}
	return b, nil
}

func (c *Client) getJSON(fullURL string, extra map[string]string) (map[string]any, error) {
	b, err := c.do(http.MethodGet, fullURL, nil, "", extra)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("json: %w (%s)", err, truncate(string(b), 160))
	}
	return m, nil
}

func (c *Client) postForm(fullURL string, form url.Values, extra map[string]string) (map[string]any, error) {
	body := strings.NewReader(form.Encode())
	b, err := c.do(http.MethodPost, fullURL, body, "application/x-www-form-urlencoded; charset=UTF-8", extra)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("json: %w (%s)", err, truncate(string(b), 160))
	}
	return m, nil
}

func (c *Client) postJSON(fullURL string, payload any) (map[string]any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	b, err := c.do(http.MethodPost, fullURL, strings.NewReader(string(raw)), "application/json", nil)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func ticketData() string {
	payload := fmt.Sprintf(`{"req_content":"ticket,path,timestamp","timestamp":%d}`, time.Now().Unix())
	return base64.StdEncoding.EncodeToString([]byte(payload))
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
		// avoid scientific for ids
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

func mapData(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	d, _ := m["data"].(map[string]any)
	return d
}
