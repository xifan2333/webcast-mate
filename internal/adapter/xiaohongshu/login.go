package xiaohongshu

import (
	"context"
	"encoding/json"
	"fmt"
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
	if c.extra == nil {
		c.extra = map[string]string{}
	}
	for _, part := range strings.Split(s.Cookie, ";") {
		part = strings.TrimSpace(part)
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		switch k {
		case "a1":
			c.A1 = v
		case "webId":
			c.WebID = v
		case "web_session":
			c.WebSession = v
		case "xsecappid":
			// ignore; set per request
		default:
			if v != "" {
				c.extra[k] = v
			}
		}
	}
	if c.WebID == "" && c.A1 != "" {
		c.WebID = GenerateWebID(c.A1)
	}
	c.UserID = s.UserID
}

// EnsureLogin loads web session or runs QR login.
func (c *Client) EnsureLogin(ctx context.Context) (*SecretData, error) {
	if s, err := loadSecret(); err == nil && s.Cookie != "" {
		c.applySecret(s)
		if c.WebSession != "" && c.A1 != "" {
			return s, nil
		}
	}
	return c.loginQR(ctx)
}

// loginQR — protocol from public reverse notes / RedNote-Skill:
//
//	POST /api/sns/web/v1/login/qrcode/create  {"qr_type":1}
//	  → data.url, data.qr_id, data.code
//	GET  /api/sns/web/v1/login/qrcode/status?code=&qr_id=
//	  code_status: 0 wait | 1 scanned | 2 ok | 3 expired
//	  on 2: data.login_info.session → web_session cookie
func (c *Client) loginQR(ctx context.Context) (*SecretData, error) {
	if err := c.EnsureWebGuest(); err != nil {
		return nil, err
	}

	// 1) create
	createBody := json.RawMessage(`{"qr_type":1}`)
	b, err := c.doEdith("POST", "/api/sns/web/v1/login/qrcode/create", createBody)
	if err != nil {
		return nil, fmt.Errorf("qr create: %w", err)
	}
	var created struct {
		Code    int    `json:"code"`
		Success bool   `json:"success"`
		Msg     string `json:"msg"`
		Data    struct {
			URL  string `json:"url"`
			Code string `json:"code"`
			QRID string `json:"qr_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &created); err != nil {
		return nil, fmt.Errorf("qr create parse: %w (%s)", err, truncate(string(b), 180))
	}
	if !created.Success || created.Code != 0 || created.Data.QRID == "" || created.Data.Code == "" {
		return nil, fmt.Errorf("qr create: %s", truncate(string(b), 200))
	}
	qrURL := created.Data.URL
	if qrURL == "" {
		qrURL = "https://www.xiaohongshu.com/login?qr_code=" + created.Data.Code
	}

	fmt.Fprintln(os.Stderr, "xiaohongshu: scan with 小红书 App")
	fmt.Fprintln(os.Stderr, qrURL)
	printQR(qrURL)
	fmt.Fprintf(os.Stderr, "xiaohongshu: qr_id=%s code=%s\n", created.Data.QRID, created.Data.Code)

	// 2) poll status every 1s (same as official web)
	deadline := time.Now().Add(3 * time.Minute)
	last := -1
	n := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, ErrQRTimeout
		}

		q := url.Values{}
		// alphabetical: code, qr_id — matches SignXS sorted GET + url.Values.Encode
		q.Set("code", created.Data.Code)
		q.Set("qr_id", created.Data.QRID)

		prevSession := c.WebSession
		body, hdr, err := c.do(http.MethodGet, edithHost, "/api/sns/web/v1/login/qrcode/status", nil, q)
		n++
		// Even on transport/HTTP errors, body may still carry business JSON (471).
		st, apiCode, loginSession, loginUID, ok := parseStatus(body)

		// After phone confirm, XHS sometimes returns HTTP 471 + empty data{},
		// with the real session only in Set-Cookie (already applied in do).
		sessionBumped := c.WebSession != "" && c.WebSession != prevSession
		if sessionBumped && (st == 2 || st == 0 && last == 1 || (ok && apiCode == 0 && last == 1)) {
			fmt.Fprintln(os.Stderr, "xiaohongshu: confirmed (session via Set-Cookie)")
			return c.finishQRLogin(body, loginSession, loginUID)
		}

		if err != nil && !ok {
			if n <= 5 || n%5 == 0 {
				fmt.Fprintf(os.Stderr, "xiaohongshu: status err: %v\n", err)
				if hdr != nil {
					if v := hdr.Get("verifytype"); v != "" || hdr.Get("verifyuuid") != "" {
						fmt.Fprintf(os.Stderr, "xiaohongshu: captcha headers verifytype=%s verifyuuid=%s\n",
							hdr.Get("verifytype"), hdr.Get("verifyuuid"))
					}
				}
			}
			sleep(ctx, time.Second)
			continue
		}
		if !ok || apiCode != 0 {
			if n <= 5 || n%5 == 0 {
				fmt.Fprintf(os.Stderr, "xiaohongshu: status body: %s\n", truncate(string(body), 220))
			}
			// empty success after scan → keep polling briefly; may still set cookie
			sleep(ctx, time.Second)
			continue
		}

		// empty data{} after scanned: treat as transitional, not failure
		if ok && apiCode == 0 && st == 0 && last == 1 && len(body) > 0 {
			// body like {"code":0,"success":true,"data":{}} after confirm
			if sessionBumped {
				fmt.Fprintln(os.Stderr, "xiaohongshu: confirmed (empty data, session updated)")
				return c.finishQRLogin(body, loginSession, loginUID)
			}
			if n <= 8 {
				fmt.Fprintf(os.Stderr, "xiaohongshu: post-confirm empty data, polling… (%s)\n", truncate(string(body), 120))
			}
		}

		if st != last {
			last = st
			switch st {
			case 0:
				fmt.Fprintln(os.Stderr, "xiaohongshu: waiting for scan…")
			case 1:
				fmt.Fprintln(os.Stderr, "xiaohongshu: scanned — confirm on phone")
			case 2:
				fmt.Fprintln(os.Stderr, "xiaohongshu: confirmed")
			case 3:
				fmt.Fprintln(os.Stderr, "xiaohongshu: qr expired")
			default:
				fmt.Fprintf(os.Stderr, "xiaohongshu: code_status=%d raw=%s\n", st, truncate(string(body), 200))
			}
		} else if st == 0 && n%15 == 0 {
			fmt.Fprintf(os.Stderr, "xiaohongshu: still waiting (poll #%d)…\n", n)
		}

		switch st {
		case 0, 1:
			sleep(ctx, time.Second)
			continue
		case 3:
			return nil, ErrQRExpired
		case 2:
			return c.finishQRLogin(body, loginSession, loginUID)
		default:
			sleep(ctx, time.Second)
		}
	}
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// parseStatus reads code_status + login_info from flexible JSON.
func parseStatus(body []byte) (codeStatus, apiCode int, session, userID string, ok bool) {
	var top map[string]any
	if err := json.Unmarshal(body, &top); err != nil {
		return 0, -1, "", "", false
	}
	apiCode = anyInt(top["code"])
	data, _ := top["data"].(map[string]any)
	if data == nil {
		return 0, apiCode, "", "", false
	}
	codeStatus = anyInt(data["code_status"])
	if codeStatus == 0 {
		// try alternate keys
		if v, has := data["codeStatus"]; has {
			codeStatus = anyInt(v)
		}
	}
	if li, ok2 := data["login_info"].(map[string]any); ok2 {
		if s, _ := li["session"].(string); s != "" {
			session = s
		}
		if u, _ := li["user_id"].(string); u != "" {
			userID = u
		}
	}
	return codeStatus, apiCode, session, userID, true
}

func anyInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	case string:
		var n int
		fmt.Sscanf(t, "%d", &n)
		return n
	default:
		return 0
	}
}

func (c *Client) finishQRLogin(body []byte, loginSession, loginUID string) (*SecretData, error) {
	if loginSession != "" {
		c.WebSession = loginSession
	}
	if loginUID != "" {
		c.UserID = loginUID
	}
	applyQRLoginPayload(c, body)
	if c.WebSession == "" {
		return nil, fmt.Errorf("qr ok but no web_session: %s", truncate(string(body), 240))
	}
	// Validate session with /user/me when possible (non-fatal).
	if me, err := c.doEdithGET("/api/sns/web/v2/user/me", nil); err == nil {
		var top map[string]any
		if json.Unmarshal(me, &top) == nil {
			if data, _ := top["data"].(map[string]any); data != nil {
				if uid, _ := data["user_id"].(string); uid != "" {
					c.UserID = uid
				}
				if name, _ := data["nickname"].(string); name != "" {
					fmt.Fprintf(os.Stderr, "xiaohongshu: logged in as %s\n", name)
				}
			}
		}
	}
	s := &SecretData{
		Cookie:  c.cookieHeader(),
		UserID:  c.UserID,
		LoginAt: time.Now().UTC(),
	}
	if err := saveSecret(s); err != nil {
		return nil, err
	}
	fmt.Fprintln(os.Stderr, "xiaohongshu: web QR login ok")
	return s, nil
}

func applyQRLoginPayload(c *Client, body []byte) {
	var raw map[string]any
	if json.Unmarshal(body, &raw) != nil {
		return
	}
	data, _ := raw["data"].(map[string]any)
	if data == nil {
		return
	}
	if s, ok := data["session"].(string); ok && s != "" {
		c.WebSession = s
	}
	if li, ok := data["login_info"].(map[string]any); ok {
		if s, ok := li["session"].(string); ok && s != "" {
			c.WebSession = s
		}
		if u, ok := li["user_id"].(string); ok && u != "" {
			c.UserID = u
		}
		// some builds put web_session here
		if s, ok := li["web_session"].(string); ok && s != "" {
			c.WebSession = s
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
