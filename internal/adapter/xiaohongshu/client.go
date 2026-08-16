package xiaohongshu

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const (
	edithHost = "https://edith.xiaohongshu.com"
	wwwHost   = "https://www.xiaohongshu.com"
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"
)

// Client is web/spectrum client (QR login + OBS 6-digit push_url).
type Client struct {
	http       *http.Client
	jar        http.CookieJar
	A1         string
	WebID      string
	WebSession string
	UserID     string
	// XsecAppID: xhs-pc-web for QR/edith, spectrum for zhibo/obs
	XsecAppID string
}

func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		http:      &http.Client{Timeout: 25 * time.Second, Jar: jar},
		jar:       jar,
		XsecAppID: "xhs-pc-web",
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
	// merge jar cookies for www/edith
	for _, host := range []string{edithHost, wwwHost} {
		u, _ := url.Parse(host)
		for _, ck := range c.jar.Cookies(u) {
			if _, ok := m[ck.Name]; !ok {
				m[ck.Name] = ck.Value
			}
		}
	}
	return m
}

func (c *Client) cookieHeader() string {
	parts := make([]string, 0, 8)
	for k, v := range c.cookieMap() {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}

func (c *Client) seedJar() {
	for _, host := range []string{edithHost, wwwHost} {
		u, _ := url.Parse(host)
		var cks []*http.Cookie
		if c.A1 != "" {
			cks = append(cks, &http.Cookie{Name: "a1", Value: c.A1, Path: "/"})
		}
		if c.WebID != "" {
			cks = append(cks, &http.Cookie{Name: "webId", Value: c.WebID, Path: "/"})
		}
		if c.WebSession != "" {
			cks = append(cks, &http.Cookie{Name: "web_session", Value: c.WebSession, Path: "/"})
		}
		cks = append(cks, &http.Cookie{Name: "xsecappid", Value: c.XsecAppID, Path: "/"})
		c.jar.SetCookies(u, cks)
	}
}

func (c *Client) doJSON(method, host, uri string, payload any, query url.Values) ([]byte, error) {
	var bodyJSON []byte
	var err error
	signPayload := payload
	if query != nil && len(query) > 0 && payload == nil {
		// GET: sign with map from query
		m := map[string]any{}
		for k, vs := range query {
			if len(vs) > 0 {
				m[k] = vs[0]
			}
		}
		signPayload = m
	}
	if payload != nil {
		bodyJSON, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		signPayload = json.RawMessage(bodyJSON)
	}
	// ensure cookies for sign include xsecappid
	cm := c.cookieMap()
	cm["xsecappid"] = c.XsecAppID
	hs, err := SignHeaders(method, uri, cm, signPayload)
	if err != nil {
		return nil, err
	}
	u := host + uri
	if query != nil && len(query) > 0 {
		u += "?" + query.Encode()
	}
	var body io.Reader
	if bodyJSON != nil {
		body = bytes.NewReader(bodyJSON)
	}
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://www.xiaohongshu.com/zhibo/obs")
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
		return nil, err
	}
	defer resp.Body.Close()
	// pull cookies from jar into fields
	uWww, _ := url.Parse(wwwHost)
	for _, ck := range c.jar.Cookies(uWww) {
		switch ck.Name {
		case "web_session":
			if ck.Value != "" {
				c.WebSession = ck.Value
			}
		case "a1":
			if ck.Value != "" {
				c.A1 = ck.Value
			}
		}
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return data, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	return data, nil
}

func (c *Client) doEdith(method, uri string, payload any) ([]byte, error) {
	c.XsecAppID = "xhs-pc-web"
	c.seedJar()
	return c.doJSON(method, edithHost, uri, payload, nil)
}

func (c *Client) doSpectrumGET(uri string, query url.Values) ([]byte, error) {
	c.XsecAppID = "spectrum"
	c.seedJar()
	return c.doJSON(http.MethodGet, wwwHost, uri, nil, query)
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

// EnsureWebGuest initializes tourist web_session for QR.
func (c *Client) EnsureWebGuest() error {
	if c.A1 != "" && c.WebSession != "" {
		c.seedJar()
		return nil
	}
	c.A1 = GenerateA1()
	c.WebID = GenerateWebID(c.A1)
	c.XsecAppID = "xhs-pc-web"
	c.seedJar()
	uri := "/api/sns/web/v1/login/activate"
	payload := map[string]any{"client_public_key_base64": randomPubKeyB64()}
	b, err := c.doEdith(http.MethodPost, uri, payload)
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
	c.seedJar()
	return nil
}
