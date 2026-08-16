package bilibili

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
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

// updateCover sets cover via UpdatePreLiveInfo (URL must be on *.hdslb.com).
func (c *Client) updateCover(coverURL, csrf string) error {
	if coverURL == "" {
		return nil
	}
	form := url.Values{}
	form.Set("platform", "web")
	form.Set("mobi_app", "web")
	form.Set("build", "1")
	form.Set("cover", coverURL)
	form.Set("coverVertical", "")
	form.Set("liveDirectionType", "1")
	form.Set("visit_id", "")
	form.Set("csrf_token", csrf)
	form.Set("csrf", csrf)
	b, err := c.doJSON("POST", urlUpdatePreLive, form, nil)
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
		return fmt.Errorf("update cover: %s (%d)", msg, env.Code)
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
		printQR(qr)
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

// StartLive applies title/area/cover then startLive (+ face auth retry).
func (c *Client) StartLive(ctx context.Context, cfg *Config) (server, key string, err error) {
	csrf := c.csrf()
	if csrf == "" {
		return "", "", ErrNotLoggedIn
	}
	if err := c.updateRoom(cfg.RoomID, cfg.Title, cfg.AreaV2, csrf); err != nil {
		return "", "", err
	}
	if cover := strings.TrimSpace(cfg.Cover); cover != "" {
		if err := c.applyCover(cover, csrf); err != nil {
			// cover optional — warn but continue open
			fmt.Fprintf(os.Stderr, "bilibili: cover: %v\n", err)
		}
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

// applyCover sets cover URL via UpdatePreLiveInfo.
// Local file upload is not implemented yet; only https://*.hdslb.com/… URLs work.
func (c *Client) applyCover(cover, csrf string) error {
	if strings.HasPrefix(cover, "http://") || strings.HasPrefix(cover, "https://") {
		if !strings.Contains(cover, "hdslb.com") {
			return fmt.Errorf("cover URL must be on *.hdslb.com (got %s)", cover)
		}
		return c.updateCover(cover, csrf)
	}
	// local path — not yet: needs BFS upload token flow
	if _, err := os.Stat(cover); err == nil {
		return fmt.Errorf("local cover upload not implemented yet; use an existing hdslb.com URL or leave empty")
	}
	return fmt.Errorf("invalid cover %q", cover)
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
