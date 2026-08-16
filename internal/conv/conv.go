// Package conv holds small shared converters used by adapters.
package conv

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// AnyString coerces JSON-ish values to string.
func AnyString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%.0f", t)
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	case json.Number:
		return t.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

// AnyInt coerces JSON-ish values to int.
func AnyInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	case string:
		var n int
		fmt.Sscanf(strings.TrimSpace(t), "%d", &n)
		return n
	default:
		return 0
	}
}

// AnyBool coerces JSON-ish values to bool.
func AnyBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case int:
		return t != 0
	case string:
		return t == "1" || strings.EqualFold(t, "true")
	default:
		return false
	}
}

// Truncate shortens s to at most n runes with an ellipsis (rune-safe).
func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// FirstString returns the first non-empty AnyString among keys.
func FirstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := AnyString(m[k]); s != "" {
			return s
		}
	}
	return ""
}

// FirstArray returns the first []any among keys.
func FirstArray(m map[string]any, keys ...string) []any {
	for _, k := range keys {
		if a, ok := m[k].([]any); ok {
			return a
		}
	}
	return nil
}

// FirstNonEmpty returns the first non-blank string.
func FirstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// IsInteractive reports whether stdin is a terminal.
func IsInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
