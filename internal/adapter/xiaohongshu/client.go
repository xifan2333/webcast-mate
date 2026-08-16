package xiaohongshu

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	edithHost = "https://edith.xiaohongshu.com"
	robsHost  = "https://robs.xiaohongshu.com"
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"
	// OBS helper PC query string (from XiaoHongShu_OBS)
	pcQS = "build=2200002&platform=pc&system_version=10.0.22000&cpu_model=Intel&gpu=ANGLE&is_win_7=false"
)

// Client holds web cookies (for QR) and optional robs sid (for live).
type Client struct {
	http       *http.Client
	A1         string
	WebID      string
	WebSession string
	UserID     string
	// Sid is robs access_token (Header sid). Populated by SMS login or future bridge.
	Sid      string
	DeviceID string
}

func NewClient() *Client {
	return &Client{
		http:     &http.Client{Timeout: 25 * time.Second},
		DeviceID: randomDeviceID(),
	}
}

func (c *Client) cookieMap() map[string]string {
	m := map[string]string{"xsecappid": xsecAppID}
	if c.A1 != "" {
		m["a1"] = c.A1
	}
	if c.WebID != "" {
		m["webId"] = c.WebID
	}
	if c.WebSession != "" {
		m["web_session"] = c.WebSession
	}
	return m
}

func (c *Client) cookieHeader() string {
	parts := make([]string, 0, 4)
	for k, v := range c.cookieMap() {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}

func (c *Client) doEdith(method, uri string, payload any) ([]byte, http.Header, error) {
	var bodyJSON []byte
	var err error
	if payload != nil {
		bodyJSON, err = json.Marshal(payload)
		if err != nil {
			return nil, nil, err
		}
	}
	hs, err := SignHeaders(method, uri, c.cookieMap(), payload)
	if err != nil {
		return nil, nil, err
	}
	var body io.Reader
	if bodyJSON != nil {
		body = bytes.NewReader(bodyJSON)
	}
	req, err := http.NewRequest(method, edithHost+uri, body)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://www.xiaohongshu.com/")
	req.Header.Set("Origin", "https://www.xiaohongshu.com")
	req.Header.Set("Accept", "application/json, text/plain, */*")
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
	if resp.StatusCode >= 400 {
		return data, resp.Header, fmt.Errorf("edith HTTP %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	return data, resp.Header, nil
}

func (c *Client) doRobs(method, path string, sid string, jsonBody any) ([]byte, error) {
	var body io.Reader
	if jsonBody != nil {
		b, err := json.Marshal(jsonBody)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, robsHost+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent+" live-helper/2.6.6")
	req.Header.Set("device-id", c.DeviceID)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	if sid != "" {
		req.Header.Set("sid", sid)
	}
	if jsonBody != nil {
		req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	}
	// form posts for sms
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return data, fmt.Errorf("robs HTTP %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	return data, nil
}

func (c *Client) doRobsForm(path string, form map[string]string) ([]byte, http.Header, error) {
	vals := make([]string, 0, len(form))
	for k, v := range form {
		vals = append(vals, k+"="+urlQueryEscape(v))
	}
	body := strings.Join(vals, "&")
	req, err := http.NewRequest(http.MethodPost, robsHost+path, strings.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", userAgent+" live-helper/2.6.6")
	req.Header.Set("device-id", c.DeviceID)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	return data, resp.Header, nil
}

func urlQueryEscape(s string) string {
	// minimal escape for phone numbers
	r := strings.NewReplacer(" ", "%20", "+", "%2B")
	return r.Replace(s)
}

func randomDeviceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
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

// EnsureWebGuest initializes a1/webId/web_session for signed edith calls.
func (c *Client) EnsureWebGuest() error {
	if c.A1 != "" && c.WebSession != "" {
		return nil
	}
	c.A1 = GenerateA1()
	c.WebID = GenerateWebID(c.A1)
	uri := "/api/sns/web/v1/login/activate"
	payload := map[string]any{"client_public_key_base64": randomPubKeyB64()}
	b, _, err := c.doEdith(http.MethodPost, uri, payload)
	if err != nil {
		return err
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
