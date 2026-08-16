package xiaohongshu

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// ObsPush is response from /web_api/sns/v1/live/obs/push_url?code=
type ObsPush struct {
	RoomID  string
	PushURL string
	Server  string
	Key     string
}

// FetchObsPushURL exchanges the phone 6-digit OBS code for RTMP push URL.
// Requires logged-in web_session (QR). Host: www.xiaohongshu.com, appid spectrum.
func (c *Client) FetchObsPushURL(code string) (*ObsPush, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("empty obs code")
	}
	uri := "/web_api/sns/v1/live/obs/push_url"
	q := url.Values{}
	q.Set("code", code)
	b, err := c.doSpectrumGET(uri, q)
	if err != nil {
		return nil, err
	}
	var out struct {
		Code    int    `json:"code"`
		Success bool   `json:"success"`
		Msg     string `json:"msg"`
		Data    struct {
			// field names may vary — collect common ones
			PushURL string `json:"push_url"`
			URL     string `json:"url"`
			RoomID  any    `json:"room_id"`
			// nested
			Stream struct {
				PushURL string `json:"push_url"`
			} `json:"stream"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if out.Code != 0 && !out.Success {
		return nil, fmt.Errorf("push_url: %s (%d)", out.Msg, out.Code)
	}
	// also parse loosely
	var raw map[string]any
	_ = json.Unmarshal(b, &raw)
	push := out.Data.PushURL
	if push == "" {
		push = out.Data.URL
	}
	if push == "" {
		push = out.Data.Stream.PushURL
	}
	if push == "" {
		if data, ok := raw["data"].(map[string]any); ok {
			for _, k := range []string{"push_url", "url", "rtmp_url", "pushUrl"} {
				if s, ok := data[k].(string); ok && strings.Contains(s, "rtmp") {
					push = s
					break
				}
			}
			// data.url might be object
			if push == "" {
				if u, ok := data["url"].(map[string]any); ok {
					if s, ok := u["push_url"].(string); ok {
						push = s
					}
				}
			}
		}
	}
	if push == "" {
		return nil, fmt.Errorf("push_url: no rtmp in response: %s", truncate(string(b), 300))
	}
	roomID := fmt.Sprint(out.Data.RoomID)
	if roomID == "" || roomID == "<nil>" {
		if data, ok := raw["data"].(map[string]any); ok {
			if r, ok := data["room_id"]; ok {
				roomID = fmt.Sprint(r)
			}
		}
	}
	server, key := SplitPushURL(push)
	return &ObsPush{RoomID: roomID, PushURL: push, Server: server, Key: key}, nil
}

// LivingPushURL tries GET living_push_url when already live.
func (c *Client) LivingPushURL() (*ObsPush, error) {
	uri := "/web_api/sns/v1/live/obs/living_push_url"
	b, err := c.doSpectrumGET(uri, nil)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	code, _ := raw["code"].(float64)
	if code != 0 {
		msg, _ := raw["msg"].(string)
		return nil, fmt.Errorf("living_push_url: %s (%v)", msg, code)
	}
	// reuse parser via wrapping
	return parsePushPayload(b)
}

func parsePushPayload(b []byte) (*ObsPush, error) {
	var c Client
	// fake by reusing FetchObsPushURL parser — duplicate minimal
	var raw map[string]any
	_ = json.Unmarshal(b, &raw)
	data, _ := raw["data"].(map[string]any)
	if data == nil {
		return nil, fmt.Errorf("no data: %s", truncate(string(b), 200))
	}
	push := ""
	for _, k := range []string{"push_url", "url", "rtmp_url"} {
		if s, ok := data[k].(string); ok && strings.Contains(s, "rtmp") {
			push = s
			break
		}
	}
	if push == "" {
		if u, ok := data["url"].(map[string]any); ok {
			if s, ok := u["push_url"].(string); ok {
				push = s
			}
		}
	}
	if push == "" {
		return nil, fmt.Errorf("no push_url: %s", truncate(string(b), 200))
	}
	roomID := ""
	if r, ok := data["room_id"]; ok {
		roomID = fmt.Sprint(r)
	}
	server, key := SplitPushURL(push)
	_ = c
	return &ObsPush{RoomID: roomID, PushURL: push, Server: server, Key: key}, nil
}

// SplitPushURL splits rtmp://host/app/key?query into server + key.
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
