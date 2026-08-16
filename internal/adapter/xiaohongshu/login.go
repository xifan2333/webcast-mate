package xiaohongshu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"github.com/xifan2333/webcast-mate/internal/secrets"
	"github.com/xifan2333/webcast-mate/internal/termimg"
)

// EnsureLogin loads secrets or runs CAS QR login (same shape as bilibili/douyin).
func (c *Client) EnsureLogin(ctx context.Context) (*secrets.File, error) {
	if s, err := secrets.Load("xiaohongshu"); err == nil && s.Cookie != "" {
		c.LoadSecrets(s)
		if ok, _ := c.CheckLogin(); ok {
			return s, nil
		}
		fmt.Fprintln(os.Stderr, "xiaohongshu: saved session invalid, re-login")
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
	qid := anyString(data["id"])
	qrURL := anyString(data["url"])
	if qid == "" {
		return nil, fmt.Errorf("qr create missing id: %v", created)
	}

	fmt.Fprintln(os.Stderr, "xiaohongshu: scan QR with 小红书 App (live-helper CAS)")
	if qrURL != "" {
		fmt.Fprintln(os.Stderr, qrURL)
		printQR(qrURL)
	}

	deadline := time.Now().Add(3 * time.Minute)
	last := -999
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
			fmt.Fprintf(os.Stderr, "xiaohongshu: poll err: %v\n", err)
			sleep(ctx, time.Second)
			continue
		}
		d, _ := st["data"].(map[string]any)
		if d == nil {
			sleep(ctx, time.Second)
			continue
		}
		status := anyInt(d["status"])
		if status != last {
			last = status
			switch status {
			case 2:
				fmt.Fprintln(os.Stderr, "xiaohongshu: waiting / scanned…")
			case 3:
				fmt.Fprintln(os.Stderr, "xiaohongshu: confirming…")
			case 1:
				fmt.Fprintln(os.Stderr, "xiaohongshu: confirmed")
			default:
				fmt.Fprintf(os.Stderr, "xiaohongshu: status=%d\n", status)
			}
		}
		if t := anyString(d["ticket"]); t != "" {
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
	fmt.Fprintln(os.Stderr, "xiaohongshu: ticket ok, exchanging AT…")

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
	at := anyString(ld["access_token"])
	if at == "" {
		return nil, fmt.Errorf("login no access_token: %v", lr)
	}
	c.AccessToken = at
	c.UserID = anyString(ld["user_id"])
	c.UserName = anyString(ld["nickname"])

	s := c.SecretsFile()
	if err := secrets.Save("xiaohongshu", s); err != nil {
		return nil, err
	}
	if c.UserName != "" {
		fmt.Fprintf(os.Stderr, "xiaohongshu: logged in as %s\n", c.UserName)
	}
	fmt.Fprintln(os.Stderr, "xiaohongshu: CAS login ok")
	return s, nil
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func printQR(content string) {
	if content == "" {
		return
	}
	q, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xiaohongshu: qr: %v\n", err)
		return
	}
	if png, err := q.PNG(320); err == nil {
		dir := filepath.Join(os.TempDir(), "webcast-mate")
		_ = os.MkdirAll(dir, 0o755)
		path := filepath.Join(dir, "xhs-cas-qr.png")
		if err := os.WriteFile(path, png, 0o600); err == nil {
			fmt.Fprintf(os.Stderr, "xiaohongshu: QR image %s\n", path)
			for _, bin := range []string{"xdg-open", "imv", "imv-wayland", "feh"} {
				if p, e := exec.LookPath(bin); e == nil {
					cmd := exec.Command(p, path)
					_ = cmd.Start()
					break
				}
			}
		}
		if termimg.SupportsKitty() && termimg.WriteKittyPNG(os.Stderr, png) == nil {
			return
		}
	}
	fmt.Fprint(os.Stderr, q.ToSmallString(false))
}
