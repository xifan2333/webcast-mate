package bilibili

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/xifan2333/webcast-mate/internal/platform"
	"github.com/xifan2333/webcast-mate/internal/secrets"
	"github.com/xifan2333/webcast-mate/internal/termimg"
)

// EnsureLogin loads secrets or runs QR login (stderr progress).
func (c *Client) EnsureLogin(ctx context.Context) (*secrets.File, error) {
	if s, err := secrets.Load(platform.Bilibili); err == nil {
		if err := c.ApplySecrets(s); err != nil {
			return nil, err
		}
		if ok, uid, name := c.checkNav(ctx); ok {
			s.UserID = uid
			s.UserName = name
			return s, nil
		}
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

func (c *Client) loginQR(ctx context.Context) (*secrets.File, error) {
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

	close := printQR(gen.Data.URL)
	defer close()

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
		switch poll.Data.Code {
		case 0:
			ck := c.cookieString()
			if ck == "" || c.csrf() == "" {
				return nil, fmt.Errorf("login ok but missing SESSDATA/bili_jct")
			}
			ok, uid, name := c.checkNav(ctx)
			if !ok {
				return nil, fmt.Errorf("login ok but nav not logged in")
			}
			s := c.ExportSecrets(uid, name, time.Now().UTC())
			if err := secrets.Save(platform.Bilibili, s); err != nil {
				return nil, err
			}
			return s, nil
		case 86101:
		case 86090:
		case 86038:
			return nil, ErrQRExpired
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func printQR(content string) func() {
	return termimg.ShowQR(nil, content)
}
