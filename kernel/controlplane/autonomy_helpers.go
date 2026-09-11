// SPDX-License-Identifier: MIT

// Autonomy feed helpers: strPayloadMap, strPayload, strSlicePayload, intPayload, clipDetail.
// Code extracted from autonomy.go during the Day-50 god-file split. Public API unchanged.
package controlplane


import (
	"encoding/json"
	"strings"
)


func strPayloadMap(payload []byte, k string) string {
	if len(payload) == 0 {
		return ""
	}
	var p map[string]any
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return strPayload(p, k)
}

func strPayload(p map[string]any, k string) string {
	if v, ok := p[k].(string); ok {
		return v
	}
	return ""
}

func strSlicePayload(p map[string]any, k string) []string {
	raw, ok := p[k].([]any)
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

func intPayload(p map[string]any, k string) int {
	switch n := p[k].(type) {
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

func clipDetail(s string) string {
	const max = 120
	r := []rune(s)
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}
