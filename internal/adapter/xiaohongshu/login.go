package xiaohongshu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xifan2333/webcast-mate/internal/conv"
	"github.com/xifan2333/webcast-mate/internal/platform"
	"github.com/xifan2333/webcast-mate/internal/secrets"
	"github.com/xifan2333/webcast-mate/internal/termimg"
)

// EnsureLogin loads secrets or runs CAS QR login (same shape as bilibili/douyin).
func (c *Client) EnsureLogin(ctx context.Context) (*secrets.File, error) {
	if s, err := secrets.Load(platform.XiaoHongShu); err == nil && s.HasAuth() {
		c.ApplySecrets(s)
		if ok, _ := c.CheckLogin(); ok {
			return s, nil
		}
	}
	return c.loginQR(ctx)
}

// CheckLogin GET robs/api/sns/check_login
func (c *Client) CheckLogin() (bool, map[string]any) {
	m, err := c.do(http.MethodGet, hostRobs, "/api/sns/check_login", nil, nil, doOpts{})
	if err != nil {
		return false, nil
	}
	return bizOK(m), m
}

func (c *Client) loginQR(ctx context.Context) (*secrets.File, error) {
	c.ensureIdentity()

	_, _ = c.do(http.MethodGet, hostCustomer, "/api/cas/customer/pc/zones", nil,
		url.Values{"service": {serviceRobs}}, doOpts{sign: true, originRobs: true})

	body := map[string]any{"service": serviceRobs, "subsystem": "robs"}
	created, err := c.do(http.MethodPost, hostCustomer, "/api/cas/customer/pc/qr-code", body, nil,
		doOpts{sign: true, originRobs: true})
	if err != nil {
		return nil, fmt.Errorf("qr create: %w", err)
	}
	if !bizOK(created) {
		return nil, fmt.Errorf("qr create: %v", created)
	}
	data, _ := created["data"].(map[string]any)
	if data == nil {
		return nil, fmt.Errorf("qr create no data: %v", created)
	}
	qid := conv.AnyString(data["id"])
	qrURL := conv.AnyString(data["url"])
	if qid == "" {
		return nil, fmt.Errorf("qr create missing id: %v", created)
	}

	if qrURL != "" {
		close := printQR(qrURL)
		defer close()
	}

	deadline := time.Now().Add(3 * time.Minute)
	var ticket string
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, ErrQRTimeout
		}
		q := url.Values{}
		q.Set("qr_code_id", qid)
		q.Set("service", serviceRobs)
		st, err := c.do(http.MethodGet, hostCustomer, "/api/cas/customer/pc/qr-code", nil, q,
			doOpts{sign: true, originRobs: true})
		if err != nil {
			sleep(ctx, time.Second)
			continue
		}
		d, _ := st["data"].(map[string]any)
		if d == nil {
			sleep(ctx, time.Second)
			continue
		}
		if t := conv.AnyString(d["ticket"]); t != "" {
			ticket = t
			break
		}
		raw, _ := json.Marshal(st)
		if i := strings.Index(string(raw), "ST-"); i >= 0 {
			end := i + 2
			for end < len(raw) {
				ch := raw[end]
				if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' {
					end++
					continue
				}
				break
			}
			ticket = string(raw[i:end])
			if len(ticket) > 10 {
				break
			}
			ticket = ""
		}
		sleep(ctx, time.Second)
	}
	if ticket == "" {
		return nil, ErrQRTimeout
	}

	loginBody := map[string]any{"ticket": ticket, "service": serviceRobs}
	lr, err := c.do(http.MethodPost, hostRobs, "/api/sns/login", loginBody, nil,
		doOpts{sign: true, originRobs: true})
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	if !bizOK(lr) {
		return nil, fmt.Errorf("login: %v", lr)
	}
	ld, _ := lr["data"].(map[string]any)
	if ld == nil {
		return nil, fmt.Errorf("login no data: %v", lr)
	}
	at := conv.AnyString(ld["access_token"])
	if at == "" {
		return nil, fmt.Errorf("login no access_token: %v", lr)
	}
	c.AccessToken = at
	c.UserID = conv.AnyString(ld["user_id"])
	c.UserName = conv.AnyString(ld["nickname"])

	s := c.ExportSecrets()
	if err := secrets.Save(platform.XiaoHongShu, s); err != nil {
		return nil, err
	}
	return s, nil
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func printQR(content string) func() {
	return termimg.ShowQR(nil, content)
}
