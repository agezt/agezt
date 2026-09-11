// SPDX-License-Identifier: MIT

package selfrepair

// Pure payload helpers used by the self-repair coordinator. Carved out of
// selfrepair.go during the Day 25 god file split so the main file can
// focus on the coordinator state machine.

import (
	"encoding/json"
	"strings"
)

func plStringMap(pl map[string]any, key string) string {
	if pl == nil {
		return ""
	}
	s, _ := pl[key].(string)
	return strings.TrimSpace(s)
}

func plStringsMap(pl map[string]any, key string) []string {
	raw, ok := pl[key].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func plIntAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

