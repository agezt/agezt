// SPDX-License-Identifier: MIT

package controlplane

// White-box test for the bounded control-plane request reader (M188).

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadBoundedLine_Request(t *testing.T) {
	// Under cap: a normal terminated line round-trips.
	r := bufio.NewReader(strings.NewReader("hello\n"))
	got, err := readBoundedLine(r, 1024)
	if err != nil || string(got) != "hello\n" {
		t.Fatalf("under cap: got %q err %v", got, err)
	}

	// Overflow: an unterminated flood larger than the cap is rejected
	// (rather than allocating without bound).
	big := bufio.NewReader(bytes.NewReader(bytes.Repeat([]byte("x"), 5000)))
	if _, err := readBoundedLine(big, 1000); !errors.Is(err, errRequestTooLarge) {
		t.Errorf("overflow err = %v want errRequestTooLarge", err)
	}

	// EOF mid-line returns the partial bytes with io.EOF.
	r2 := bufio.NewReader(strings.NewReader("partial"))
	if g, err := readBoundedLine(r2, 1024); err != io.EOF || string(g) != "partial" {
		t.Errorf("eof-mid-line: got %q err %v", g, err)
	}
}

// TestReadBoundedLine_MultiChunkReassembly exercises the bufio.ErrBufferFull
// accumulation path: a line LONGER than the reader's buffer but UNDER the cap
// must be reassembled whole across multiple ReadSlice chunks, not truncated at
// a buffer boundary. This is the trickiest branch of readBoundedLine (the
// `continue` on ErrBufferFull, copying each chunk out so the returned slice is
// stable) and the one a real >4 KiB control-plane request would hit.
func TestReadBoundedLine_MultiChunkReassembly(t *testing.T) {
	// 16-byte buffer (bufio's minimum) forces ReadSlice to return ErrBufferFull
	// repeatedly for a 100-byte line; cap of 1024 leaves it well under the bound.
	line := strings.Repeat("a", 100)
	r := bufio.NewReaderSize(strings.NewReader(line+"\n"), 16)
	got, err := readBoundedLine(r, 1024)
	if err != nil {
		t.Fatalf("multi-chunk line should reassemble cleanly, got err %v", err)
	}
	if string(got) != line+"\n" {
		t.Errorf("reassembled line = %q (len %d), want the full %d-byte line + newline",
			got, len(got), len(line))
	}
}

// TestReadBoundedLine_ExactlyMaxAccepted pins the inclusive upper bound of the M188
// pre-auth DoS guard: a request whose length (including the trailing newline) is EXACTLY
// max is accepted — the limit is inclusive — while max+1 is rejected. The existing tests
// cover only under-cap and a flood well over it, so `len(buf)+len(chunk) > max` was unpinned
// at the boundary (mutation M531: `> → >=` would reject a request that exactly fills the cap).
func TestReadBoundedLine_ExactlyMaxAccepted(t *testing.T) {
	const max = 64
	exact := strings.Repeat("a", max-1) + "\n" // 64 bytes total, including the newline
	got, err := readBoundedLine(bufio.NewReader(strings.NewReader(exact)), max)
	if err != nil {
		t.Fatalf("a request of exactly max=%d bytes must be accepted (inclusive limit), got err %v", max, err)
	}
	if len(got) != max {
		t.Errorf("got %d bytes, want %d", len(got), max)
	}
	over := strings.Repeat("a", max) + "\n" // max+1 bytes total
	if _, err := readBoundedLine(bufio.NewReader(strings.NewReader(over)), max); !errors.Is(err, errRequestTooLarge) {
		t.Errorf("a request of max+1 bytes must be rejected, got err %v", err)
	}
}
