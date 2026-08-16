package bilibili

import (
	"encoding/json"
	"fmt"
	"strconv"
)

const urlAreaList = "https://api.live.bilibili.com/room/v1/Area/getList?show_pinyin=1"

// Area is a secondary (leaf) live partition.
type Area struct {
	ID         string
	Name       string
	ParentName string
}

// ListAreas fetches leaf areas for the select UI.
func (c *Client) ListAreas() ([]Area, error) {
	b, err := c.doJSON("GET", urlAreaList, nil, nil)
	if err != nil {
		return nil, err
	}
	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    []struct {
			ID   any    `json:"id"`
			Name string `json:"name"`
			List []struct {
				ID         any    `json:"id"`
				Name       string `json:"name"`
				ParentName string `json:"parent_name"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("area list: %s (%d)", env.Message, env.Code)
	}
	var out []Area
	for _, p := range env.Data {
		for _, ch := range p.List {
			id := anyToString(ch.ID)
			if id == "" {
				continue
			}
			parent := ch.ParentName
			if parent == "" {
				parent = p.Name
			}
			out = append(out, Area{ID: id, Name: ch.Name, ParentName: parent})
		}
	}
	return out, nil
}

func anyToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case json.Number:
		return t.String()
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		return fmt.Sprint(t)
	}
}
