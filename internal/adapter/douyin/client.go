package douyin

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/adapter/httpx"
	"github.com/xifan2333/webcast-mate/internal/conv"
	"github.com/xifan2333/webcast-mate/internal/secrets"
)

const (
	hostStreaming = "https://streamingtool.douyin.com"
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

var (
	dyCookieSetHosts  = []string{hostStreaming, hostAPI, "https://www.douyin.com"}
	dyCookieReadHosts = []string{hostStreaming, hostAPI}
	dyCookiePrefer    = []string{
		"sessionid", "sessionid_ss", "sid_tt", "sid_guard", "sid_ucp_v1", "ssid_ucp_v1",
		"uid_tt", "uid_tt_ss", "odin_tt", "passport_assist_user",
		"passport_csrf_token", "passport_csrf_token_default", "ttwid",
	}
)

// Client is streamingtool / webcast HTTP client (cookie jar).
type Client struct {
	*httpx.Client
	DeviceID string
	IID      string
}

func NewClient() *Client {
	return &Client{
		Client:   httpx.New(),
		DeviceID: envOr("WEBCAST_MATE_DY_DEVICE_ID", ""),
		IID:      envOr("WEBCAST_MATE_DY_IID", ""),
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func (c *Client) commonQuery() url.Values {
	return c.commonQueryWithExtra(true)
}

func (c *Client) ApplySecrets(f *secrets.File) {
	if f == nil {
		return
	}
	f.Normalize()
	if h := f.CookieHeader(); h != "" {
		c.SetCookieHeader(h, "", dyCookieSetHosts...)
	}
	if did := f.Params["did"]; did != "" {
		c.DeviceID = did
	}
	if iid := f.Params["iid"]; iid != "" {
		c.IID = iid
	}
}

func (c *Client) ExportSecrets(userID, userName string, loginAt time.Time) *secrets.File {
	f := &secrets.File{Version: secrets.Version, UserID: userID, UserName: userName, LoginAt: loginAt,
		Cookies: secrets.ParseCookieHeader(c.cookieHeader()), Headers: map[string]string{}, Params: map[string]string{}}
	if c.DeviceID != "" {
		f.Params["did"] = c.DeviceID
	}
	if c.IID != "" {
		f.Params["iid"] = c.IID
	}
	f.Normalize()
	return f
}

func (c *Client) commonQueryWithExtra(withExtra bool) url.Values {
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
	if withExtra {
		q.Set("extra_first_tag_id", "16")
		q.Set("extra_second_tag_id", "16069")
		q.Set("extra_third_tag_id", "16069163")
		q.Set("extra_encoder_core", "cpu")
		q.Set("extra_codec_name", "bytevc0")
		q.Set("extra_codec_is_ex", "0")
		q.Set("extra_use_265", "0")
	}
	return q
}

func (c *Client) pingQuery() url.Values { return c.commonQueryWithExtra(false) }

func (c *Client) passportQuery() url.Values {
	q := url.Values{}
	q.Set("passport_jssdk_version", "2.4.13")
	q.Set("passport_jssdk_type", "normal")
	q.Set("is_from_ttaccountsdk", "1")
	q.Set("aid", aid)
	q.Set("language", "zh")
	q.Set("is_from_iesaccountsaas", "1")
	q.Set("account_sdk_source", "web")
	q.Set("account_sdk_source_info", accountSDKSourceInfo())
	q.Set("p_js_v", "2.4.13")
	q.Set("p_js_t", "pro")
	q.Set("p_zt", "3.3.5")
	q.Set("p_ver", "1.0.29")
	q.Set("request_host", "file%3A%2F%2F")
	q.Set("p_bd", "1.0.1.20")
	q.Set("biz_trace_id", fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff))
	q.Set("isBoe", "false")
	q.Set("loginType[]", "LOGIN_MOBILE_CODE")
	q.Set("did", c.DeviceID)
	q.Set("iid", c.IID)
	q.Set("host", hostStreaming)
	q.Set("domain", hostStreaming)
	q.Set("captchaHost", "https://verify.snssdk.com")
	q.Set("version_code", appVersion)
	q.Set("app_name", "aweme_live_assistant")
	q.Set("channel", "online")
	q.Set("device_platform", "Windows")
	q.Set("device_type", "4291MU5")
	q.Set("os_version", "10.0.19045")
	q.Set("unionLoginPanel", "true")
	q.Set("globalMobileSupport", "true")
	return q
}

func withABogus(queryEncoded, body string) (string, error) {
	ab, err := SignABogus(queryEncoded, body)
	if err != nil {
		return "", err
	}
	return queryEncoded + "&a_bogus=" + url.QueryEscape(ab), nil
}

func (c *Client) cookieHeader() string {
	return c.CookieString(dyCookieReadHosts, dyCookiePrefer)
}

func (c *Client) csrfToken() string {
	return c.CookieValue(dyCookieReadHosts, []string{"passport_csrf_token", "passport_csrf_token_default"})
}

func (c *Client) do(method, fullURL string, body io.Reader, contentType string, extra map[string]string) ([]byte, error) {
	b, _, err := c.doHDR(method, fullURL, body, contentType, extra)
	return b, err
}

// doHDR is do + response headers (needed for x-tt-verify-passport-decision etc).
func (c *Client) doHDR(method, fullURL string, body io.Reader, contentType string, extra map[string]string) ([]byte, http.Header, error) {
	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, nil, err
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
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", adapter.ErrNetwork, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.Header, err
	}
	if resp.StatusCode >= 400 {
		return b, resp.Header, fmt.Errorf("%w: HTTP %d: %s", adapter.ErrNetwork, resp.StatusCode, conv.Truncate(string(b), 200))
	}
	return b, resp.Header.Clone(), nil
}

func (c *Client) getJSON(fullURL string, extra map[string]string) (map[string]any, error) {
	b, err := c.do(http.MethodGet, fullURL, nil, "", extra)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("json: %w (%s)", err, conv.Truncate(string(b), 160))
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
		return nil, fmt.Errorf("json: %w (%s)", err, conv.Truncate(string(b), 160))
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

func mapData(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	d, _ := m["data"].(map[string]any)
	return d
}
