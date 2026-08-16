package douyin

// Upstream device_register (companion deviceIdManage / registry).
//
// Critical wire fields (from companion static JS):
//   os=Windows  device_platform=PC  device_type=PC
//
// did/iid lifecycle (aligned with companion deviceIdManage):
//   1. client already has valid did+iid (env / ApplySecrets) → reuse
//   2. else secrets.params → reuse
//   3. else POST device_register (+ activate) → persist to secrets.params
// Never invent random ids; never hardcode another install's did.
//
// Hardware fingerprint comes from this host (see hostinfo.go). Protocol
// still reports os/device_platform as Windows/PC like the companion client.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xifan2333/webcast-mate/internal/conv"
	"github.com/xifan2333/webcast-mate/internal/platform"
	"github.com/xifan2333/webcast-mate/internal/secrets"
)

const (
	registerURL = hostAPI + "/service/2/desktop/device_register/"
	activateURL = hostAPI + "/service/2/app_alert_check/"
	fixURL      = hostStreaming + "/mate_core/api/device/fix-device-fingerprint"
	uaTT        = "TTNetwork PC"
)

func (c *Client) hasDevice() bool {
	return c.DeviceID != "" && c.IID != "" && c.DeviceID != "0" && c.IID != "0"
}

// EnsureDevice fills did/iid: env/client → secrets → device_register.
// Persists a successful pair into secrets.params so later runs reuse it.
func (c *Client) EnsureDevice(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if c.hasDevice() {
		// env or prior ApplySecrets — make sure secrets catch up
		_ = c.persistDeviceIDs()
		return nil
	}
	// secrets may hold did/iid without going through EnsureLogin's ApplySecrets
	if s, err := secrets.Load(platform.Douyin); err == nil {
		if did := s.Params["did"]; did != "" && did != "0" {
			c.DeviceID = did
		}
		if iid := s.Params["iid"]; iid != "" && iid != "0" {
			c.IID = iid
		}
		if c.hasDevice() {
			return nil
		}
	}

	if err := c.registerDevice(ctx); err != nil {
		return err
	}
	if !c.hasDevice() {
		return fmt.Errorf("device_register: empty did/iid after success")
	}
	if err := c.persistDeviceIDs(); err != nil {
		return fmt.Errorf("persist did/iid: %w", err)
	}
	return nil
}

// persistDeviceIDs merges did/iid into secrets (creates params-only file if needed).
func (c *Client) persistDeviceIDs() error {
	if !c.hasDevice() {
		return nil
	}
	var f *secrets.File
	if s, err := secrets.Load(platform.Douyin); err == nil {
		f = s
	} else {
		f = &secrets.File{Version: secrets.Version}
	}
	f.Normalize()
	if f.Params["did"] == c.DeviceID && f.Params["iid"] == c.IID {
		return nil
	}
	f.Params["did"] = c.DeviceID
	f.Params["iid"] = c.IID
	return secrets.Save(platform.Douyin, f)
}

func tzFields() (zone string, name string, offset int) {
	now := time.Now()
	name, offSec := now.Zone()
	jsOffMin := -offSec / 60
	offset = int(float64(jsOffMin) / -1 * 60)
	zone = "GMT" + now.Format("-0700")
	if name == "" {
		name = "CST"
	}
	return zone, name, offset
}

// deviceFingerprint builds companion-shaped register header from this host.
func (c *Client) deviceFingerprint() map[string]any {
	tz, tzName, tzOff := tzFields()
	h := localHostFingerprint()
	P := map[string]any{
		"os":              "Windows",
		"device_platform": "PC",
		"device_type":     "PC",
		"sdk_version":     "1.0.2",
		"aid":             2079,
		"channel":         "online",
		"package":         "webcast_mate",
		"language":        "",
		"app_version":     appVersion,
		"os_version":      "10.0.19045", // companion wire; not host kernel
		"device_model":    h.DeviceModel,
		"pc_uuid":         h.PCUUID,
		"pc_serial":       h.PCSerial,
		"mc":              h.MAC,
		"time_zone":       tz,
		"tz_name":         tzName,
		"tz_offset":       tzOff,
		"resolution":      h.Resolution,
		"app_region":      "",
		"app_language":    "",
		"display_name":    "",
	}
	if c.DeviceID != "" && c.DeviceID != "0" {
		P["device_id"] = c.DeviceID
	}
	if c.IID != "" && c.IID != "0" {
		P["install_id"] = c.IID
	}
	return P
}

func (c *Client) fixDeviceFingerprint(ctx context.Context, P map[string]any) error {
	body, err := json.Marshal(P)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fixURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return nil
	}
	if out["mate_fixed"] == nil {
		return nil
	}
	for k, v := range out {
		if k == "mate_fixed" {
			continue
		}
		if s := conv.AnyString(v); s != "" {
			P[k] = s
		}
	}
	// companion: mate_fixed==1 → drop device_id/install_id so server mints new ones
	if conv.AnyInt(out["mate_fixed"]) == 1 {
		delete(P, "device_id")
		delete(P, "install_id")
	}
	return nil
}

func (c *Client) registerDevice(ctx context.Context) error {
	P := c.deviceFingerprint()
	if P["pc_uuid"] == "" && P["mc"] == "" {
		return fmt.Errorf("device_register: empty machine fingerprint")
	}
	_ = c.fixDeviceFingerprint(ctx, P)

	plainM := map[string]any{"header": P, "_gen_time": 0, "magic_tag": "ss_app_log"}
	plainB, err := json.Marshal(plainM)
	if err != nil {
		return err
	}
	blob := leEncrypt(string(plainB))
	if len(blob) < 8 {
		return fmt.Errorf("device_register: encrypt failed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registerURL, strings.NewReader(string(blob)))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", uaTT)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("device_register: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("device_register: bad json (%d): %s", resp.StatusCode, conv.Truncate(string(b), 160))
	}
	did := conv.AnyString(m["device_id_str"])
	if did == "" {
		did = conv.AnyString(m["device_id"])
	}
	iid := conv.AnyString(m["install_id_str"])
	if iid == "" {
		iid = conv.AnyString(m["install_id"])
	}
	if did == "" || did == "0" {
		return fmt.Errorf("device_register: no device_id (%d): %s", resp.StatusCode, conv.Truncate(string(b), 200))
	}
	c.DeviceID = did
	c.IID = iid
	_ = c.activate(ctx, did, iid)
	return nil
}

func (c *Client) activate(ctx context.Context, did, iid string) error {
	q := url.Values{}
	q.Set("aid", "2079")
	q.Set("app_name", "直播伴侣")
	q.Set("channel", "online")
	q.Set("device_id", did)
	q.Set("iid", iid)
	q.Set("os", "Windows")
	q.Set("device_platform", "PC")
	q.Set("version_code", appVersion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, activateURL+"?"+q.Encode(), strings.NewReader(""))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", uaTT)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
