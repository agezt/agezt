// SPDX-License-Identifier: MIT

package app

import (
	"encoding/json"
	"strings"
)

// The lenient readers below are the shared input contract for the migrated
// operations: a flag that accepts a JSON boolean or the strings "true" and
// "1", a count that accepts a number (truncated) or a string of decimal
// digits, and trimmed text. Anything else reads as the zero value rather than
// an error, so a caller that sends a slightly wrong shape gets the default
// instead of a failure — the behavior the native handlers had.

// LenientText reads trimmed text; anything else reads as empty.
func LenientText(raw json.RawMessage) string {
	s, _ := LenientValue(raw).(string)
	return strings.TrimSpace(s)
}

// LenientBool reads a boolean or the strings "true" and "1"; anything else is
// false.
func LenientBool(raw json.RawMessage) bool {
	switch v := LenientValue(raw).(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	default:
		return false
	}
}

// LenientCount reads a number (truncated) or a string of decimal digits;
// anything else reads as zero.
func LenientCount(raw json.RawMessage) int {
	switch v := LenientValue(raw).(type) {
	case float64:
		return int(v)
	case string:
		n := 0
		for _, r := range v {
			if r < '0' || r > '9' {
				return 0
			}
			n = n*10 + int(r-'0')
		}
		return n
	default:
		return 0
	}
}

// LenientValue decodes a raw argument, or nil when there is none.
func LenientValue(raw json.RawMessage) any {
	var v any
	if len(raw) != 0 {
		_ = json.Unmarshal(raw, &v)
	}
	return v
}

// SanitizeCorrelation accepts a client-supplied correlation id only if it's a
// short, plain token ([A-Za-z0-9_-], <=80 chars) — the id becomes a bus subject
// suffix and event field, so we don't let arbitrary text through. Anything else
// returns "" and the caller mints a server-side id instead.
func SanitizeCorrelation(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 80 {
		return ""
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return ""
		}
	}
	return s
}
