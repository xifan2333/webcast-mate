package bilibili

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

type apiEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Msg     string          `json:"msg"`
	Data    json.RawMessage `json:"data"`
}

type startLiveData struct {
	QR   string `json:"qr"`
	RTMP struct {
		Addr string `json:"addr"`
		Code string `json:"code"`
	} `json:"rtmp"`
}

func (c *Client) updateRoom(roomID, title, areaID, csrf string) error {
	if title == "" && areaID == "" {
		return nil
	}
	form := url.Values{}
	form.Set("room_id", roomID)
	form.Set("platform", "pc_link")
	form.Set("csrf_token", csrf)
	form.Set("csrf", csrf)
	if title != "" {
		form.Set("title", title)
	}
	if areaID != "" {
		form.Set("area_id", areaID)
	}
	b, err := c.doJSON("POST", urlUpdateRoom, form, nil)
	if err != nil {
		return err
	}
	var env apiEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		return err
	}
	if env.Code != 0 {
		msg := env.Message
		if msg == "" {
			msg = env.Msg
		}
		return fmt.Errorf("update room: %s (%d)", msg, env.Code)
	}
	return nil
}

func (c *Client) startLiveOnce(roomID, areaV2, csrf string) (*startLiveData, int, string, error) {
	form := url.Values{}
	form.Set("room_id", roomID)
	form.Set("platform", "pc_link")
	form.Set("area_v2", areaV2)
	form.Set("backup_stream", "0")
	form.Set("csrf_token", csrf)
	form.Set("csrf", csrf)
	b, err := c.doJSON("POST", urlStartLive, form, nil)
	if err != nil {
		return nil, -1, "", err
	}
	var env apiEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, -1, "", err
	}
	msg := env.Message
	if msg == "" {
		msg = env.Msg
	}
	var data startLiveData
	if len(env.Data) > 0 && string(env.Data) != "null" {
		_ = json.Unmarshal(env.Data, &data)
	}
	return &data, env.Code, msg, nil
}

func (c *Client) waitFaceAuth(ctx context.Context, roomID, qr, csrf string) error {
	if qr != "" {
		close := printQR(qr)
		defer close()
	}
	form := url.Values{}
	form.Set("room_id", roomID)
	form.Set("face_auth_code", "60024")
	form.Set("csrf_token", csrf)
	form.Set("csrf", csrf)
	form.Set("visit_id", "")

	deadline := time.Now().Add(2 * time.Minute)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return ErrFaceTimeout
		}
		b, err := c.doJSON("POST", urlFaceAuth, form, nil)
		if err == nil {
			var env apiEnvelope
			if json.Unmarshal(b, &env) == nil && env.Code == 0 {
				var data struct {
					IsIdentified bool `json:"is_identified"`
				}
				if json.Unmarshal(env.Data, &data) == nil && data.IsIdentified {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// StartLive applies title/area then startLive (+ face auth retry).
// Cover is not set from CLI; platform keeps the existing room cover.
func (c *Client) StartLive(ctx context.Context, cfg *OpenConfig) (server, key string, err error) {
	csrf := c.csrf()
	if csrf == "" {
		return "", "", ErrNotLoggedIn
	}
	if err := c.updateRoom(cfg.RoomID, cfg.Title, cfg.Area, csrf); err != nil {
		return "", "", err
	}

	data, code, msg, err := c.startLiveOnce(cfg.RoomID, cfg.Area, csrf)
	if err != nil {
		return "", "", err
	}
	if code == 60024 || (data != nil && data.QR != "") {
		if err := c.waitFaceAuth(ctx, cfg.RoomID, data.QR, csrf); err != nil {
			return "", "", err
		}
		data, code, msg, err = c.startLiveOnce(cfg.RoomID, cfg.Area, csrf)
		if err != nil {
			return "", "", err
		}
	}
	if code != 0 {
		return "", "", fmt.Errorf("startLive: %s (%d)", msg, code)
	}
	if data == nil || data.RTMP.Addr == "" || data.RTMP.Code == "" {
		return "", "", fmt.Errorf("startLive: empty rtmp")
	}
	return data.RTMP.Addr, data.RTMP.Code, nil
}

// StopLive ends the stream.
func (c *Client) StopLive(roomID string) error {
	csrf := c.csrf()
	if csrf == "" {
		return ErrNotLoggedIn
	}
	if roomID == "" {
		return nil
	}
	form := url.Values{}
	form.Set("room_id", roomID)
	form.Set("platform", "pc_link")
	form.Set("csrf_token", csrf)
	form.Set("csrf", csrf)
	b, err := c.doJSON("POST", urlStopLive, form, nil)
	if err != nil {
		return err
	}
	var env apiEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		return err
	}
	if env.Code != 0 {
		msg := env.Message
		if msg == "" {
			msg = env.Msg
		}
		// already stopped etc. — treat some as ok?
		return fmt.Errorf("stopLive: %s (%d)", msg, env.Code)
	}
	return nil
}

type BlinkRoomInfo struct {
	RoomID, UID, Title, AreaV2ID, AreaV2Name, ParentID, ParentName, Face string
	LiveStatus                                                           int
}

func (c *Client) GetBlinkRoomInfo() (*BlinkRoomInfo, error) {
	b, err := c.doJSON("GET", urlBlinkGetInfo, nil, map[string]string{
		"Origin": "https://link.bilibili.com", "Referer": "https://link.bilibili.com/p/center/index",
		"Accept": "application/json, text/plain, */*",
	})
	if err != nil {
		return nil, err
	}
	var env apiEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	if env.Code != 0 {
		msg := env.Message
		if msg == "" {
			msg = env.Msg
		}
		return nil, fmt.Errorf("blink GetInfo: %s (%d)", msg, env.Code)
	}
	var data struct {
		RoomID     any    `json:"room_id"`
		UID        any    `json:"uid"`
		Title      string `json:"title"`
		AreaV2ID   any    `json:"area_v2_id"`
		AreaV2Name string `json:"area_v2_name"`
		ParentID   any    `json:"parent_id"`
		ParentName string `json:"parent_name"`
		LiveStatus int    `json:"live_status"`
		Face       string `json:"face"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return nil, err
	}
	rid := anyToString(data.RoomID)
	if rid == "" {
		return nil, fmt.Errorf("blink GetInfo: empty room_id")
	}
	return &BlinkRoomInfo{RoomID: rid, UID: anyToString(data.UID), Title: data.Title,
		AreaV2ID: anyToString(data.AreaV2ID), AreaV2Name: data.AreaV2Name,
		ParentID: anyToString(data.ParentID), ParentName: data.ParentName,
		LiveStatus: data.LiveStatus, Face: data.Face}, nil
}

// RoomLiveInfo is a remote room snapshot from get_info.
type RoomLiveInfo struct {
	RoomID     string
	LiveStatus int // 0 idle, 1 live, 2 round
	Title      string
}

// QueryRoomInfo hits GET room/v1/Room/get_info (public; cookie optional).
func (c *Client) QueryRoomInfo(roomID string) (*RoomLiveInfo, error) {
	if roomID == "" {
		return nil, fmt.Errorf("empty room_id")
	}
	u := urlRoomInfo + "?room_id=" + url.QueryEscape(roomID)
	b, err := c.doJSON("GET", u, nil, nil)
	if err != nil {
		return nil, err
	}
	var env apiEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	if env.Code != 0 {
		msg := env.Message
		if msg == "" {
			msg = env.Msg
		}
		return nil, fmt.Errorf("get_info: %s (%d)", msg, env.Code)
	}
	var data struct {
		RoomID     any    `json:"room_id"`
		LiveStatus int    `json:"live_status"`
		Title      string `json:"title"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return nil, err
	}
	rid := anyToString(data.RoomID)
	if rid == "" {
		rid = roomID
	}
	return &RoomLiveInfo{RoomID: rid, LiveStatus: data.LiveStatus, Title: data.Title}, nil
}

// LiveStatusString maps bilibili live_status to our status field.
func LiveStatusString(code int) string {
	switch code {
	case 1:
		return "live"
	case 2:
		return "round"
	default:
		return "idle"
	}
}
