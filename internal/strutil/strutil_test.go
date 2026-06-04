// SPDX-License-Identifier: MIT

package strutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEllipsis_UnderAndExactCapUnchanged(t *testing.T) {
	for _, s := range []string{"", "short", "exactly-ten"} {
		if got := Ellipsis(s, len(s), "…"); got != s {
			t.Errorf("Ellipsis(%q, %d) = %q, want unchanged", s, len(s), got)
		}
	}
	if got := Ellipsis("abc", 10, "…"); got != "abc" {
		t.Errorf("under cap = %q, want %q", got, "abc")
	}
}

func TestEllipsis_ASCIITruncatesWithMarker(t *testing.T) {
	got := Ellipsis("hello world", 5, "…")
	if got != "hello…" {
		t.Errorf("got %q, want %q", got, "hello…")
	}
	// Marker is parametrised so each call site keeps its own style.
	if got := Ellipsis("hello world", 5, "..."); got != "hello..." {
		t.Errorf("got %q, want %q", got, "hello...")
	}
}

func TestEllipsis_RuneSafeAtByteBoundary(t *testing.T) {
	// "aş" where 'ş' (U+015F) is 2 bytes: "a"(1) + C5 9F. maxBytes=2 lands on the
	// 0x9F continuation byte — a raw slice would leave a lone C5 (invalid UTF-8).
	got := Ellipsis("aşb", 2, "…")
	if !utf8.ValidString(got) {
		t.Fatalf("Ellipsis produced invalid UTF-8: %q", got)
	}
	if got != "a…" {
		t.Errorf("got %q, want %q (straddling rune dropped whole)", got, "a…")
	}
}

func TestEllipsis_AllMultiByteOddCap(t *testing.T) {
	in := strings.Repeat("ş", 100) // 200 bytes
	got := Ellipsis(in, 51, "…")   // odd → cut inside a rune
	if !utf8.ValidString(got) {
		t.Fatalf("invalid UTF-8: %q", got)
	}
	if strings.ContainsRune(got, '�') {
		t.Errorf("contains replacement char: %q", got)
	}
	// 51 bytes → 25 whole 2-byte runes (50 bytes) kept + marker.
	if rc := utf8.RuneCountInString(strings.TrimSuffix(got, "…")); rc != 25 {
		t.Errorf("kept %d runes, want 25", rc)
	}
}

func TestEllipsis_NonPositiveMax(t *testing.T) {
	if got := Ellipsis("abc", 0, "…"); got != "…" {
		t.Errorf("max 0 = %q, want just the marker", got)
	}
	if got := Ellipsis("abc", -5, "…"); got != "…" {
		t.Errorf("negative max = %q, want just the marker", got)
	}
}
