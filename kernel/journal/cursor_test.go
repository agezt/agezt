// SPDX-License-Identifier: MIT

package journal

import "testing"

func TestEncodeDecodeCursorRoundTrip(t *testing.T) {
	cases := []struct{ ms, seq int64 }{
		{1, 1},
		{1_700_000_000_000, 42},
		{9_999_999_999_999, 0},
		{0, 7},
	}
	for _, c := range cases {
		tok := EncodeCursor(c.ms, c.seq)
		gotMS, gotSeq, ok := DecodeCursor(tok)
		if !ok {
			t.Fatalf("EncodeCursor(%d,%d)=%q did not round-trip (ok=false)", c.ms, c.seq, tok)
		}
		if gotMS != c.ms || gotSeq != c.seq {
			t.Fatalf("round-trip mismatch: encoded (%d,%d) -> %q -> (%d,%d)", c.ms, c.seq, tok, gotMS, gotSeq)
		}
	}
}

func TestEncodeCursorZeroIsEmpty(t *testing.T) {
	if got := EncodeCursor(0, 0); got != "" {
		t.Fatalf("EncodeCursor(0,0) = %q, want empty string", got)
	}
	// And the empty string decodes as absent.
	if _, _, ok := DecodeCursor(""); ok {
		t.Fatalf("DecodeCursor(\"\") returned ok=true, want absent")
	}
}

func TestDecodeCursorMalformedFallsBack(t *testing.T) {
	// Every one of these must be treated as "no cursor" (ok=false), not an error.
	bad := []any{
		nil,             // absent
		"",              // empty
		"abc",           // no colon
		":",             // empty halves
		":5",            // empty ms
		"5:",            // empty seq
		"x:5",           // non-numeric ms
		"5:x",           // non-numeric seq
		"-1:5",          // negative ms (forged/aged-out)
		"5:-1",          // negative seq
		42,              // non-string arg
		"1700000000000", // missing colon
	}
	for _, b := range bad {
		if ms, seq, ok := DecodeCursor(b); ok {
			t.Fatalf("DecodeCursor(%v) = (%d,%d,true), want ok=false", b, ms, seq)
		}
	}
}

func TestDecodeCursorToleratesLargeValues(t *testing.T) {
	// Real journal cursors carry 13-digit ms + multi-digit seq; ensure no overflow.
	ms, seq, ok := DecodeCursor("1735689600000:1234567")
	if !ok || ms != 1_735_689_600_000 || seq != 1_234_567 {
		t.Fatalf("DecodeCursor large = (%d,%d,%v)", ms, seq, ok)
	}
}

// TestKeepBeforeCursor pins the direction-sensitive filter. This is the exact
// semantics that caught a real bug in the /api/runs pager: without the >=
// equality clause the cursor's own row re-appears at the top of the next page.
func TestKeepBeforeCursor(t *testing.T) {
	const cMS, cSeq = 100, 5
	cases := []struct {
		name          string
		rowMS, rowSeq int64
		keep          bool
	}{
		{"newer ms dropped", 101, 0, false},
		{"same ms newer seq dropped", 100, 6, false},
		{"same ms equal seq dropped (the cursor row itself)", 100, 5, false},
		{"same ms older seq kept", 100, 4, true},
		{"older ms kept regardless of seq", 99, 999, true},
		{"much older kept", 1, 1, true},
	}
	for _, c := range cases {
		if got := KeepBeforeCursor(c.rowMS, c.rowSeq, cMS, cSeq); got != c.keep {
			t.Errorf("%s: KeepBeforeCursor(%d,%d,%d,%d)=%v, want %v",
				c.name, c.rowMS, c.rowSeq, cMS, cSeq, got, c.keep)
		}
	}
}

func TestNextCursor(t *testing.T) {
	// Full page (emitted == limit) -> advertise the last row's cursor.
	if got := NextCursor(100, 5, 20, 20); got != "100:5" {
		t.Fatalf("full page NextCursor = %q, want 100:5", got)
	}
	// Short page (emitted < limit) -> terminal, no cursor.
	if got := NextCursor(100, 5, 7, 20); got != "" {
		t.Fatalf("short page NextCursor = %q, want empty", got)
	}
	// Empty page -> no cursor.
	if got := NextCursor(0, 0, 0, 20); got != "" {
		t.Fatalf("empty page NextCursor = %q, want empty", got)
	}
}
