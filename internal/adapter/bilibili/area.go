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
	ParentID   string
	ParentName string
}

// AreaParent is a top-level partition (网游 / 手游 / 娱乐 …).
type AreaParent struct {
	ID   string
	Name string
}

// AreaTree is the two-level partition list for UI.
type AreaTree struct {
	Parents  []AreaParent
	Children map[string][]Area // parentID → leaves
}

// ListAreaTree fetches parent + child partitions.
func (c *Client) ListAreaTree() (*AreaTree, error) {
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
				ParentID   any    `json:"parent_id"`
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

	tree := &AreaTree{
		Children: make(map[string][]Area),
	}
	for _, p := range env.Data {
		pid := anyToString(p.ID)
		if pid == "" {
			continue
		}
		tree.Parents = append(tree.Parents, AreaParent{ID: pid, Name: p.Name})
		for _, ch := range p.List {
			id := anyToString(ch.ID)
			if id == "" {
				continue
			}
			parentName := ch.ParentName
			if parentName == "" {
				parentName = p.Name
			}
			tree.Children[pid] = append(tree.Children[pid], Area{
				ID:         id,
				Name:       ch.Name,
				ParentID:   pid,
				ParentName: parentName,
			})
		}
	}
	return tree, nil
}

// ListAreas is a flat leaf list (legacy).
func (c *Client) ListAreas() ([]Area, error) {
	tree, err := c.ListAreaTree()
	if err != nil {
		return nil, err
	}
	var out []Area
	for _, p := range tree.Parents {
		out = append(out, tree.Children[p.ID]...)
	}
	return out, nil
}

// FindParentOf returns parent id for a leaf area id.
func (t *AreaTree) FindParentOf(leafID string) string {
	if t == nil {
		return ""
	}
	for pid, kids := range t.Children {
		for _, k := range kids {
			if k.ID == leafID {
				return pid
			}
		}
	}
	return ""
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
