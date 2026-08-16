package xiaohongshu

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/xifan2333/webcast-mate/internal/conv"
)

// CategoryOtherValue is the synthetic select value for non-game ("其他直播").
// Start body: categoryIds = [].
const CategoryOtherValue = "other"

// Category is a flat selectable option for UI (bilibili-style "Parent / Child").
//
// XHS model flattened for one dropdown:
//   - "游戏 / <leaf>"  → value = leaf id → categoryIds:[id]
//   - "其他 / 其他"    → value = CategoryOtherValue → categoryIds:[]
type Category struct {
	ID          string // leaf id, or CategoryOtherValue
	Name        string
	ParentID    string
	ParentName  string
	ContentType int
	Other       bool // true → non-game
}

// Label for select UI: "游戏 / xx" or "其他 / 其他".
func (c Category) Label() string {
	if c.Other || c.ID == CategoryOtherValue {
		return "其他 / 其他"
	}
	leaf := c.Name
	if leaf == "" {
		leaf = c.ID
	}
	return "游戏 / " + leaf
}

// CategoryIDForStart returns leaf id for categoryIds, or "" for other.
func CategoryIDForStart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == CategoryOtherValue {
		return ""
	}
	return value
}

// ListCategories GET …/room/categories?contentType=6, flattened as:
//
//	游戏 / … leaves + 其他 / 其他
func (c *Client) ListCategories(roomID string) ([]Category, error) {
	if c.UserID == "" {
		return nil, fmt.Errorf("no user_id")
	}
	q := url.Values{}
	q.Set("hostId", c.UserID)
	if roomID != "" {
		q.Set("roomId", roomID)
	} else {
		q.Set("roomId", "0")
	}
	q.Set("contentType", "6")
	m, err := c.do(http.MethodGet, hostRedobs,
		"/api/sns/redobs/live/app/v1/room/categories",
		nil, q, doOpts{redobs: true, originRobs: true})
	if err != nil {
		return nil, err
	}
	if !bizOK(m) {
		return nil, fmt.Errorf("categories: %v", m)
	}
	data, _ := m["data"].(map[string]any)
	var roots []any
	if data != nil {
		roots, _ = data["categories"].([]any)
	}
	if roots == nil {
		if arr, ok := m["data"].([]any); ok {
			roots = arr
		}
	}
	if roots == nil && data != nil {
		if arr, ok := data["list"].([]any); ok {
			roots = arr
		}
	}
	var out []Category
	// synthetic first: easy to find via filter "其他"
	out = append(out, Category{
		ID: CategoryOtherValue, Name: "其他", ParentName: "其他", Other: true,
	})
	for _, n := range roots {
		pm, ok := n.(map[string]any)
		if !ok {
			continue
		}
		pid := conv.AnyString(pm["id"])
		pname := conv.FirstNonEmpty(conv.AnyString(pm["name"]), conv.AnyString(pm["title"]))
		subs, _ := pm["subCategories"].([]any)
		if subs == nil {
			subs, _ = pm["sub_categories"].([]any)
		}
		if len(subs) == 0 {
			if pid == "" {
				continue
			}
			out = append(out, Category{
				ID: pid, Name: pname, ParentID: pid, ParentName: pname,
			})
			continue
		}
		for _, s := range subs {
			sm, ok := s.(map[string]any)
			if !ok {
				continue
			}
			id := conv.AnyString(sm["id"])
			if id == "" {
				continue
			}
			name := conv.FirstNonEmpty(conv.AnyString(sm["name"]), conv.AnyString(sm["title"]))
			out = append(out, Category{
				ID: id, Name: name, ParentID: pid, ParentName: pname,
				ContentType: anyIntCT(sm),
			})
		}
	}
	return out, nil
}

func anyIntCT(sm map[string]any) int {
	for _, k := range []string{"contentType", "content_type"} {
		if v, ok := sm[k]; ok && v != nil {
			return conv.AnyInt(v)
		}
	}
	return 0
}

// LastCategoryID GET …/host_last_category → prefer contentType==6 leaf id.
// Empty string means treat as other / unknown.
func (c *Client) LastCategoryID() (string, error) {
	if c.UserID == "" {
		return "", fmt.Errorf("no user_id")
	}
	q := url.Values{}
	q.Set("hostId", c.UserID)
	m, err := c.do(http.MethodGet, hostRedobs,
		"/api/sns/redobs/live/app/v1/room/host_last_category",
		nil, q, doOpts{redobs: true, originRobs: true})
	if err != nil {
		return "", err
	}
	if !bizOK(m) {
		return "", fmt.Errorf("host_last_category: %v", m)
	}
	var cats []any
	if data, ok := m["data"].(map[string]any); ok {
		cats, _ = data["categories"].([]any)
	}
	if cats == nil {
		cats, _ = m["data"].([]any)
	}
	var flat []map[string]any
	for _, n := range cats {
		pm, ok := n.(map[string]any)
		if !ok {
			continue
		}
		subs, _ := pm["subCategories"].([]any)
		if subs == nil {
			subs, _ = pm["sub_categories"].([]any)
		}
		if len(subs) == 0 {
			flat = append(flat, pm)
			continue
		}
		for _, s := range subs {
			if sm, ok := s.(map[string]any); ok {
				flat = append(flat, sm)
			}
		}
	}
	for _, sm := range flat {
		if anyIntCT(sm) == 6 {
			if id := conv.AnyString(sm["id"]); id != "" {
				return id, nil
			}
		}
	}
	if len(flat) > 0 {
		return conv.AnyString(flat[0]["id"]), nil
	}
	return CategoryOtherValue, nil
}
