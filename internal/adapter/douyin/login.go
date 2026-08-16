package douyin

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"time"

	"github.com/xifan2333/webcast-mate/internal/conv"
	"github.com/xifan2333/webcast-mate/internal/platform"
	"github.com/xifan2333/webcast-mate/internal/secrets"
	"github.com/xifan2333/webcast-mate/internal/termimg"
)

// EnsureLogin loads secrets or runs streamingtool QR login.
// Order: restore secrets → EnsureDevice (reuse or device_register) →
// session check; never overwrite a persisted did with a new random one.
func (c *Client) EnsureLogin(ctx context.Context) (*secrets.File, error) {
	var sec *secrets.File
	if s, err := secrets.Load(platform.Douyin); err == nil && s.HasAuth() {
		c.ApplySecrets(s)
		sec = s
	}
	if err := c.EnsureDevice(ctx); err != nil {
		return nil, err
	}
	// EnsureDevice may have written/updated did/iid; reload so session save keeps them.
	if s, err := secrets.Load(platform.Douyin); err == nil {
		// keep cookies already applied on client; merge params onto sec snapshot
		if sec == nil {
			sec = s
		} else if s.Params != nil {
			if sec.Params == nil {
				sec.Params = map[string]string{}
			}
			for k, v := range s.Params {
				sec.Params[k] = v
			}
		}
	}
	if sec != nil && sec.HasAuth() {
		if ok, uid, name := c.checkSession(); ok {
			sec.UserID = uid
			sec.UserName = name
			if sec.Params == nil {
				sec.Params = map[string]string{}
			}
			if c.DeviceID != "" {
				sec.Params["did"] = c.DeviceID
				sec.Params["iid"] = c.IID
			}
			_ = secrets.Save(platform.Douyin, sec)
			return sec, nil
		}
	}
	return c.loginQR(ctx)
}

func (c *Client) checkSession() (ok bool, uid, name string) {
	// user/me on webcast — short query + pure a_bogus
	qs, err := withABogus(c.pingQuery().Encode(), "")
	if err != nil {
		return false, "", ""
	}
	m, err := c.getJSON(hostAPI+"/webcast/user/me/?"+qs, nil)
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
	uid = conv.AnyString(data["id_str"])
	if uid == "" {
		uid = conv.AnyString(data["id"])
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
	q := c.passportQuery()
	qs, err := withABogus(q.Encode(), "")
	if err != nil {
		return false, "", ""
	}
	m, err := c.getJSON(hostStreaming+"/passport/account/info/v2/?"+qs, nil)
	if err != nil {
		return false, "", ""
	}
	data := mapData(m)
	if data == nil {
		return cookieHasSession(c.cookieHeader()), "", ""
	}
	uid := conv.AnyString(data["user_id"])
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

// loginQR follows webcast_mate 38514 QR state machine (not login.py):
//
//	poll interval G: 1s → 3s after 60 checks → 5s after 180 checks
//	status new/scanned → keep polling
//	confirmed → success (session from Set-Cookie on that response)
//	refused/expired → refresh QR from response up to 5 times, else fail
//	catch: error_code 6 / ECONNABORTED ≤3; bare network ≤3; else stop
//	error_code 7 is NOT retried (companion fail branch) — do not spam
func (c *Client) loginQR(ctx context.Context) (*secrets.File, error) {
	if err := c.ensureTTWid(); err != nil {
		return nil, fmt.Errorf("ttwid: %w", err)
	}

	token, png, content, err := c.getQRCode()
	if err != nil {
		return nil, err
	}
	closeQR := termimg.ShowQR(png, content)
	defer func() { closeQR() }()

	var (
		pollN       int // R.current — check attempts
		intervalS   = 1 // G.current seconds
		refreshN    int // H.current — expired/refused auto-refresh
		net6N       int // q.current — error_code 6 / abort
		netBareN    int // V.current — error with empty code
		maxRefresh  = 5
		maxNetRetry = 3
	)

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// companion has no hard wall-clock deadline; bound runaway polls (~180*5s)
		if pollN > 240 {
			return nil, ErrQRTimeout
		}

		pollN++
		if pollN >= 180 {
			intervalS = 5
		} else if pollN >= 60 && intervalS < 5 {
			intervalS = 3
		}

		res, err := c.checkQR(token)
		if err != nil {
			// transport-level: count as bare network retry (companion V.current)
			netBareN++
			if netBareN > maxNetRetry {
				return nil, err
			}
			sleep(ctx, time.Duration(intervalS)*time.Second)
			continue
		}

		// confirmed may leave session cookies even if status parse is odd
		if res.Status == "confirmed" || (cookieHasSession(c.cookieHeader()) && res.Status == "") {
			if cookieHasSession(c.cookieHeader()) {
				return c.finishLogin(res.Redirect)
			}
			return c.finishLogin(res.Redirect)
		}

		switch {
		case res.ErrCode == 6:
			// companion: error_code 6 or ECONNABORTED → ≤3 continues
			net6N++
			if net6N > maxNetRetry {
				return nil, fmt.Errorf("check_qrconnect error_code=6 after %d retries", maxNetRetry)
			}
		case res.ErrCode != 0:
			// companion: any other error_code (incl. 7) → fail, do not backoff-spam
			if res.ErrCode == 7 {
				return nil, fmt.Errorf("%w: %s", ErrQRRateLimit, res.Description)
			}
			if res.Description != "" {
				return nil, fmt.Errorf("check_qrconnect error_code=%d %s", res.ErrCode, res.Description)
			}
			return nil, fmt.Errorf("check_qrconnect error_code=%d", res.ErrCode)

		case res.Status == "new", res.Status == "scanned":
			// keep polling

		case res.Status == "refused", res.Status == "expired":
			// companion: refresh QR from payload up to 5 times
			if refreshN >= maxRefresh {
				if res.Status == "refused" {
					return nil, ErrQRRefused
				}
				return nil, ErrQRExpired
			}
			refreshN++
			if res.NewToken == "" {
				// no replacement token in body — hard fail like companion after budget
				if res.Status == "refused" {
					return nil, ErrQRRefused
				}
				return nil, ErrQRExpired
			}
			token = res.NewToken
			closeQR()
			closeQR = termimg.ShowQR(res.NewPNG, res.NewContent)
			// reset soft network counters on fresh code; keep poll cadence
			net6N, netBareN = 0, 0

		default:
			if res.Status == "" && res.ErrCode == 0 {
				// empty oddity: treat like bare retry
				netBareN++
				if netBareN > maxNetRetry {
					return nil, fmt.Errorf("check_qrconnect: empty status")
				}
				break
			}
			return nil, fmt.Errorf("check_qrconnect unexpected status %q", res.Status)
		}

		sleep(ctx, time.Duration(intervalS)*time.Second)
	}
}

// qrCheck is one check_qrconnect outcome (companion Q() then-branch).
type qrCheck struct {
	Status      string
	Redirect    string
	ErrCode     int
	Description string
	// refresh payload when status is refused/expired
	NewToken   string
	NewPNG     []byte
	NewContent string
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
	s := c.ExportSecrets(uid, name, time.Now().UTC())
	if err := secrets.Save(platform.Douyin, s); err != nil {
		return nil, err
	}
	return s, nil
}

// getQRCode returns poll token, a pre-rendered PNG (if the API gave one),
// and a content URL (ASCII fallback).
func (c *Client) getQRCode() (token string, png []byte, content string, err error) {
	q := c.passportQuery()
	q.Set("next", hostStreaming)
	q.Set("need_logo", "false")
	q.Set("need_short_url", "false")
	q.Set("is_new_login", "1")
	qs, err := withABogus(q.Encode(), "")
	if err != nil {
		return "", nil, "", err
	}
	m, err := c.getJSON(hostStreaming+"/passport/web/get_qrcode/?"+qs, nil)
	if err != nil {
		return "", nil, "", err
	}
	data := mapData(m)
	if data == nil {
		return "", nil, "", fmt.Errorf("get_qrcode: %v", m)
	}
	if ec := conv.AnyString(data["error_code"]); ec != "" && ec != "0" {
		return "", nil, "", fmt.Errorf("get_qrcode error_code=%s msg=%v", ec, data["description"])
	}
	token = conv.AnyString(data["token"])
	if token == "" {
		return "", nil, "", fmt.Errorf("get_qrcode missing token")
	}
	// Prefer the server-rendered PNG (uniform size) over a long URL.
	if b64 := conv.AnyString(data["qrcode"]); b64 != "" {
		png = decodeQRBase64(b64)
	}
	for _, k := range []string{"qrcode_index_url", "qrcode_url", "url"} {
		if u := conv.AnyString(data[k]); u != "" {
			content = u
			break
		}
	}
	if len(png) == 0 && content == "" {
		return "", nil, "", fmt.Errorf("get_qrcode missing qrcode")
	}
	return token, png, content, nil
}

func (c *Client) checkQR(token string) (qrCheck, error) {
	q := c.passportQuery()
	form := url.Values{}
	form.Set("need_logo", "false")
	form.Set("need_short_url", "false")
	form.Set("is_frontier", "true")
	form.Set("token", token)
	form.Set("is_new_login", "1")
	form.Set("next", "https://www.douyin.com")
	body := form.Encode()
	qs, err := withABogus(q.Encode(), body)
	if err != nil {
		return qrCheck{}, err
	}
	m, err := c.postForm(hostStreaming+"/passport/web/check_qrconnect/?"+qs, form, nil)
	if err != nil {
		return qrCheck{}, err
	}
	out := qrCheck{}
	data := mapData(m)
	if data == nil {
		out.ErrCode = conv.AnyInt(m["error_code"])
		if out.ErrCode == 0 {
			out.ErrCode = conv.AnyInt(m["status_code"])
		}
		out.Description = conv.AnyString(m["description"])
		if out.ErrCode == 0 {
			return out, fmt.Errorf("check_qrconnect: %v", m)
		}
		return out, nil
	}
	out.ErrCode = conv.AnyInt(data["error_code"])
	out.Description = conv.AnyString(data["description"])
	if out.Description == "" {
		out.Description = conv.AnyString(data["captcha"])
	}
	// business error_code on data (incl. 7) — caller decides; no special 8s retry
	if out.ErrCode != 0 {
		return out, nil
	}
	// some failures only set message != success with nested code
	if msg, _ := m["message"].(string); msg != "" && msg != "success" {
		if ec := conv.AnyInt(m["error_code"]); ec != 0 {
			out.ErrCode = ec
			return out, nil
		}
	}

	out.Status = conv.AnyString(data["status"])
	switch out.Status {
	case "1":
		out.Status = "new"
	case "2":
		out.Status = "scanned"
	case "3":
		out.Status = "confirmed"
	case "4":
		out.Status = "refused"
	case "5":
		out.Status = "expired"
	}
	out.Redirect = conv.AnyString(data["redirect_url"])
	if out.Redirect == "" {
		out.Redirect = conv.AnyString(data["url"])
	}
	// companion refused/expired path may embed next qrcode+token
	if out.Status == "refused" || out.Status == "expired" {
		out.NewToken = conv.AnyString(data["token"])
		if b64 := conv.AnyString(data["qrcode"]); b64 != "" {
			out.NewPNG = decodeQRBase64(b64)
		}
		for _, k := range []string{"qrcode_index_url", "qrcode_url", "url"} {
			if u := conv.AnyString(data[k]); u != "" {
				out.NewContent = u
				break
			}
		}
	}
	return out, nil
}

func decodeQRBase64(b64 string) []byte {
	if i := indexOf(b64, ","); i >= 0 && hasPrefix(b64, "data:") {
		b64 = b64[i+1:]
	}
	if p, err := base64.StdEncoding.DecodeString(b64); err == nil {
		return p
	}
	if p, err := base64.RawStdEncoding.DecodeString(b64); err == nil {
		return p
	}
	return nil
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
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
