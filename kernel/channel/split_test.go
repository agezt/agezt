// SPDX-License-Identifier: MIT

package channel

import (
	"strings"
	"testing"
	"unicode/utf16"
)

// units counts UTF-16 code units, the limit SplitText enforces.
func units(s string) int { return len(utf16.Encode([]rune(s))) }

func TestSplitText_WithinLimitUnchanged(t *testing.T) {
	got := SplitText("hello world", 4096)
	if len(got) != 1 || got[0] != "hello world" {
		t.Fatalf("got %v, want single unchanged piece", got)
	}
}

func TestSplitText_EveryPieceWithinLimit(t *testing.T) {
	text := strings.Repeat("abcdefghij ", 1000) // ~11k chars
	for _, limit := range []int{10, 100, 2000, 4096} {
		pieces := SplitText(text, limit)
		if len(pieces) < 2 {
			t.Errorf("limit %d: expected multiple pieces, got %d", limit, len(pieces))
		}
		for i, p := range pieces {
			if units(p) > limit {
				t.Errorf("limit %d: piece %d is %d units (over)", limit, i, units(p))
			}
		}
	}
}

// The core invariant: no characters are added or dropped.
func TestSplitText_LosslessRejoin(t *testing.T) {
	inputs := []string{
		strings.Repeat("word ", 2000),
		"line one\nline two\n" + strings.Repeat("x", 5000) + "\ntail",
		strings.Repeat("nospaceswhatsoever", 500),
		"短い文 " + strings.Repeat("日本語のテキスト ", 800), // multibyte
	}
	for _, in := range inputs {
		got := strings.Join(SplitText(in, 1000), "")
		if got != in {
			t.Errorf("rejoin mismatch: len(in)=%d len(out)=%d", len(in), len(got))
		}
	}
}

func TestSplitText_PrefersBoundaries(t *testing.T) {
	// Lines that each fit; SplitText should break at the newlines, not mid-line.
	text := strings.Repeat("a", 30) + "\n" + strings.Repeat("b", 30) + "\n" + strings.Repeat("c", 30)
	pieces := SplitText(text, 35)
	for _, p := range pieces {
		// No piece should contain two different letters jammed together past a
		// boundary that fit — i.e. a break happened at a newline.
		if strings.Contains(p, "a") && strings.Contains(p, "b") && strings.Contains(p, "c") {
			t.Errorf("piece spans all blocks, boundary not used: %q", p)
		}
	}
	if strings.Join(pieces, "") != text {
		t.Error("boundary split lost characters")
	}
}

func TestSplitText_EmojiCountAsTwoUnits(t *testing.T) {
	// Each 😀 is 1 rune but 2 UTF-16 units; with limit 4, only two fit per piece.
	got := SplitText("😀😀😀😀😀", 4)
	for i, p := range got {
		if units(p) > 4 {
			t.Errorf("piece %d = %d units, over limit 4", i, units(p))
		}
	}
	if strings.Join(got, "") != "😀😀😀😀😀" {
		t.Error("emoji split lost characters")
	}
}

func TestSplitText_LongWordHardCut(t *testing.T) {
	// A single 100-char token with no boundary must still be cut into ≤limit pieces.
	text := strings.Repeat("Z", 100)
	got := SplitText(text, 25)
	if len(got) != 4 {
		t.Fatalf("got %d pieces, want 4", len(got))
	}
	for _, p := range got {
		if len([]rune(p)) > 25 {
			t.Errorf("hard-cut piece over limit: %d", len([]rune(p)))
		}
	}
}

func TestSplitText_NonPositiveLimit(t *testing.T) {
	if got := SplitText("anything", 0); len(got) != 1 || got[0] != "anything" {
		t.Errorf("limit 0 should return text unsplit, got %v", got)
	}
}

// TestSplitText_NeverEmptyPiece pins the empty-buffer cut guard. When a single
// character is wider than the limit (e.g. a 2-unit emoji at limit 1) the char can't
// be made to fit, so SplitText emits it as its own over-limit piece — but it must
// never emit an EMPTY piece, which would make a channel send a blank message (some
// platforms reject those). The `len(cur) > 0` guard on the cut is what prevents a cut
// against an empty buffer; mutation testing (M511) showed `> 0 → >= 0` survived because
// no test used a sub-character limit (the emoji test uses limit 4, where each char
// fits). Under the mutant `SplitText("😀😀", 1)` yields a leading "" piece.
func TestSplitText_NeverEmptyPiece(t *testing.T) {
	cases := []struct {
		text  string
		limit int
	}{
		{"😀😀", 1},    // each emoji is 2 units, limit 1 — char wider than the limit
		{"ab😀cd", 1}, // mix of fitting and over-wide chars
		{"日本", 1},    // CJK, each 1 unit but exercises the same path at limit boundaries
	}
	for _, c := range cases {
		got := SplitText(c.text, c.limit)
		for i, p := range got {
			if p == "" {
				t.Errorf("SplitText(%q, %d): piece %d is empty (would send a blank message)", c.text, c.limit, i)
			}
		}
		if strings.Join(got, "") != c.text {
			t.Errorf("SplitText(%q, %d): rejoin mismatch", c.text, c.limit)
		}
	}
}
