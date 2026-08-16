package bilibili

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"github.com/xifan2333/webcast-mate/internal/session"
	"github.com/xifan2333/webcast-mate/internal/termimg"
)

type Session struct {
	Cookie    string    `json:"cookie"`
	LoginAt   time.Time `json:"login_at"`
	UserID    string    `json:"user_id,omitempty"`
	UserName  string    `json:"user_name,omitempty"`
}

func sessionPath() (string, error) {
	d, err := session.PlatformDir("bilibili")
	if err != nil {
		return "", err
	}
	return d + "/session.json", nil
}

func LoadSession() (*Session, error) {
	path, err := sessionPath()
	if err != nil {
		return nil, err
	}
	var s Session
	if err := session.ReadJSON(path, &s); err != nil {
		return nil, err
	}
	if s.Cookie == "" {
		return nil, ErrNotLoggedIn
	}
	return &s, nil
}

func SaveSession(s *Session) error {
	path, err := sessionPath()
	if err != nil {
		return err
	}
	return session.WriteJSON(path, s)
}

type qrGenResp struct {
	Code int `json:"code"`
	Data struct {
		URL       string `json:"url"`
		QRCodeKey string `json:"qrcode_key"`
	} `json:"data"`
	Message string `json:"message"`
}

type qrPollResp struct {
	Code int `json:"code"`
	Data struct {
		Code         int    `json:"code"`
		Message      string `json:"message"`
		RefreshToken string `json:"refresh_token"`
		URL          string `json:"url"`
	} `json:"data"`
	Message string `json:"message"`
}

type navResp struct {
	Code int `json:"code"`
	Data struct {
		IsLogin bool   `json:"isLogin"`
		Uname   string `json:"uname"`
		Mid     int64  `json:"mid"`
	} `json:"data"`
}

// EnsureLogin loads session or runs QR login (stderr progress).
func (c *Client) EnsureLogin(ctx context.Context) (*Session, error) {
	if s, err := LoadSession(); err == nil {
		if err := c.setCookieHeader(s.Cookie); err != nil {
			return nil, err
		}
		if ok, uid, name := c.checkNav(ctx); ok {
			s.UserID = uid
			s.UserName = name
			return s, nil
		}
		fmt.Fprintln(os.Stderr, "bilibili: saved session invalid, re-login")
	}
	return c.loginQR(ctx)
}

func (c *Client) checkNav(ctx context.Context) (ok bool, uid, name string) {
	_ = ctx
	b, err := c.doJSON("GET", urlNav, nil, nil)
	if err != nil {
		return false, "", ""
	}
	var nav navResp
	if json.Unmarshal(b, &nav) != nil || nav.Code != 0 || !nav.Data.IsLogin {
		return false, "", ""
	}
	return true, fmt.Sprintf("%d", nav.Data.Mid), nav.Data.Uname
}

func (c *Client) loginQR(ctx context.Context) (*Session, error) {
	b, err := c.doJSON("GET", urlQRGenerate, nil, map[string]string{
		"Referer": "https://passport.bilibili.com/login",
		"Origin":  "https://passport.bilibili.com",
	})
	if err != nil {
		return nil, err
	}
	var gen qrGenResp
	if err := json.Unmarshal(b, &gen); err != nil {
		return nil, err
	}
	if gen.Code != 0 || gen.Data.QRCodeKey == "" {
		return nil, fmt.Errorf("qr generate: code=%d %s", gen.Code, gen.Message)
	}

	fmt.Fprintln(os.Stderr, "bilibili: scan QR with bilibili app")
	fmt.Fprintln(os.Stderr, gen.Data.URL)
	printQR(gen.Data.URL, "login-qr.png")

	deadline := time.Now().Add(3 * time.Minute)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, ErrQRTimeout
		}
		pollURL := urlQRPoll + "?qrcode_key=" + url.QueryEscape(gen.Data.QRCodeKey)
		pb, err := c.doJSON("GET", pollURL, nil, map[string]string{
			"Referer": "https://passport.bilibili.com/login",
			"Origin":  "https://passport.bilibili.com",
		})
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		var poll qrPollResp
		if json.Unmarshal(pb, &poll) != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		// outer code should be 0; inner data.code is status
		switch poll.Data.Code {
		case 0:
			// success — cookies in jar
			ck := c.cookieString()
			if ck == "" || c.csrf() == "" {
				return nil, fmt.Errorf("login ok but missing SESSDATA/bili_jct")
			}
			ok, uid, name := c.checkNav(ctx)
			if !ok {
				return nil, fmt.Errorf("login ok but nav not logged in")
			}
			s := &Session{
				Cookie:   ck,
				LoginAt:  time.Now().UTC(),
				UserID:   uid,
				UserName: name,
			}
			if err := SaveSession(s); err != nil {
				return nil, err
			}
			fmt.Fprintf(os.Stderr, "bilibili: logged in as %s (%s)\n", name, uid)
			return s, nil
		case 86101:
			// waiting scan
		case 86090:
			fmt.Fprintln(os.Stderr, "bilibili: scanned, confirm on phone")
		case 86038:
			return nil, ErrQRExpired
		default:
			// keep polling unknown
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// printQR shows a scannable QR on stderr.
// Prefer Kitty graphics protocol (true PNG pixels) when supported; else character art.
func printQR(content, filename string) {
	_ = filename
	if content == "" {
		return
	}
	q, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bilibili: qr: %v\n", err)
		return
	}
	// quiet zone helps phone cameras; keep library default border
	if termimg.SupportsKitty() {
		png, err := q.PNG(280)
		if err == nil && termimg.WriteKittyPNG(os.Stderr, png) == nil {
			return
		}
	}
	fmt.Fprint(os.Stderr, q.ToSmallString(false))
}
