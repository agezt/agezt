// SPDX-License-Identifier: MIT

package acpagent

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// A runaway agent that streams far more than MaxOutputBytes must not grow the
// in-memory accumulation without bound. The result is truncated to
// MaxOutputBytes, and the truncation footer reports only a small overshoot
// (one chunk) rather than the full overflow — proving the accumulation is
// capped near MaxOutputBytes instead of holding the entire stream (M256).
func TestACPAgent_BoundsRunawayStream(t *testing.T) {
	const chunkSize = 2000 // not a divisor of MaxOutputBytes, so a footer appears
	const nChunks = 100    // 200000 bytes streamed, well over the 60 KiB cap
	chunks := make([]string, nChunks)
	for i := range chunks {
		chunks[i] = strings.Repeat("x", chunkSize)
	}
	tool := &Tool{Cmd: "x", Cwd: "/w", dial: fakePeer(&peerRunner{chunks: chunks})}

	out, isErr := invoke(t, tool, "go")
	if isErr {
		t.Fatalf("unexpected error result: %s", out)
	}

	m := regexp.MustCompile(`truncated (\d+) bytes`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("expected a truncation footer for an over-cap stream; got %d bytes of output", len(out))
	}
	overshoot, _ := strconv.Atoi(m[1])
	// With the cap the accumulation stops ~one chunk past MaxOutputBytes, so the
	// overshoot is ~chunkSize. Without it the footer would report ~140 KiB
	// (200 KiB streamed − 60 KiB kept). Allow a couple of chunks of slack.
	if overshoot > 4*chunkSize {
		t.Errorf("accumulation not bounded: footer reports %d bytes past the %d cap (want ≤ a chunk or two)", overshoot, MaxOutputBytes)
	}
}
