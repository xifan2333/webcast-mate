package douyin

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// CreateInfo → preschedule_key + cover uri.
func (c *Client) CreateInfo() (prekey, cover string, err error) {
	q := c.commonQuery()
	form := url.Values{}
	form.Set("enable_preview_stream", "1")
	form.Set("speed_test_info", "[]")
	form.Set("live_room_mode", "1")
	form.Set("orientation", "1")
	m, err := c.postForm(hostPC+"/webcast/room/create_info/?"+q.Encode(), form, nil)
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
	prekey = anyString(ps["preschedule_key"])
	coverM, _ := data["cover"].(map[string]any)
	if coverM != nil {
		cover = anyString(coverM["uri"])
	}
	if prekey == "" {
		return "", "", fmt.Errorf("create_info empty prekey: %v", m)
	}
	return prekey, cover, nil
}

func buildCreateBody(title, prekey, cover string) string {
	payload := `[["webcast","gift_menu_flow_mode","true"],["game","sei_change","true"],["game","pk_dual_screen","false"],["game","enable_rtc","true"],["webcast","client_ai_lab","false"],["webcast","client_ocr_lab","false"]]`
	form := url.Values{}
	form.Set("multi_resolution", "true")
	form.Set("title", title)
	form.Set("orientation", "1")
	form.Set("base_category", "-1")
	form.Set("category", "-1")
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

// CreateRoom: create_info → a_bogus → create → ping LIVING.
func (c *Client) CreateRoom(title string) (*CreateResult, error) {
	if title == "" {
		title = "直播"
	}
	prekey, cover, err := c.CreateInfo()
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "douyin: create_info ok prekey_len=%d\n", len(prekey))

	body := buildCreateBody(title, prekey, cover)
	query := c.commonQuery().Encode()

	ab, err := SignABogus(query, body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrABogus, err)
	}
	fmt.Fprintf(os.Stderr, "douyin: a_bogus len=%d\n", len(ab))

	full := hostPC + "/webcast/room/create/?" + query + "&a_bogus=" + url.QueryEscape(ab)
	extra := map[string]string{
		"bd-ticket-guard-client-data":       ticketData(),
		"bd-ticket-guard-version":           "2",
		"bd-ticket-guard-iteration-version": "2",
		"sec-ch-ua":                         `"Not.A/Brand";v="99", "Chromium";v="136"`,
		"sec-ch-ua-mobile":                  "?0",
		"sec-ch-ua-platform":                `"Windows"`,
	}
	b, err := c.do("POST", full, strings.NewReader(body), "application/x-www-form-urlencoded; charset=UTF-8", extra)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	sc, _ := m["status_code"].(float64)
	if sc != 0 {
		return nil, fmt.Errorf("room/create status_code=%v data=%v", sc, m["data"])
	}
	data := mapData(m)
	if data == nil {
		return nil, fmt.Errorf("room/create no data: %s", truncate(string(b), 200))
	}
	roomID := anyString(data["id_str"])
	if roomID == "" {
		roomID = anyString(data["id"])
	}
	streamID := anyString(data["stream_id_str"])
	if streamID == "" {
		streamID = anyString(data["stream_id"])
	}
	su, _ := data["stream_url"].(map[string]any)
	push := ""
	if su != nil {
		push = anyString(su["rtmp_push_url"])
		if push == "" {
			push = anyString(su["rtmps_push_url"])
		}
	}
	if push == "" {
		return nil, fmt.Errorf("room/create no rtmp_push_url")
	}
	if err := c.PingAnchor(roomID, streamID, RoomLiving); err != nil {
		fmt.Fprintf(os.Stderr, "douyin: warn ping LIVING: %v\n", err)
	} else {
		fmt.Fprintln(os.Stderr, "douyin: ping LIVING ok")
	}
	server, key := splitPushURL(push)
	return &CreateResult{
		RoomID:   roomID,
		StreamID: streamID,
		PushURL:  push,
		Title:    anyString(data["title"]),
		Server:   server,
		Key:      key,
	}, nil
}

// PingAnchor reports room status (2=LIVING, 4=FINISH).
func (c *Client) PingAnchor(roomID, streamID string, status int) error {
	q := c.commonQuery()
	form := url.Values{}
	form.Set("room_id", roomID)
	form.Set("stream_id", streamID)
	form.Set("status", fmt.Sprintf("%d", status))
	m, err := c.postForm(hostAPI+"/webcast/room/ping/anchor/?"+q.Encode(), form, nil)
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
	q := c.commonQuery()
	q.Set("room_id", roomID)
	return c.getJSON(hostAPI+"/webcast/room/check_exist/?"+q.Encode(), nil)
}

// PCObsStatus best-effort remote live status.
func (c *Client) PCObsStatus() (map[string]any, error) {
	q := c.commonQuery()
	return c.getJSON(hostAPI+"/webcast/room/get_pc_obs_status/?"+q.Encode(), nil)
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
