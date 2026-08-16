package xiaohongshu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"github.com/xifan2333/webcast-mate/internal/secrets"
	"github.com/xifan2333/webcast-mate/internal/termimg"
)

// SecretData stored as JSON in secrets/xiaohongshu.json.
type SecretData struct {
	Cookie   string    `json:"cookie"`
	UserID   string    `json:"user_id,omitempty"`
	UserName string    `json:"user_name,omitempty"`
	LoginAt  time.Time `json:"login_at,omitempty"`
}

func loadSecret() (*SecretData, error) {
	f, err := secrets.Load("xiaohongshu")
	if err != nil {
		return nil, err
	}
	if len(f.Cookie) > 0 && f.Cookie[0] == '{' {
		var s SecretData
		if err := json.Unmarshal([]byte(f.Cookie), &s); err != nil {
			return nil, err
		}
		return &s, nil
	}
	return &SecretData{Cookie: f.Cookie, UserID: f.UserID, UserName: f.UserName, LoginAt: f.LoginAt}, nil
}

func saveSecret(s *SecretData) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return secrets.Save("xiaohongshu", &secrets.File{
		Cookie:   string(b),
		UserID:   s.UserID,
		UserName: s.UserName,
		LoginAt:  s.LoginAt,
	})
}

func (c *Client) applySecret(s *SecretData) {
	if s == nil || s.Cookie == "" {
		return
	}
	for _, part := range strings.Split(s.Cookie, ";") {
		part = strings.TrimSpace(part)
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "a1":
			c.A1 = strings.TrimSpace(v)
		case "webId":
			c.WebID = strings.TrimSpace(v)
		case "web_session":
			c.WebSession = strings.TrimSpace(v)
		}
	}
	c.UserID = s.UserID
	c.seedJar()
}

// EnsureLogin loads web session or runs QR login.
func (c *Client) EnsureLogin(ctx context.Context) (*SecretData, error) {
	if s, err := loadSecret(); err == nil && s.Cookie != "" {
		c.applySecret(s)
		if c.WebSession != "" {
			return s, nil
		}
	}
	return c.loginQR(ctx)
}

func (c *Client) loginQR(ctx context.Context) (*SecretData, error) {
	if err := c.EnsureWebGuest(); err != nil {
		return nil, err
	}
	uri := "/api/sns/web/v1/login/qrcode/create"
	payload := map[string]any{"qr_type": 1}
	b, err := c.doEdith(http.MethodPost, uri, payload)
	if err != nil {
		return nil, err
	}
	var created struct {
		Code    int  `json:"code"`
		Success bool `json:"success"`
		Data    struct {
			URL  string `json:"url"`
			Code string `json:"code"`
			QRID string `json:"qr_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &created); err != nil {
		return nil, err
	}
	if !created.Success || created.Code != 0 || created.Data.QRID == "" {
		return nil, fmt.Errorf("qr create: %s", truncate(string(b), 200))
	}

	fmt.Fprintln(os.Stderr, "xiaohongshu: scan QR with xiaohongshu app")
	fmt.Fprintln(os.Stderr, created.Data.URL)
	printQR(created.Data.URL)

	deadline := time.Now().Add(3 * time.Minute)
	stURI := "/api/sns/web/v1/login/qrcode/status"
	lastStatus := -1
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, ErrQRTimeout
		}
		params := map[string]any{"code": created.Data.Code, "qr_id": created.Data.QRID}
		q := url.Values{}
		q.Set("code", created.Data.Code)
		q.Set("qr_id", created.Data.QRID)
		c.XsecAppID = "xhs-pc-web"
		hs, err := SignHeaders(http.MethodGet, stURI, c.cookieMap(), params)
		if err != nil {
			return nil, err
		}
		req, _ := http.NewRequest(http.MethodGet, edithHost+stURI+"?"+q.Encode(), nil)
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Referer", "https://www.xiaohongshu.com/")
		req.Header.Set("Origin", "https://www.xiaohongshu.com")
		req.Header.Set("Cookie", c.cookieHeader())
		for k, v := range hs {
			req.Header.Set(k, v)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var st struct {
			Code int `json:"code"`
			Data struct {
				CodeStatus int `json:"code_status"`
				LoginInfo  struct {
					Session string `json:"session"`
					UserID  string `json:"user_id"`
				} `json:"login_info"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &st); err != nil || st.Code != 0 {
			fmt.Fprintf(os.Stderr, "xiaohongshu: qr status: %s\n", truncate(string(body), 160))
			time.Sleep(time.Second)
			continue
		}
		if st.Data.CodeStatus != lastStatus {
			lastStatus = st.Data.CodeStatus
			switch st.Data.CodeStatus {
			case 0:
				fmt.Fprintln(os.Stderr, "xiaohongshu: waiting for scan…")
			case 1:
				fmt.Fprintln(os.Stderr, "xiaohongshu: scanned, confirm on phone")
			case 2:
				fmt.Fprintln(os.Stderr, "xiaohongshu: confirmed")
			case 3:
				fmt.Fprintln(os.Stderr, "xiaohongshu: qr expired")
			}
		}
		switch st.Data.CodeStatus {
		case 0, 1:
		case 2:
			applyQRLoginPayload(c, body, st.Data.LoginInfo.Session, st.Data.LoginInfo.UserID)
			c.seedJar()
			s := &SecretData{Cookie: c.cookieHeader(), UserID: c.UserID, LoginAt: time.Now().UTC()}
			if err := saveSecret(s); err != nil {
				return nil, err
			}
			fmt.Fprintln(os.Stderr, "xiaohongshu: web QR login ok")
			return s, nil
		case 3:
			return nil, ErrQRExpired
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func applyQRLoginPayload(c *Client, body []byte, sessionHint, userHint string) {
	if sessionHint != "" {
		c.WebSession = sessionHint
	}
	if userHint != "" {
		c.UserID = userHint
	}
	var raw map[string]any
	if json.Unmarshal(body, &raw) != nil {
		return
	}
	data, _ := raw["data"].(map[string]any)
	if data == nil {
		return
	}
	for _, k := range []string{"session", "web_session", "secure_session"} {
		if s, ok := data[k].(string); ok && s != "" {
			c.WebSession = s
		}
	}
	if uid, ok := data["user_id"].(string); ok && uid != "" {
		c.UserID = uid
	}
	if li, ok := data["login_info"].(map[string]any); ok {
		if s, ok := li["session"].(string); ok && s != "" {
			c.WebSession = s
		}
		if uid, ok := li["user_id"].(string); ok && uid != "" {
			c.UserID = uid
		}
	}
}

func printQR(content string) {
	if content == "" {
		return
	}
	q, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return
	}
	if termimg.SupportsKitty() {
		if png, err := q.PNG(280); err == nil && termimg.WriteKittyPNG(os.Stderr, png) == nil {
			return
		}
	}
	fmt.Fprint(os.Stderr, q.ToSmallString(false))
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
