// SPDX-License-Identifier: MIT

package plugin

// White-box tests for the bounded stdout frame reader (M177): the guard
// that stops an untrusted plugin from OOM-ing the daemon by writing a
// frame that never ends (or is pathologically large).

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadFrame_NormalFrames(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("one\ntwo\nthree\n"))
	for _, want := range []string{"one\n", "two\n", "three\n"} {
		got, err := readFrame(r, 1024)
		if err != nil {
			t.Fatalf("readFrame: unexpected err %v", err)
		}
		if string(got) != want {
			t.Errorf("frame = %q want %q", got, want)
		}
	}
	if _, err := readFrame(r, 1024); err != io.EOF {
		t.Errorf("final read err = %v want io.EOF", err)
	}
}

// A line longer than bufio's internal buffer (4096) must still be
// returned whole when it's under max — exercises the ErrBufferFull
// chunk-accumulation path, not just single-shot reads.
func TestReadFrame_MultiChunkUnderMax(t *testing.T) {
	long := strings.Repeat("x", 10000)
	r := bufio.NewReader(strings.NewReader(long + "\n"))
	got, err := readFrame(r, 1<<20)
	if err != nil {
		t.Fatalf("readFrame: unexpected err %v", err)
	}
	if string(got) != long+"\n" {
		t.Errorf("multi-chunk frame truncated: got %d bytes want %d", len(got), len(long)+1)
	}
}

// The core OOM guard: a frame over max yields errFrameTooLarge rather
// than allocating without bound. Tested both within a single bufio
// buffer and across chunk boundaries (max below the 4096 buffer size,
// so the cap must fire mid-accumulation before a newline is ever seen).
func TestReadFrame_OverflowRejected(t *testing.T) {
	cases := []struct {
		name string
		line int // bytes before the (never-reached, for the unterminated case) newline
		max  int
	}{
		{"small-cap", 200, 50},
		{"across-chunks", 50000, 4096},
		{"unterminated-flood", 1 << 20, 64 << 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// No trailing newline: simulates a plugin that writes bytes
			// forever without framing. readFrame must give up at max.
			r := bufio.NewReader(bytes.NewReader(bytes.Repeat([]byte("a"), tc.line)))
			_, err := readFrame(r, tc.max)
			if !errors.Is(err, errFrameTooLarge) {
				t.Errorf("err = %v want errFrameTooLarge", err)
			}
		})
	}
}

// A stream that ends mid-line (no trailing newline, under max) returns
// the partial bytes with io.EOF — matching the prior ReadBytes('\n')
// contract the read loop treats as a fatal end-of-plugin.
func TestReadFrame_EOFMidLine(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("partial"))
	got, err := readFrame(r, 1024)
	if err != io.EOF {
		t.Errorf("err = %v want io.EOF", err)
	}
	if string(got) != "partial" {
		t.Errorf("got %q want %q", got, "partial")
	}
}

// TestReadFrame_ExactlyMaxAccepted pins the inclusive boundary of the OOM guard: a
// frame whose total length (including the trailing newline) is EXACTLY max is accepted;
// max+1 is rejected. The existing tests cover only under-max and over-max, so the
// `len(buf)+len(chunk) > max` boundary was unpinned — mutation testing (M509) showed
// `>` could weaken to `>=` (rejecting a frame that exactly fills the limit) undetected.
func TestReadFrame_ExactlyMaxAccepted(t *testing.T) {
	const max = 64

	exact := strings.Repeat("a", max-1) + "\n" // 64 bytes total, including newline
	got, err := readFrame(bufio.NewReader(strings.NewReader(exact)), max)
	if err != nil {
		t.Fatalf("frame of exactly max=%d bytes: unexpected err %v (the limit is inclusive)", max, err)
	}
	if len(got) != max {
		t.Errorf("frame len = %d, want %d", len(got), max)
	}

	over := strings.Repeat("a", max) + "\n" // max+1 bytes total
	if _, err := readFrame(bufio.NewReader(strings.NewReader(over)), max); !errors.Is(err, errFrameTooLarge) {
		t.Errorf("frame of max+1 bytes: err = %v, want errFrameTooLarge", err)
	}
}
