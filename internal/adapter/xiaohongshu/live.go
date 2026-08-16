package xiaohongshu

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type preData struct {
	RoomID string `json:"room_id"`
	Name   string `json:"name"`
	Cover  string `json:"cover"`
	URL    struct {
		PushURL string `json:"push_url"`
	} `json:"url"`
}

// LivePre returns room_id + push_url (needs robs sid).
func (c *Client) LivePre() (*preData, error) {
	if c.Sid == "" {
		return nil, ErrNeedSID
	}
	path := "/api/sns/live/pre?" + pcQS
	b, err := c.doRobs(http.MethodGet, path, c.Sid, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Result int     `json:"result"`
		Msg    string  `json:"msg"`
		Data   preData `json:"data"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if out.Result != 0 {
		return nil, fmt.Errorf("live/pre: %s (%d)", out.Msg, out.Result)
	}
	if out.Data.RoomID == "" || out.Data.URL.PushURL == "" {
		return nil, fmt.Errorf("live/pre: empty room or push_url")
	}
	return &out.Data, nil
}

// LiveStart starts the room.
func (c *Client) LiveStart(roomID, name, cover string) error {
	if c.Sid == "" {
		return ErrNeedSID
	}
	path := "/api/sns/live/" + url.PathEscape(roomID) + "/start?" + pcQS
	body := map[string]any{
		"name":          name,
		"notice":        "",
		"is_distribute": true,
		"cover":         cover,
		"lesson_id":     0,
	}
	b, err := c.doRobs(http.MethodPost, path, c.Sid, body)
	if err != nil {
		return err
	}
	var out struct {
		Result int    `json:"result"`
		Msg    string `json:"msg"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return err
	}
	if out.Result != 0 {
		return fmt.Errorf("live/start: %s (%d)", out.Msg, out.Result)
	}
	return nil
}

// LiveStop stops the room.
func (c *Client) LiveStop(roomID string) error {
	if c.Sid == "" || roomID == "" {
		return nil
	}
	path := "/api/sns/live/" + url.PathEscape(roomID) + "/stop"
	b, err := c.doRobs(http.MethodPost, path, c.Sid, map[string]any{})
	if err != nil {
		return err
	}
	var out struct {
		Result int    `json:"result"`
		Msg    string `json:"msg"`
	}
	_ = json.Unmarshal(b, &out)
	if out.Result != 0 {
		return fmt.Errorf("live/stop: %s (%d)", out.Msg, out.Result)
	}
	return nil
}

// CheckLive returns whether currently live (best-effort).
func (c *Client) CheckLive() (live bool, roomID string, err error) {
	if c.Sid == "" {
		return false, "", ErrNeedSID
	}
	b, err := c.doRobs(http.MethodGet, "/api/sns/live/check_live", c.Sid, nil)
	if err != nil {
		return false, "", err
	}
	var out struct {
		Result int    `json:"result"`
		Msg    string `json:"msg"`
		Data   struct {
			// field names vary; try common ones
			IsLive bool   `json:"is_live"`
			Living bool   `json:"living"`
			RoomID string `json:"room_id"`
			Status int    `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return false, "", err
	}
	if out.Result != 0 {
		// not logged in etc.
		return false, "", nil
	}
	live = out.Data.IsLive || out.Data.Living || out.Data.Status == 1
	return live, out.Data.RoomID, nil
}

// SplitPushURL splits rtmp://host/app/key?query into server + key.
func SplitPushURL(push string) (server, key string) {
	push = strings.TrimSpace(push)
	if push == "" {
		return "", ""
	}
	// rtmp://live-push.xhscdn.com/live/XXXX?auth=...
	const prefix = "rtmp://"
	if !strings.HasPrefix(push, prefix) && !strings.HasPrefix(push, "rtmps://") {
		return push, ""
	}
	// find path after host
	rest := push
	if i := strings.Index(rest, "://"); i >= 0 {
		rest = rest[i+3:]
	}
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return push, ""
	}
	host := rest[:slash]
	pathq := rest[slash+1:] // live/KEY?q
	scheme := "rtmp://"
	if strings.HasPrefix(push, "rtmps://") {
		scheme = "rtmps://"
	}
	// first path segment is app name (live)
	app, keyPart, ok := strings.Cut(pathq, "/")
	if !ok {
		return scheme + host + "/" + pathq, ""
	}
	server = scheme + host + "/" + app
	key = keyPart
	return server, key
}
