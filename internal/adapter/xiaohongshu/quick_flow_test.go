package xiaohongshu

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestQuickFlowNoScan(t *testing.T) {
	cli := NewClient()
	if err := cli.EnsureWebGuest(); err != nil {
		t.Fatalf("guest: %v", err)
	}
	t.Logf("guest ok session_len=%d uid=%s", len(cli.WebSession), cli.UserID)

	b, err := cli.doEdith(http.MethodPost, "/api/sns/web/v1/login/qrcode/create", json.RawMessage(`{"qr_type":1}`))
	if err != nil {
		t.Fatalf("create: %v body=%s", err, b)
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
	if err := json.Unmarshal(b, &created); err != nil || !created.Success || created.Code != 0 {
		t.Fatalf("create body: %s", b)
	}
	t.Logf("QR create ok id=%s code=%s", created.Data.QRID, created.Data.Code)

	q := url.Values{}
	q.Set("code", created.Data.Code)
	q.Set("qr_id", created.Data.QRID)
	sb, err := cli.doEdithGET("/api/sns/web/v1/login/qrcode/status", q)
	if err != nil {
		t.Fatalf("status: %v %s", err, sb)
	}
	st, api, _, _, ok := parseStatus(sb)
	if !ok || api != 0 {
		t.Fatalf("status parse: %s", sb)
	}
	t.Logf("status poll OK code_status=%d", st)

	time.Sleep(800 * time.Millisecond)
	sb2, err := cli.doEdithGET("/api/sns/web/v1/login/qrcode/status", q)
	if err != nil {
		t.Fatalf("status2: %v", err)
	}
	st2, api2, _, _, ok2 := parseStatus(sb2)
	if !ok2 || api2 != 0 {
		t.Fatalf("status2: %s", sb2)
	}
	t.Logf("status poll2 OK code_status=%d", st2)

	q2 := url.Values{}
	q2.Set("code", "000000")
	b2, err := cli.DebugGET("/web_api/sns/v1/live/obs/push_url", q2)
	t.Logf("push_url guest+dummy: err=%v body=%s", err, trunc(string(b2), 160))
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
