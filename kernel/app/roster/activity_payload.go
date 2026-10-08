// SPDX-License-Identifier: MIT

package roster

// Journal payload accessors for activity text; copies of the native roster
// helpers so the application owns its decoding without importing controlplane.

import (
	"strings"

	"github.com/agezt/agezt/internal/strutil"
)

func plString(pl map[string]any, key string) string {
	s, _ := pl[key].(string)
	return s
}

func plInt(pl map[string]any, key string) int {
	switch n := pl[key].(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func plStrings(pl map[string]any, key string) []string {
	raw, ok := pl[key].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func truncate(s string, n int) string {
	return strutil.Ellipsis(strings.TrimSpace(s), n, "…")
}

func firstNonEmpty(items ...string) string { return strutil.FirstNonEmpty(items...) }

func intNumber(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
