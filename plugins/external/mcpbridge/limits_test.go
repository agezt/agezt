// SPDX-License-Identifier: MIT

package main

// Tests for the bounded MCP-server frame reader (M185) — the guard that
// stops an untrusted MCP server (stdio child or remote SSE stream) from
// OOM-ing the bridge with an unterminated or huge frame.

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadBoundedLine_NormalFrames(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("a\nbb\nccc\n"))
	for _, want := range []string{"a\n", "bb\n", "ccc\n"} {
		got, err := readBoundedLine(r, 1024)
		if err != nil {
			t.Fatalf("unexpected err %v", err)
		}
		if string(got) != want {
			t.Errorf("frame = %q want %q", got, want)
		}
	}
	if _, err := readBoundedLine(r, 1024); err != io.EOF {
		t.Errorf("final err = %v want io.EOF", err)
	}
}

func TestReadBoundedLine_MultiChunkUnderMax(t *testing.T) {
	long := strings.Repeat("x", 10000) // exceeds bufio's 4 KiB buffer
	r := bufio.NewReader(strings.NewReader(long + "\n"))
	got, err := readBoundedLine(r, 1<<20)
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if string(got) != long+"\n" {
		t.Errorf("multi-chunk frame truncated: %d bytes", len(got))
	}
}

func TestReadBoundedLine_OverflowRejected(t *testing.T) {
	cases := []struct {
		name string
		line int
		max  int
	}{
		{"small-cap", 200, 50},
		{"across-chunks", 50000, 4096},
		{"unterminated-flood", 1 << 20, 64 << 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := bufio.NewReader(bytes.NewReader(bytes.Repeat([]byte("a"), tc.line)))
			if _, err := readBoundedLine(r, tc.max); !errors.Is(err, errMCPFrameTooLarge) {
				t.Errorf("err = %v want errMCPFrameTooLarge", err)
			}
		})
	}
}

// TestReadBoundedLine_ExactlyMaxAccepted pins the inclusive upper bound: a frame whose
// total length (including the trailing newline) is EXACTLY max is accepted, while max+1 is
// rejected. The other tests cover under-max and over-max floods, so the
// `len(buf)+len(chunk) > max` edge was unpinned — mutation testing (M538) showed `> → >=`
// would reject a frame that exactly fills the cap (same class as plugin readFrame M509 and
// control-plane readBoundedLine M531).
func TestReadBoundedLine_ExactlyMaxAccepted(t *testing.T) {
	const max = 64
	exact := strings.Repeat("a", max-1) + "\n" // 64 bytes total, including the newline
	got, err := readBoundedLine(bufio.NewReader(strings.NewReader(exact)), max)
	if err != nil {
		t.Fatalf("a frame of exactly max=%d bytes must be accepted (inclusive limit), got err %v", max, err)
	}
	if len(got) != max {
		t.Errorf("got %d bytes, want %d", len(got), max)
	}
	over := strings.Repeat("a", max) + "\n" // max+1 bytes
	if _, err := readBoundedLine(bufio.NewReader(strings.NewReader(over)), max); !errors.Is(err, errMCPFrameTooLarge) {
		t.Errorf("a frame of max+1 bytes must be rejected, got err %v", err)
	}
}

func TestReadBoundedLine_EOFMidLine(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("partial"))
	got, err := readBoundedLine(r, 1024)
	if err != io.EOF {
		t.Errorf("err = %v want io.EOF", err)
	}
	if string(got) != "partial" {
		t.Errorf("got %q want partial", got)
	}
}
