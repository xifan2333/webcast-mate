package douyin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/xifan2333/webcast-mate/internal/conv"
)

// Face challenge from room/create status_code=4003028 (companion face-detect).
const (
	createFaceRequired = 4003028
	faceStatusPending  = 0
	faceStatusSuccess  = 1
	faceStatusFailed   = 2
	faceStatusError    = 3
	facePollInterval   = 2 * time.Second
	defaultFaceWait    = 10 * time.Minute
)

// CreateInfo → preschedule_key + cover uri.
func (c *Client) CreateInfo() (prekey, cover string, err error) {
	q := c.commonQuery()
	form := url.Values{}
	form.Set("enable_preview_stream", "1")
	form.Set("speed_test_info", "[]")
	form.Set("live_room_mode", "1")
	form.Set("orientation", "1")
	body := form.Encode()
	qs, err := withABogus(q.Encode(), body)
	if err != nil {
		return "", "", err
	}
	m, err := c.postForm(hostAPI+"/webcast/room/create_info/?"+qs, form, nil)
	if err != nil {
		return "", "", err
	}
	data := mapData(m)
	if data == nil {
		return "", "", fmt.Errorf("create_info: %v", m)
	}
	ps, _ := data["preview_stream"].(map[string]any)
	if ps == nil {
		return "", "", fmt.Errorf("create_info no preview_stream: %v", m)
	}
	prekey = conv.AnyString(ps["preschedule_key"])
	coverM, _ := data["cover"].(map[string]any)
	if coverM != nil {
		cover = conv.AnyString(coverM["uri"])
	}
	if prekey == "" {
		return "", "", fmt.Errorf("create_info empty prekey: %v", m)
	}
	return prekey, cover, nil
}

func buildCreateBody(title, prekey, cover, categoryEnc string) string {
	base, leaf := ParseCategoryValue(categoryEnc)
	if base == "" {
		base = "-1"
	}
	if leaf == "" {
		leaf = "-1"
	}
	payload := `[["webcast","gift_menu_flow_mode","true"],["game","sei_change","true"],["game","pk_dual_screen","false"],["game","enable_rtc","true"],["webcast","client_ai_lab","false"],["webcast","client_ocr_lab","false"]]`
	form := url.Values{}
	form.Set("multi_resolution", "true")
	form.Set("title", title)
	form.Set("orientation", "1")
	form.Set("base_category", base)
	form.Set("category", leaf)
	form.Set("has_commerce_goods", "false")
	form.Set("disable_location_permission", "1")
	form.Set("push_stream_type", "1")
	form.Set("auto_cover", "2")
	form.Set("payload", payload)
	form.Set("visibility_range", "0")
	form.Set("pre_schedule_key", prekey)
	form.Set("third_party", "1")
	form.Set("enable_health_score_check", "true")
	form.Set("cover_uri", cover)
	form.Set("thumb_width", "1280")
	form.Set("thumb_height", "780")
	form.Set("audience_display_type", "99")
	form.Set("gift_auth", "1")
	form.Set("gen_replay", "true")
	form.Set("record_screen", "1")
	form.Set("account", "douyin")
	form.Set("max_bit_rate", "4000000")
	return form.Encode()
}

// CreateResult from room/create + living ping.
type CreateResult struct {
	RoomID   string
	StreamID string
	PushURL  string
	Title    string
	Server   string
	Key      string
}

// faceChallenge carries companion 4003028 extra fields.
type faceChallenge struct {
	AuthURL  string
	Ticket   string
	Scene    string
	FaceType string
	Prompt   string
}

func parseFaceChallenge(m map[string]any) *faceChallenge {
	extra, _ := m["extra"].(map[string]any)
	if extra == nil {
		return nil
	}
	u := conv.AnyString(extra["web_auth_address"])
	if u == "" {
		return nil
	}
	fc := &faceChallenge{
		AuthURL:  u,
		Ticket:   conv.AnyString(extra["ticket"]),
		Scene:    conv.AnyString(extra["scene"]),
		FaceType: conv.AnyString(extra["face_type"]),
	}
	if data, _ := m["data"].(map[string]any); data != nil {
		fc.Prompt = conv.AnyString(data["prompts"])
	}
	return fc
}

func openAuthURL(u string) {
	if u == "" {
		return
	}
	// Prefer host browser (has real camera); fall back to printing URL.
	for _, bin := range []string{"xdg-open", "gio"} {
		if p, err := exec.LookPath(bin); err == nil {
			cmd := exec.Command(p, u)
			if bin == "gio" {
				cmd = exec.Command(p, "open", u)
			}
			cmd.Stdout = nil
			cmd.Stderr = nil
			_ = cmd.Start()
			return
		}
	}
}

// waitFaceResult polls companion get_face_result until SUCCESS/FAILED/timeout.
// Companion: GET /webcast/anchor/pc_live/get_face_result/ every 2s.
func (c *Client) waitFaceResult(ctx context.Context) error {
	deadline := time.Now().Add(defaultFaceWait)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	failStreak := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("face auth timeout (%s)", defaultFaceWait)
		}
		q := c.pingQuery()
		qs, err := withABogus(q.Encode(), "")
		if err != nil {
			return err
		}
		m, err := c.getJSON(hostAPI+"/webcast/anchor/pc_live/get_face_result/?"+qs, nil)
		if err != nil {
			failStreak++
			sleep := facePollInterval * time.Duration(failStreak)
			if sleep > 10*time.Second {
				sleep = 10 * time.Second
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sleep):
			}
			continue
		}
		failStreak = 0
		sc := conv.AnyInt(m["status_code"])
		if sc != 0 {
			// keep polling — companion only switches on data.status when sc==0
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(facePollInterval):
			}
			continue
		}
		data, _ := m["data"].(map[string]any)
		st := faceStatusPending
		if data != nil {
			st = conv.AnyInt(data["status"])
		}
		switch st {
		case faceStatusSuccess:
			return nil
		case faceStatusFailed, faceStatusError:
			return fmt.Errorf("face auth failed (status=%d)", st)
		default: // PENDING
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(facePollInterval):
			}
		}
	}
}

func (c *Client) postCreateOnce(title, prekey, cover, categoryEnc string) (map[string]any, http.Header, error) {
	body := buildCreateBody(title, prekey, cover, categoryEnc)
	query, err := withABogus(c.commonQuery().Encode(), body)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrABogus, err)
	}
	full := hostAPI + "/webcast/room/create/?" + query
	extra := map[string]string{
		"bd-ticket-guard-client-data":       ticketData(),
		"bd-ticket-guard-version":           "2",
		"bd-ticket-guard-iteration-version": "2",
		"sec-ch-ua":                         `"Not.A/Brand";v="99", "Chromium";v="136"`,
		"sec-ch-ua-mobile":                  "?0",
		"sec-ch-ua-platform":                `"Windows"`,
	}
	b, hdr, err := c.doHDR("POST", full, strings.NewReader(body), "application/x-www-form-urlencoded; charset=UTF-8", extra)
	if err != nil {
		return nil, hdr, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, hdr, err
	}
	return m, hdr, nil
}

func parseCreateOK(m map[string]any) (*CreateResult, error) {
	data := mapData(m)
	if data == nil {
		return nil, fmt.Errorf("room/create no data")
	}
	roomID := conv.AnyString(data["id_str"])
	if roomID == "" {
		roomID = conv.AnyString(data["id"])
	}
	streamID := conv.AnyString(data["stream_id_str"])
	if streamID == "" {
		streamID = conv.AnyString(data["stream_id"])
	}
	su, _ := data["stream_url"].(map[string]any)
	push := ""
	if su != nil {
		push = conv.AnyString(su["rtmp_push_url"])
		if push == "" {
			push = conv.AnyString(su["rtmps_push_url"])
		}
	}
	if push == "" {
		return nil, fmt.Errorf("room/create no rtmp_push_url")
	}
	server, key := splitPushURL(push)
	return &CreateResult{
		RoomID:   roomID,
		StreamID: streamID,
		PushURL:  push,
		Title:    conv.AnyString(data["title"]),
		Server:   server,
		Key:      key,
	}, nil
}

// CreateRoom: create_info → create; on 4003028 open face URL, poll get_face_result, retry create → ping LIVING.
// categoryEnc is "base|leaf" from ListCategories; empty keeps -1/-1.
func (c *Client) CreateRoom(ctx context.Context, title, categoryEnc string) (*CreateResult, error) {
	if title == "" {
		title = "Live"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	prekey, cover, err := c.CreateInfo()
	if err != nil {
		return nil, err
	}

	var cr *CreateResult
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// refresh prekey on retries after face (companion re-enters create flow)
		if attempt > 0 {
			prekey, cover, err = c.CreateInfo()
			if err != nil {
				return nil, err
			}
		}
		m, hdr, err := c.postCreateOnce(title, prekey, cover, categoryEnc)
		if err != nil {
			return nil, err
		}
		// companion: response header forces secondary passport verify
		if hdr != nil && hdr.Get("x-tt-verify-passport-decision") != "" {
			return nil, &CreateError{
				Code:   conv.AnyInt(m["status_code"]),
				Kind:   "passport",
				Prompt: "passport secondary verification required (re-login)",
				Err:    ErrPassportVerify,
			}
		}
		sc := conv.AnyInt(m["status_code"])
		if sc == 0 {
			cr, err = parseCreateOK(m)
			if err != nil {
				return nil, err
			}
			break
		}
		ce := classifyCreateResponse(m)
		if ce != nil && ce.Kind == "face" && ce.AuthURL != "" {
			// stderr: one JSONL event (stderr = diagnostics, no progress spam)
			enc := json.NewEncoder(os.Stderr)
			enc.SetEscapeHTML(false)
			_ = enc.Encode(map[string]any{
				"event":    "face_auth_required",
				"platform": "douyin",
				"prompt":   ce.Prompt,
				"url":      ce.AuthURL,
			})
			openAuthURL(ce.AuthURL)
			if err := c.waitFaceResult(ctx); err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
					return nil, fmt.Errorf("%w: %v", ErrFaceRequired, err)
				}
				return nil, fmt.Errorf("%w: %v", ErrFaceFailed, err)
			}
			continue
		}
		if ce == nil {
			ce = &CreateError{Code: sc, Kind: "unknown", Err: ErrCreateFailed}
		}
		return nil, ce
	}
	if cr == nil {
		return nil, fmt.Errorf("room/create: still blocked after face retries")
	}
	if err := c.PingAnchor(cr.RoomID, cr.StreamID, RoomLiving); err != nil {
		return nil, fmt.Errorf("ping LIVING: %w", err)
	}
	return cr, nil
}

// PingAnchor reports room status (2=LIVING, 4=FINISH).
func (c *Client) PingAnchor(roomID, streamID string, status int) error {
	q := c.pingQuery()
	form := url.Values{}
	form.Set("room_id", roomID)
	form.Set("stream_id", streamID)
	form.Set("status", fmt.Sprintf("%d", status))
	body := form.Encode()
	qs, err := withABogus(q.Encode(), body)
	if err != nil {
		return err
	}
	m, err := c.postForm(hostAPI+"/webcast/room/ping/anchor/?"+qs, form, nil)
	if err != nil {
		return err
	}
	sc, _ := m["status_code"].(float64)
	if sc != 0 {
		return fmt.Errorf("ping status_code=%v msg=%v", sc, m["data"])
	}
	return nil
}

// CheckRoomExist probes room presence.
func (c *Client) CheckRoomExist(roomID string) (map[string]any, error) {
	q := c.pingQuery()
	q.Set("room_id", roomID)
	qs, err := withABogus(q.Encode(), "")
	if err != nil {
		return nil, err
	}
	return c.getJSON(hostAPI+"/webcast/room/check_exist/?"+qs, nil)
}

// PCObsStatus best-effort remote live status.
func (c *Client) PCObsStatus() (map[string]any, error) {
	q := c.pingQuery()
	qs, err := withABogus(q.Encode(), "")
	if err != nil {
		return nil, err
	}
	return c.getJSON(hostAPI+"/webcast/room/get_pc_obs_status/?"+qs, nil)
}

func splitPushURL(push string) (server, key string) {
	push = strings.TrimSpace(push)
	if push == "" {
		return "", ""
	}
	rest := push
	scheme := "rtmp://"
	if strings.HasPrefix(rest, "rtmps://") {
		scheme = "rtmps://"
		rest = rest[len("rtmps://"):]
	} else if strings.HasPrefix(rest, "rtmp://") {
		rest = rest[len("rtmp://"):]
	} else {
		return push, ""
	}
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return scheme + rest, ""
	}
	host := rest[:slash]
	pathq := rest[slash+1:]
	app, keyPart, ok := strings.Cut(pathq, "/")
	if !ok {
		return scheme + host + "/" + pathq, ""
	}
	return scheme + host + "/" + app, keyPart
}
