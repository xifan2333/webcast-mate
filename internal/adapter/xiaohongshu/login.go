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

// SecretData is stored as JSON inside secrets/xiaohongshu.json Cookie field.
type SecretData struct {
	Sid      string    `json:"sid,omitempty"`    // robs access_token for live
	Cookie   string    `json:"cookie,omitempty"` // web a1;webId;web_session
	UserID   string    `json:"user_id,omitempty"`
	UserName string    `json:"user_name,omitempty"`
	DeviceID string    `json:"device_id,omitempty"`
	Phone    string    `json:"phone,omitempty"`
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
	if s.DeviceID != "" {
		c.DeviceID = s.DeviceID
	}
	c.Sid = s.Sid
	c.UserID = s.UserID
	if s.Cookie == "" {
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
}

// EnsureLogin reuses secrets, else QR (web), else SMS for robs sid.
func (c *Client) EnsureLogin(ctx context.Context) (*SecretData, error) {
	if s, err := loadSecret(); err == nil {
		c.applySecret(s)
		if c.Sid != "" {
			if ok, name := c.checkRobsLogin(); ok {
				s.UserName = name
				return s, nil
			}
			fmt.Fprintln(os.Stderr, "xiaohongshu: saved sid invalid")
		}
	}

	s, err := c.loginQR(ctx)
	if err == nil {
		// Web QR alone has no robs sid — try SMS if interactive for live open
		if c.Sid == "" && isInteractive() {
			fmt.Fprintln(os.Stderr, "xiaohongshu: web login ok; live open needs SMS sid")
			if s2, err2 := c.loginSMS(ctx); err2 == nil {
				return s2, nil
			} else {
				fmt.Fprintf(os.Stderr, "xiaohongshu: SMS skipped/failed: %v\n", err2)
			}
		}
		return s, nil
	}
	fmt.Fprintf(os.Stderr, "xiaohongshu: qr login failed: %v\n", err)
	return c.loginSMS(ctx)
}

func (c *Client) loginQR(ctx context.Context) (*SecretData, error) {
	if err := c.EnsureWebGuest(); err != nil {
		return nil, err
	}
	uri := "/api/sns/web/v1/login/qrcode/create"
	payload := map[string]any{"qr_type": 1}
	b, _, err := c.doEdith(http.MethodPost, uri, payload)
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
		params := map[string]any{"qr_id": created.Data.QRID, "code": created.Data.Code}
		q := url.Values{}
		q.Set("qr_id", created.Data.QRID)
		q.Set("code", created.Data.Code)
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
			Code    int  `json:"code"`
			Success bool `json:"success"`
			Data    struct {
				CodeStatus int `json:"code_status"`
				LoginInfo  struct {
					Session string `json:"session"`
					UserID  string `json:"user_id"`
				} `json:"login_info"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &st); err != nil || st.Code != 0 {
			// surface API errors so user knows poll is failing
			var raw map[string]any
			_ = json.Unmarshal(body, &raw)
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
			default:
				fmt.Fprintf(os.Stderr, "xiaohongshu: qr code_status=%d\n", st.Data.CodeStatus)
			}
		}

		switch st.Data.CodeStatus {
		case 0, 1:
			// keep polling
		case 2:
			// parse session from several possible shapes
			applyQRLoginPayload(c, body, &st.Data.LoginInfo.Session, &st.Data.LoginInfo.UserID)
			s := &SecretData{
				Cookie:   c.cookieHeader(),
				UserID:   c.UserID,
				DeviceID: c.DeviceID,
				LoginAt:  time.Now().UTC(),
			}
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

func (c *Client) loginSMS(ctx context.Context) (*SecretData, error) {
	_ = ctx
	if !isInteractive() {
		return nil, fmt.Errorf("%w: need interactive SMS or valid sid in secrets", ErrNotLoggedIn)
	}
	phone := ""
	if s, err := loadSecret(); err == nil {
		phone = s.Phone
	}
	fmt.Fprint(os.Stderr, "xiaohongshu: phone (+86)")
	if phone != "" {
		fmt.Fprintf(os.Stderr, " [%s]", phone)
	}
	fmt.Fprint(os.Stderr, ": ")
	var line string
	_, _ = fmt.Scanln(&line)
	if line != "" {
		phone = line
	}
	if phone == "" {
		return nil, fmt.Errorf("empty phone")
	}

	b, _, err := c.doRobsForm("/api/sns/send_sms", map[string]string{
		"phone_number":  phone,
		"phone_country": "86",
	})
	if err != nil {
		return nil, err
	}
	var sent struct {
		Result int    `json:"result"`
		Msg    string `json:"msg"`
	}
	_ = json.Unmarshal(b, &sent)
	if sent.Result != 0 {
		return nil, fmt.Errorf("send_sms: %s (%d)", sent.Msg, sent.Result)
	}

	fmt.Fprint(os.Stderr, "xiaohongshu: sms code: ")
	var code string
	_, _ = fmt.Scanln(&code)
	if code == "" {
		return nil, fmt.Errorf("empty sms code")
	}

	b, _, err = c.doRobsForm("/api/sns/login_by_sms", map[string]string{
		"phone_number":  phone,
		"phone_country": "86",
		"sms_code":      code,
	})
	if err != nil {
		return nil, err
	}
	var login struct {
		Result int    `json:"result"`
		Msg    string `json:"msg"`
		Data   struct {
			AccessToken string `json:"access_token"`
			Nickname    string `json:"nickname"`
			UserID      string `json:"user_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &login); err != nil {
		return nil, err
	}
	if login.Result != 0 || login.Data.AccessToken == "" {
		return nil, fmt.Errorf("login_by_sms: %s (%d)", login.Msg, login.Result)
	}
	c.Sid = login.Data.AccessToken
	c.UserID = login.Data.UserID
	s := &SecretData{
		Sid:      c.Sid,
		UserID:   c.UserID,
		UserName: login.Data.Nickname,
		DeviceID: c.DeviceID,
		Phone:    phone,
		Cookie:   c.cookieHeader(),
		LoginAt:  time.Now().UTC(),
	}
	if err := saveSecret(s); err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "xiaohongshu: logged in as %s\n", login.Data.Nickname)
	return s, nil
}

func (c *Client) checkRobsLogin() (bool, string) {
	if c.Sid == "" {
		return false, ""
	}
	b, err := c.doRobs(http.MethodGet, "/api/sns/check_login", c.Sid, nil)
	if err != nil {
		return false, ""
	}
	var out struct {
		Result int `json:"result"`
		Data   struct {
			Nickname string `json:"nickname"`
		} `json:"data"`
	}
	if json.Unmarshal(b, &out) != nil || out.Result != 0 {
		return false, ""
	}
	return true, out.Data.Nickname
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

// applyQRLoginPayload extracts session/user_id from status response variants.
func applyQRLoginPayload(c *Client, body []byte, sessionHint, userHint *string) {
	if sessionHint != nil && *sessionHint != "" {
		c.WebSession = *sessionHint
	}
	if userHint != nil && *userHint != "" {
		c.UserID = *userHint
	}
	var raw map[string]any
	if json.Unmarshal(body, &raw) != nil {
		return
	}
	data, _ := raw["data"].(map[string]any)
	if data == nil {
		return
	}
	// data.session / data.web_session
	for _, k := range []string{"session", "web_session", "secure_session"} {
		if s, ok := data[k].(string); ok && s != "" {
			c.WebSession = s
		}
	}
	if uid, ok := data["user_id"].(string); ok && uid != "" {
		c.UserID = uid
	}
	// data.login_info.{session,user_id}
	if li, ok := data["login_info"].(map[string]any); ok {
		if s, ok := li["session"].(string); ok && s != "" {
			c.WebSession = s
		}
		if uid, ok := li["user_id"].(string); ok && uid != "" {
			c.UserID = uid
		}
	}
}
