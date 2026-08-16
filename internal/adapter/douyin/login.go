package douyin

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"github.com/xifan2333/webcast-mate/internal/secrets"
	"github.com/xifan2333/webcast-mate/internal/termimg"
)

// EnsureLogin loads secrets or runs streamingtool QR login.
func (c *Client) EnsureLogin(ctx context.Context) (*secrets.File, error) {
	if s, err := secrets.Load("douyin"); err == nil && s.Cookie != "" {
		c.setCookieHeader(s.Cookie)
		if ok, uid, name := c.checkSession(); ok {
			s.UserID = uid
			s.UserName = name
			return s, nil
		}
		fmt.Fprintln(os.Stderr, "douyin: saved session invalid, re-login")
	}
	return c.loginQR(ctx)
}

func (c *Client) checkSession() (ok bool, uid, name string) {
	// user/me on webcast
	q := c.commonQuery()
	m, err := c.getJSON(hostAPI+"/webcast/user/me/?"+q.Encode(), nil)
	if err != nil {
		return false, "", ""
	}
	// status_code 0 + data
	sc, _ := m["status_code"].(float64)
	if sc != 0 {
		// try account info
		return c.checkAccountInfo()
	}
	data := mapData(m)
	if data == nil {
		return c.checkAccountInfo()
	}
	uid = anyString(data["id_str"])
	if uid == "" {
		uid = anyString(data["id"])
	}
	if n, ok := data["nickname"].(string); ok {
		name = n
	}
	// weak: presence of session cookie
	if c.cookieHeader() == "" {
		return false, "", ""
	}
	if uid == "" && !cookieHasSession(c.cookieHeader()) {
		return false, "", ""
	}
	if cookieHasSession(c.cookieHeader()) {
		return true, uid, name
	}
	return false, "", ""
}

func (c *Client) checkAccountInfo() (bool, string, string) {
	q := c.sdkParams()
	m, err := c.getJSON(hostStreaming+"/passport/account/info/v2/?"+q.Encode(), nil)
	if err != nil {
		return false, "", ""
	}
	data := mapData(m)
	if data == nil {
		return cookieHasSession(c.cookieHeader()), "", ""
	}
	uid := anyString(data["user_id"])
	name, _ := data["screen_name"].(string)
	if name == "" {
		name, _ = data["username"].(string)
	}
	return cookieHasSession(c.cookieHeader()), uid, name
}

func cookieHasSession(h string) bool {
	return containsCookie(h, "sessionid") || containsCookie(h, "sid_guard") || containsCookie(h, "sessionid_ss")
}

func containsCookie(h, name string) bool {
	for _, p := range splitCookie(h) {
		k, _, ok := cutKV(p)
		if ok && k == name {
			return true
		}
	}
	return false
}

func splitCookie(h string) []string {
	var out []string
	start := 0
	for i := 0; i < len(h); i++ {
		if h[i] == ';' {
			out = append(out, trimSpace(h[start:i]))
			start = i + 1
		}
	}
	out = append(out, trimSpace(h[start:]))
	return out
}

func cutKV(p string) (string, string, bool) {
	for i := 0; i < len(p); i++ {
		if p[i] == '=' {
			return trimSpace(p[:i]), trimSpace(p[i+1:]), true
		}
	}
	return "", "", false
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func (c *Client) ensureTTWid() error {
	payload := map[string]any{
		"aid":     2079,
		"service": "streamingtool.douyin.com",
		"host":    hostStreaming,
		"union":   false,
		"needFid": false,
	}
	m, err := c.postJSON(hostStreaming+"/ttwid/check/", payload)
	if err != nil {
		return err
	}
	if containsCookie(c.cookieHeader(), "ttwid") {
		return nil
	}
	// register
	payload["migrate_info"] = m["migrate_info"]
	if _, err := c.postJSON(hostStreaming+"/ttwid/register/", payload); err != nil {
		return err
	}
	if !containsCookie(c.cookieHeader(), "ttwid") {
		return fmt.Errorf("ttwid missing after register")
	}
	return nil
}

func (c *Client) loginQR(ctx context.Context) (*secrets.File, error) {
	if err := c.ensureTTWid(); err != nil {
		return nil, fmt.Errorf("ttwid: %w", err)
	}

	token, qrURL, err := c.getQRCode()
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(os.Stderr, "douyin: scan QR with Douyin app")
	if qrURL != "" {
		fmt.Fprintln(os.Stderr, qrURL)
	}
	printQR(qrURL)

	// Align with ~/douyin-live/login.py poll_until_login:
	// error 7 = 访问太频繁 → wait 8s, do NOT re-issue QR
	// 2156/1105 = 系统繁忙 → wait 3s
	deadline := time.Now().Add(4 * time.Minute)
	last := ""
	scanned := false
	interval := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, ErrQRTimeout
		}
		st, redirect, errCode, err := c.checkQR(token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "douyin: poll err: %v\n", err)
			sleep(ctx, interval)
			continue
		}
		// Session may already be set while API still returns busy after confirm.
		if cookieHasSession(c.cookieHeader()) && (scanned || errCode == 7 || errCode == 2156 || st == "confirmed") {
			return c.finishLogin("")
		}
		// retryable — same waits as login.py (7 → 8s, others → 3s)
		if errCode == 7 || errCode == 2156 || errCode == 1105 {
			wait := 3 * time.Second
			if errCode == 7 {
				wait = 8 * time.Second
			}
			if last != fmt.Sprintf("busy-%d", errCode) {
				fmt.Fprintf(os.Stderr, "douyin: %d rate-limit/busy — wait %ds, keep same QR (do not rescan)\n",
					errCode, int(wait.Seconds()))
				last = fmt.Sprintf("busy-%d", errCode)
			}
			// stretch deadline while backing off after scan/confirm
			if scanned {
				deadline = deadline.Add(wait)
			}
			sleep(ctx, wait)
			continue
		}
		if st != last {
			last = st
			switch st {
			case "new":
				fmt.Fprintln(os.Stderr, "douyin: waiting for scan…")
			case "scanned":
				scanned = true
				interval = 2 * time.Second
				fmt.Fprintln(os.Stderr, "douyin: scanned — confirm on phone")
			case "confirmed":
				fmt.Fprintln(os.Stderr, "douyin: confirmed")
			case "refused":
				return nil, ErrQRRefused
			case "expired":
				return nil, ErrQRExpired
			default:
				if st != "" {
					fmt.Fprintf(os.Stderr, "douyin: status=%s\n", st)
				}
			}
		}
		if st == "confirmed" {
			if redirect != "" {
				_, _ = c.do("GET", redirect, nil, "", nil)
			}
			sleep(ctx, 500*time.Millisecond)
			return c.finishLogin(redirect)
		}
		if st == "expired" {
			return nil, ErrQRExpired
		}
		if st == "refused" {
			return nil, ErrQRRefused
		}
		if scanned {
			interval = 2 * time.Second
		}
		sleep(ctx, interval)
	}
}

func (c *Client) finishLogin(redirect string) (*secrets.File, error) {
	if redirect != "" {
		_, _ = c.do("GET", redirect, nil, "", nil)
	}
	if !cookieHasSession(c.cookieHeader()) {
		// one more account probe may set nothing; still fail clearly
		return nil, fmt.Errorf("login settled but no session cookie yet")
	}
	_, uid, name := c.checkSession()
	s := &secrets.File{
		Cookie:   c.cookieHeader(),
		UserID:   uid,
		UserName: name,
		LoginAt:  time.Now().UTC(),
	}
	if err := secrets.Save("douyin", s); err != nil {
		return nil, err
	}
	if name != "" {
		fmt.Fprintf(os.Stderr, "douyin: logged in as %s\n", name)
	}
	fmt.Fprintln(os.Stderr, "douyin: QR login ok")
	return s, nil
}

// getQRCode returns poll token and a URL suitable for terminal QR (same as bili/xhs).
func (c *Client) getQRCode() (token, qrURL string, err error) {
	q := c.sdkParams()
	q.Set("next", hostStreaming)
	q.Set("need_logo", "false")
	q.Set("need_short_url", "false")
	q.Set("is_new_login", "1")
	m, err := c.getJSON(hostStreaming+"/passport/web/get_qrcode/?"+q.Encode(), nil)
	if err != nil {
		return "", "", err
	}
	data := mapData(m)
	if data == nil {
		return "", "", fmt.Errorf("get_qrcode: %v", m)
	}
	if ec := anyString(data["error_code"]); ec != "" && ec != "0" {
		return "", "", fmt.Errorf("get_qrcode error_code=%s msg=%v", ec, data["description"])
	}
	token = anyString(data["token"])
	if token == "" {
		return "", "", fmt.Errorf("get_qrcode missing token")
	}
	// Prefer scan URL for go-qrcode (same display path as bilibili/xhs).
	for _, k := range []string{"qrcode_index_url", "qrcode_url", "url"} {
		if u := anyString(data[k]); u != "" {
			qrURL = u
			break
		}
	}
	if qrURL == "" {
		// API often only returns PNG base64; decode and show via Kitty, else write file.
		b64 := anyString(data["qrcode"])
		if b64 == "" {
			return "", "", fmt.Errorf("get_qrcode missing qrcode")
		}
		if i := indexOf(b64, ","); i >= 0 && hasPrefix(b64, "data:") {
			b64 = b64[i+1:]
		}
		png, decErr := base64.StdEncoding.DecodeString(b64)
		if decErr != nil {
			png, decErr = base64.RawStdEncoding.DecodeString(b64)
		}
		if decErr != nil {
			return token, "", fmt.Errorf("qrcode b64: %w", decErr)
		}
		printQRPNG(png)
		return token, "", nil
	}
	return token, qrURL, nil
}

func (c *Client) checkQR(token string) (status, redirect string, errCode int, err error) {
	q := c.sdkParams()
	form := url.Values{}
	form.Set("token", token)
	form.Set("next", hostStreaming)
	form.Set("need_logo", "false")
	form.Set("need_short_url", "false")
	form.Set("is_frontier", "false")
	form.Set("is_new_login", "1")
	form.Set("aid", aid)
	m, err := c.postForm(hostStreaming+"/passport/web/check_qrconnect/?"+q.Encode(), form, nil)
	if err != nil {
		return "", "", 0, err
	}
	data := mapData(m)
	if data == nil {
		// top-level error_code sometimes
		errCode = anyInt(m["error_code"])
		if errCode == 0 {
			errCode = anyInt(m["status_code"])
		}
		if errCode == 7 || errCode == 2156 || errCode == 1105 {
			return "", "", errCode, nil
		}
		return "", "", 0, fmt.Errorf("check_qrconnect: %v", m)
	}
	errCode = anyInt(data["error_code"])
	msg, _ := m["message"].(string)
	// login.py: message != success + retryable error_code → backoff
	if msg != "" && msg != "success" && (errCode == 7 || errCode == 2156 || errCode == 1105) {
		return "", "", errCode, nil
	}
	if errCode == 7 || errCode == 2156 || errCode == 1105 {
		return "", "", errCode, nil
	}
	if errCode != 0 {
		// non-retryable business error
		desc := anyString(data["description"])
		if desc == "" {
			desc = anyString(data["captcha"])
		}
		return "", "", errCode, fmt.Errorf("check_qrconnect error_code=%d %s", errCode, desc)
	}
	status = anyString(data["status"])
	switch status {
	case "1":
		status = "new"
	case "2":
		status = "scanned"
	case "3":
		status = "confirmed"
	case "4":
		status = "refused"
	case "5":
		status = "expired"
	}
	redirect = anyString(data["redirect_url"])
	if redirect == "" {
		redirect = anyString(data["url"])
	}
	return status, redirect, 0, nil
}

func anyInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case string:
		var n int
		fmt.Sscanf(t, "%d", &n)
		return n
	default:
		return 0
	}
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// printQR matches bilibili/xiaohongshu: Kitty graphics PNG, else ToSmallString.
func printQR(content string) {
	if content == "" {
		return
	}
	q, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		fmt.Fprintf(os.Stderr, "douyin: qr: %v\n", err)
		return
	}
	if termimg.SupportsKitty() {
		png, err := q.PNG(280)
		if err == nil && termimg.WriteKittyPNG(os.Stderr, png) == nil {
			return
		}
	}
	fmt.Fprint(os.Stderr, q.ToSmallString(false))
}

// printQRPNG when API only returns a PNG (no index URL).
func printQRPNG(png []byte) {
	if len(png) == 0 {
		return
	}
	if termimg.SupportsKitty() && termimg.WriteKittyPNG(os.Stderr, png) == nil {
		return
	}
	_ = os.MkdirAll(filepath.Join(os.TempDir(), "webcast-mate"), 0o755)
	path := filepath.Join(os.TempDir(), "webcast-mate", "douyin-qr.png")
	_ = os.WriteFile(path, png, 0o600)
	fmt.Fprintf(os.Stderr, "douyin: QR image %s\n", path)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func hasPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}
