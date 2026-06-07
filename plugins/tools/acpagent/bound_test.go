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

// The runaway guard `answer.Len() >= MaxOutputBytes` is inclusive: once the
// accumulation has reached the cap *exactly*, the next chunk must be dropped, not
// appended. We stream chunks summing to precisely MaxOutputBytes, then one more
// whole chunk. The guard must stop at the cap, leaving the accumulation at
// exactly MaxOutputBytes — which render() then passes through truncate()
// unchanged (len == max), so NO truncation footer appears. Were the guard `>`
// instead of `>=`, the extra chunk would be appended (len == max+chunk), forcing
// a footer. Pins `>=` against `>`.
func TestACPAgent_RunawayGuard_StopsExactlyAtCap(t *testing.T) {
	const chunkSize = 1024
	const exact = MaxOutputBytes / chunkSize // 60 chunks == exactly MaxOutputBytes
	if exact*chunkSize != MaxOutputBytes {
		t.Fatalf("test assumes chunkSize divides MaxOutputBytes: %d*%d != %d", exact, chunkSize, MaxOutputBytes)
	}
	chunks := make([]string, exact+1) // one chunk past the exact fill
	for i := range chunks {
		chunks[i] = strings.Repeat("x", chunkSize)
	}
	tool := &Tool{Cmd: "x", Cwd: "/w", dial: fakePeer(&peerRunner{chunks: chunks})}

	out, isErr := invoke(t, tool, "go")
	if isErr {
		t.Fatalf("unexpected error result: %s", out)
	}
	if strings.Contains(out, "truncated") {
		t.Errorf("guard appended past the exact cap: output carries a truncation footer, want none\n%s",
			out[max(0, len(out)-120):])
	}
	if len(out) != MaxOutputBytes {
		t.Errorf("accumulation = %d bytes, want exactly %d (the inclusive cap)", len(out), MaxOutputBytes)
	}
}

// truncate's length test is inclusive: a string of exactly max bytes fits and
// must be returned verbatim (no footer); only max+1 triggers truncation. Pins
// `len(s) <= max` against `< max`, which would wrongly tear a footer onto output
// that exactly fills the cap.
func TestTruncate_InclusiveMaxBoundary(t *testing.T) {
	const max = 16
	exact := strings.Repeat("x", max)
	if got := truncate(exact, max); got != exact {
		t.Errorf("truncate(len==max) = %q, want it unchanged", got)
	}
	over := strings.Repeat("x", max+1)
	got := truncate(over, max)
	if !strings.Contains(got, "truncated 1 bytes") {
		t.Errorf("truncate(len==max+1) = %q, want a 1-byte truncation footer", got)
	}
	if got[:max] != exact {
		t.Errorf("truncate kept %q, want the first %d bytes", got[:max], max)
	}
}
