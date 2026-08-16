package douyin

import (
	"fmt"
	"strings"

	"github.com/xifan2333/webcast-mate/internal/conv"
)

// Category is a leaf live partition (flat list for UI, same style as bilibili).
// Create body: BaseID → base_category, ID → category.
type Category struct {
	ID       string // leaf category_id
	Name     string
	BaseID   string
	BaseName string
	Type     string
	Other    bool
}

// Label for select UI: "Parent / Child".
func (c Category) Label() string {
	if c.BaseName != "" && c.BaseName != c.Name {
		return c.BaseName + " / " + c.Name
	}
	if c.Name != "" {
		return c.Name
	}
	return c.ID
}

// EncodeValue stores base|leaf for config.
func (c Category) EncodeValue() string {
	if c.BaseID == "" || c.BaseID == c.ID {
		return c.ID
	}
	return c.BaseID + "|" + c.ID
}

// ParseCategoryValue splits "base|leaf" or bare leaf.
func ParseCategoryValue(s string) (base, leaf string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if a, b, ok := strings.Cut(s, "|"); ok {
		return strings.TrimSpace(a), strings.TrimSpace(b)
	}
	return "", s
}

// ListCategories GET /webcast/room/get_all_category/?platform=1
// Returns flat leaves labeled "Parent / Child" (no cascade UI).
func (c *Client) ListCategories() ([]Category, error) {
	q := c.commonQuery()
	q.Set("platform", "1")
	qs, err := withABogus(q.Encode(), "")
	if err != nil {
		return nil, err
	}
	m, err := c.getJSON(hostAPI+"/webcast/room/get_all_category/?"+qs, nil)
	if err != nil {
		return nil, err
	}
	if sc := conv.AnyInt(m["status_code"]); sc != 0 {
		msg := conv.AnyString(m["status_msg"])
		if msg == "" {
			msg = fmt.Sprint(m["data"])
		}
		return nil, fmt.Errorf("get_all_category: status_code=%d %s", sc, msg)
	}
	roots := extractCategoryRoots(m["data"])
	if len(roots) == 0 {
		return nil, fmt.Errorf("get_all_category: empty list: %s", conv.Truncate(fmt.Sprint(m), 240))
	}
	var out []Category
	walkDYCategories(roots, "", "", &out)
	return out, nil
}

func extractCategoryRoots(data any) []any {
	switch d := data.(type) {
	case []any:
		return d
	case map[string]any:
		for _, k := range []string{"category_list", "categories", "categorys", "list", "data"} {
			if arr, ok := d[k].([]any); ok {
				return arr
			}
		}
	}
	return nil
}

func walkDYCategories(nodes []any, parentID, parentName string, out *[]Category) {
	for _, n := range nodes {
		m, ok := n.(map[string]any)
		if !ok {
			continue
		}
		id := conv.FirstString(m, "category_id", "id", "value")
		name := conv.FirstString(m, "title", "name", "label")
		typ := conv.FirstString(m, "show_type", "type")
		other := conv.AnyBool(m["is_other_category"]) || conv.AnyBool(m["other"])
		subs := conv.FirstArray(m, "sub_categorys", "sub_categories", "children", "list")
		if len(subs) == 0 {
			if id == "" {
				continue
			}
			baseID, baseName := parentID, parentName
			if baseID == "" {
				baseID, baseName = id, name
			}
			*out = append(*out, Category{
				ID: id, Name: name, BaseID: baseID, BaseName: baseName, Type: typ, Other: other,
			})
			continue
		}
		// has children
		nextBaseID, nextBaseName := id, name
		if parentID != "" {
			// deeper than 2: keep original base
			nextBaseID, nextBaseName = parentID, parentName
		}
		walkDYCategories(subs, nextBaseID, nextBaseName, out)
	}
}
