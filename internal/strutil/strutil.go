// SPDX-License-Identifier: MIT

// Package strutil holds small, dependency-free string helpers shared across the
// daemon and CLI. It exists so a fix in one place (e.g. rune-safe truncation)
// covers every call site rather than being re-implemented — and occasionally
// re-broken — per package.
package strutil

import (
	"strings"
	"unicode/utf8"
)

// Ellipsis returns s unchanged when it fits within maxBytes, otherwise a prefix
// cut on a UTF-8 rune boundary (never splitting a multi-byte rune) with marker
// appended. maxBytes bounds the retained PREFIX in bytes; the marker is extra.
//
// Truncating with a raw s[:maxBytes] slice splits a multi-byte rune whenever the
// cut lands mid-character, emitting invalid UTF-8 — a real hazard for non-ASCII
// text (e.g. Turkish ç/ş/ğ, or arbitrary fetched web content). Ellipsis backs the
// cut up to the start of the straddling rune and drops it whole.
//
// A non-positive maxBytes yields just the marker for an over-length string.
func Ellipsis(s string, maxBytes int, marker string) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	if cut < 0 {
		cut = 0
	}
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + marker
}

// FirstNonEmpty returns the first item that is non-empty after trimming
// whitespace, trimmed. Empty string when none qualifies.
func FirstNonEmpty(items ...string) string {
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			return item
		}
	}
	return ""
}

// FirstNonEmptySlice returns primary when it has any elements, else fallback.
func FirstNonEmptySlice(primary, fallback []string) []string {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}
