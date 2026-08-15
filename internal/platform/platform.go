package platform

import "strings"

// ID is a stable platform identifier (no aliases).
type ID string

const (
	Bilibili    ID = "bilibili"
	Douyin      ID = "douyin"
	XiaoHongShu ID = "xiaohongshu"
)

// All is the ordered list of supported platforms.
var All = []ID{Bilibili, Douyin, XiaoHongShu}

// Parse returns a known platform id (case-insensitive). No aliases.
func Parse(s string) (ID, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, p := range All {
		if string(p) == s {
			return p, true
		}
	}
	return "", false
}

func (p ID) String() string { return string(p) }
