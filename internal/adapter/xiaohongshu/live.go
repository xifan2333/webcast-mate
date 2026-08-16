package xiaohongshu

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/xifan2333/webcast-mate/internal/conv"
)

// PreRoom POST redobs …/center/room/pre
func (c *Client) PreRoom() (roomID, pushURL string, raw map[string]any, err error) {
	m, err := c.do(http.MethodPost, hostRedobs,
		"/api/sns/redobs/live/app/v1/center/room/pre",
		map[string]any{"app_id": "1"}, nil,
		doOpts{redobs: true, originRobs: true})
	if err != nil {
		return "", "", nil, err
	}
	if !bizOK(m) {
		return "", "", m, fmt.Errorf("pre: %v", m)
	}
	roomID, pushURL = extractRoomPush(m)
	c.RoomID = roomID
	c.PushURL = pushURL
	return roomID, pushURL, m, nil
}

// BeforeStart POST …/center/room/before/start
func (c *Client) BeforeStart(roomID string) error {
	m, err := c.do(http.MethodPost, hostRedobs,
		"/api/sns/redobs/live/app/v1/center/room/before/start",
		map[string]any{"room_id": roomID, "app_id": "1"}, nil,
		doOpts{redobs: true, originRobs: true})
	if err != nil {
		return err
	}
	if !bizOK(m) {
		return fmt.Errorf("before/start: %v", m)
	}
	data, _ := m["data"].(map[string]any)
	if data != nil {
		if allow, ok := data["allow_start"].(bool); ok && !allow {
			return fmt.Errorf("%w: %v", ErrStartDenied, data)
		}
	}
	return nil
}

// ReportPushInfo POST robs …/live/room/report_push_info
func (c *Client) ReportPushInfo(roomID, pushURL string, w, h, bitrate, fps int) error {
	if w == 0 {
		w = 1280
	}
	if h == 0 {
		h = 720
	}
	if bitrate == 0 {
		bitrate = 2500
	}
	if fps == 0 {
		fps = 30
	}
	pr, _ := json.Marshal(map[string]any{
		"codec": 0, "push_type": 3, "bitrate": bitrate, "resolution": 1, "fps": fps,
		"height": h, "width": w, "push_url": pushURL,
		"camera_type": "none", "voice_type": "real_micro",
	})
	m, err := c.do(http.MethodPost, hostRobs,
		"/api/sns/live/room/report_push_info",
		map[string]any{"room_id": roomID, "push_result": string(pr)}, nil,
		doOpts{originRobs: true})
	if err != nil {
		return err
	}
	if !bizOK(m) {
		return fmt.Errorf("report_push_info: %v", m)
	}
	return nil
}

// StartRoom POST …/center/room/start  (verified body).
// categoryValue: game leaf id → categoryIds:[id]; "other"/empty → [].
func (c *Client) StartRoom(roomID, title, cover string, distribute int, categoryValue string) error {
	if distribute != 0 && distribute != 1 {
		distribute = 1
	}
	catIDs := []any{}
	if id := CategoryIDForStart(categoryValue); id != "" {
		catIDs = []any{id}
	}
	body := map[string]any{
		"room_id":      roomID,
		"app_id":       "1",
		"style":        1, // PC
		"push_type":    0, // FollowDispatch
		"content_type": 0, // Video
		"obs":          1, // Obs
		"distribute":   distribute,
		"join_limit":   0, // All
		"name":         title,
		"cover":        cover,
		"cover_url":    cover,
		"categoryIds":  catIDs,
	}
	m, err := c.do(http.MethodPost, hostRedobs,
		"/api/sns/redobs/live/app/v1/center/room/start",
		body, nil, doOpts{redobs: true, originRobs: true})
	if err != nil {
		return err
	}
	if !bizOK(m) {
		return fmt.Errorf("start: %v", m)
	}
	// nested result.success == false is failure
	if data, _ := m["data"].(map[string]any); data != nil {
		if res, _ := data["result"].(map[string]any); res != nil {
			if s, ok := res["success"].(bool); ok && !s {
				return fmt.Errorf("start result: %v", res)
			}
		}
	}
	c.RoomID = roomID
	return nil
}

// StopRoom POST …/center/room/stop
func (c *Client) StopRoom(roomID string) error {
	if roomID == "" {
		roomID = c.RoomID
	}
	if roomID == "" {
		return nil
	}
	m, err := c.do(http.MethodPost, hostRedobs,
		"/api/sns/redobs/live/app/v1/center/room/stop",
		map[string]any{"room_id": roomID, "app_id": "1"}, nil,
		doOpts{redobs: true, originRobs: true})
	if err != nil {
		return err
	}
	if !bizOK(m) {
		return fmt.Errorf("stop: %v", m)
	}
	return nil
}

// StreamInfo GET …/get_stream_info
func (c *Client) StreamInfo(roomID string) (map[string]any, error) {
	q := url.Values{}
	q.Set("room_id", roomID)
	q.Set("app_id", "1")
	return c.do(http.MethodGet, hostRedobs,
		"/api/sns/redobs/live/app/v1/center/room/get_stream_info",
		nil, q, doOpts{redobs: true, originRobs: true})
}

// LastRoomInfo GET …/last_room_info?host_id=
func (c *Client) LastRoomInfo() (title, cover string, err error) {
	if c.UserID == "" {
		return "", "", fmt.Errorf("no user_id")
	}
	q := url.Values{}
	q.Set("host_id", c.UserID)
	m, err := c.do(http.MethodGet, hostRedobs,
		"/api/sns/redobs/live/app/v1/room/last_room_info",
		nil, q, doOpts{redobs: true, originRobs: true})
	if err != nil {
		return "", "", err
	}
	if !bizOK(m) {
		return "", "", fmt.Errorf("last_room_info: %v", m)
	}
	data, _ := m["data"].(map[string]any)
	if data == nil {
		return "", "", nil
	}
	ri, _ := data["room_info"].(map[string]any)
	if ri == nil {
		return "", "", nil
	}
	title = conv.AnyString(ri["room_name"])
	if title == "" {
		title = conv.AnyString(ri["name"])
	}
	if ci, _ := ri["cover_info"].(map[string]any); ci != nil {
		cover = conv.AnyString(ci["cover_url"])
	}
	return title, cover, nil
}

func extractRoomPush(m map[string]any) (roomID, pushURL string) {
	data, _ := m["data"].(map[string]any)
	if data == nil {
		return "", ""
	}
	live, _ := data["live_info"].(map[string]any)
	if live == nil {
		live = data
	}
	if ri, _ := live["room_info"].(map[string]any); ri != nil {
		roomID = conv.AnyString(ri["room_id"])
	}
	if roomID == "" {
		roomID = conv.AnyString(data["room_id"])
	}
	// walk for rtmp
	raw, _ := json.Marshal(m)
	re := regexp.MustCompile(`rtmps?://[^"\\\s]+`)
	if loc := re.Find(raw); loc != nil {
		pushURL = string(loc)
		// unescape \u0026 etc — json already unescaped in string values; from raw marshal may have
		pushURL = strings.ReplaceAll(pushURL, `\u0026`, "&")
	}
	// also try push_dispatch_config
	if pushURL == "" {
		if si, _ := live["stream_info"].(map[string]any); si != nil {
			if pi, _ := si["push_info"].(map[string]any); pi != nil {
				if cfg, ok := pi["push_dispatch_config"].(string); ok {
					if loc := re.FindString(cfg); loc != "" {
						pushURL = loc
					}
				}
			}
		}
	}
	return roomID, pushURL
}

// SplitPushURL splits rtmp://host/app/key?q into server + key.
func SplitPushURL(push string) (server, key string) {
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
