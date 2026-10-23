// SPDX-License-Identifier: MIT

package reaper

// Reaper small helpers: sameTaskModelChain + plStringAny + plStringsAny +
// plIntAny + parsePositiveInt.

import (
	"encoding/json"
	"strings"
)

// sameTaskModelChain reports whether two model chains are the same, ignoring
// case and surrounding blanks.
func sameTaskModelChain(a, b []string) bool {
	if len(a) != len(b) || len(a) == 0 {
		return false
	}
	for i := range a {
		if !strings.EqualFold(strings.TrimSpace(a[i]), strings.TrimSpace(b[i])) {
			return false
		}
	}
	return true
}

// plStringAny reads a string out of a decoded payload.
func plStringAny(v any) string {
	s, _ := v.(string)
	return s
}

// plStringsAny reads a list of non-empty strings out of a decoded payload.
func plStringsAny(v any) []string {
	raw, ok := v.([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// plIntAny reads an integer out of a decoded payload, whatever shape it
// arrived in.
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

// parsePositiveInt reads a positive integer out of an environment value. Any
// character that is not a digit makes the whole value invalid.
func parsePositiveInt(raw string) int {
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
