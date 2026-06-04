// SPDX-License-Identifier: MIT

package warden

import (
	"bytes"
	"strings"
	"testing"
)

// capBuffer is the memory bound on tool output: a runaway or hostile
// command can emit gigabytes, and we keep only the most-recent `max`
// bytes. The truncation arithmetic has two distinct branches and is
// off-by-one-prone, so these unit tests pin the tail-most-recent
// invariant directly rather than only through warden.Run.

func TestCapBuffer_UnderCapKeepsEverything(t *testing.T) {
	c := newCapBuffer(10)
	n, err := c.Write([]byte("abc"))
	if err != nil || n != 3 {
		t.Fatalf("Write = (%d,%v), want (3,nil)", n, err)
	}
	if got := string(c.Bytes()); got != "abc" {
		t.Errorf("Bytes = %q, want %q", got, "abc")
	}
	if c.Truncated() {
		t.Error("Truncated = true under cap, want false")
	}
}

func TestCapBuffer_ExactFitNotTruncated(t *testing.T) {
	c := newCapBuffer(5)
	c.Write([]byte("abcde")) // len+n == max → fast path
	if c.Truncated() {
		t.Error("an exact-fit write must not be flagged truncated")
	}
	if got := string(c.Bytes()); got != "abcde" {
		t.Errorf("Bytes = %q, want %q", got, "abcde")
	}
}

func TestCapBuffer_SingleWriteLargerThanCapKeepsTail(t *testing.T) {
	c := newCapBuffer(4)
	n, _ := c.Write([]byte("abcdefg")) // n=7 > max=4, buf empty → drop>=len(buf) branch
	if n != 7 {
		t.Errorf("Write returned %d, want 7 (io.Writer must report all bytes consumed)", n)
	}
	if got := string(c.Bytes()); got != "defg" {
		t.Errorf("Bytes = %q, want %q (most-recent tail)", got, "defg")
	}
	if !c.Truncated() {
		t.Error("Truncated = false after dropping data, want true")
	}
}

func TestCapBuffer_SecondWriteCrossingBoundary_DefaultBranch(t *testing.T) {
	// drop < len(buf): keep the tail of the existing buffer, then all of p.
	c := newCapBuffer(5)
	c.Write([]byte("abc"))  // buf="abc"
	c.Write([]byte("defg")) // 3+4=7>5, drop=2 < len(buf)=3 → default branch
	if got := string(c.Bytes()); got != "cdefg" {
		t.Errorf("Bytes = %q, want %q", got, "cdefg")
	}
	if !c.Truncated() {
		t.Error("Truncated = false, want true")
	}
}

func TestCapBuffer_SecondWriteDropsWholeBuffer_DropGEBranch(t *testing.T) {
	// drop >= len(buf): the whole existing buffer plus the head of p go.
	c := newCapBuffer(4)
	c.Write([]byte("ab"))     // buf="ab"
	c.Write([]byte("cdefgh")) // 2+6=8>4, drop=4 >= len(buf)=2 → keep tail of p
	if got := string(c.Bytes()); got != "efgh" {
		t.Errorf("Bytes = %q, want %q", got, "efgh")
	}
}

func TestCapBuffer_EmptyWriteIsNoOp(t *testing.T) {
	c := newCapBuffer(4)
	c.Write([]byte("abcd"))
	n, err := c.Write(nil)
	if n != 0 || err != nil {
		t.Errorf("Write(nil) = (%d,%v), want (0,nil)", n, err)
	}
	if string(c.Bytes()) != "abcd" || c.Truncated() {
		t.Errorf("empty write must not alter state: bytes=%q truncated=%v", c.Bytes(), c.Truncated())
	}
}

func TestCapBuffer_NonPositiveMaxDefaults(t *testing.T) {
	for _, m := range []int{0, -1, -1024} {
		if c := newCapBuffer(m); c.max != DefaultMaxOutputBytes {
			t.Errorf("newCapBuffer(%d).max = %d, want default %d", m, c.max, DefaultMaxOutputBytes)
		}
	}
}

// TestCapBuffer_TailInvariantManyWrites drives both truncation branches
// repeatedly and asserts the buffer always equals the last `max` bytes of
// everything ever written — the property the whole type exists to provide.
func TestCapBuffer_TailInvariantManyWrites(t *testing.T) {
	const max = 16
	c := newCapBuffer(max)
	var full bytes.Buffer
	// Varying chunk sizes (some < max, some > max) exercise the fast path,
	// the default branch, and the drop>=len branch in one sequence.
	chunks := []string{"a", "bb", "ccc", "dddddddddddddddddddd" /*20>max*/, "ee", "f", "gggggggg", "hhhh", "i"}
	for _, ch := range chunks {
		c.Write([]byte(ch))
		full.WriteString(ch)
		want := full.String()
		if len(want) > max {
			want = want[len(want)-max:]
		}
		if got := string(c.Bytes()); got != want {
			t.Fatalf("after writing %q: Bytes = %q, want tail %q", ch, got, want)
		}
		if len(c.Bytes()) > max {
			t.Fatalf("buffer exceeded cap: len=%d max=%d", len(c.Bytes()), max)
		}
	}
	if !c.Truncated() {
		t.Error("Truncated = false after total writes exceeded cap, want true")
	}
}

// TestCapBuffer_NeverExceedsCapUnderHugeWrite is the memory-bound guarantee:
// a single write far larger than the cap retains exactly `max` bytes.
func TestCapBuffer_NeverExceedsCapUnderHugeWrite(t *testing.T) {
	const max = 1024
	c := newCapBuffer(max)
	huge := strings.Repeat("x", 5*max+7)
	c.Write([]byte(huge))
	if len(c.Bytes()) != max {
		t.Errorf("retained %d bytes, want exactly cap %d", len(c.Bytes()), max)
	}
	if !c.Truncated() {
		t.Error("Truncated = false, want true")
	}
}
