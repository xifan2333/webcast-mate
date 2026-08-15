package bilibili

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/xifan2333/webcast-mate/internal/session"
)

type RoomState struct {
	RoomID   string    `json:"room_id"`
	Server   string    `json:"server,omitempty"`
	Key      string    `json:"key,omitempty"`
	AreaV2   string    `json:"area_v2,omitempty"`
	Title    string    `json:"title,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
}

func roomStatePath() (string, error) {
	d, err := session.PlatformDir("bilibili")
	if err != nil {
		return "", err
	}
	return d + "/room.json", nil
}

func LoadRoomState() (*RoomState, error) {
	path, err := roomStatePath()
	if err != nil {
		return nil, err
	}
	var s RoomState
	if err := session.ReadJSON(path, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func SaveRoomState(s *RoomState) error {
	path, err := roomStatePath()
	if err != nil {
		return err
	}
	return session.WriteJSON(path, s)
}

func ClearRoomState() error {
	path, err := roomStatePath()
	if err != nil {
		return err
	}
	return session.Remove(path)
}

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

func (c *Client) updateTitle(roomID, title, csrf string) error {
	if title == "" {
		return nil
	}
	form := url.Values{}
	form.Set("room_id", roomID)
	form.Set("title", title)
	form.Set("platform", "pc_link")
	form.Set("csrf_token", csrf)
	form.Set("csrf", csrf)
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
		return fmt.Errorf("update title: %s (%d)", msg, env.Code)
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
		fmt.Fprintln(os.Stderr, "bilibili: face auth required — scan with bilibili app")
		fmt.Fprintln(os.Stderr, qr)
		printTerminalQR(qr)
	} else {
		fmt.Fprintln(os.Stderr, "bilibili: face auth required (no qr url)")
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
					fmt.Fprintln(os.Stderr, "bilibili: face auth ok")
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

// StartLive runs update title + startLive (+ face auth retry).
func (c *Client) StartLive(ctx context.Context, cfg *Config) (server, key string, err error) {
	csrf := c.csrf()
	if csrf == "" {
		return "", "", ErrNotLoggedIn
	}
	if err := c.updateTitle(cfg.RoomID, cfg.Title, csrf); err != nil {
		// non-fatal? oil monkey treats as fatal — keep fatal
		return "", "", err
	}

	data, code, msg, err := c.startLiveOnce(cfg.RoomID, cfg.AreaV2, csrf)
	if err != nil {
		return "", "", err
	}
	if code == 60024 || (data != nil && data.QR != "") {
		if err := c.waitFaceAuth(ctx, cfg.RoomID, data.QR, csrf); err != nil {
			return "", "", err
		}
		data, code, msg, err = c.startLiveOnce(cfg.RoomID, cfg.AreaV2, csrf)
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
