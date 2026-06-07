// SPDX-License-Identifier: MIT

package journal

import (
	"os"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

// These tests close gaps that mutation testing (go-mutesting, M491) exposed in
// the existing rotation/Tail coverage: the prior tests used tiny segment
// thresholds where a *single* event line already exceeds the limit, so they
// could not distinguish correct byte accounting from broken accounting, and they
// gathered exactly n events in Tail so the trim path never ran.

func appendOne(t *testing.T, j *Journal) {
	t.Helper()
	if _, err := j.Append(event.Spec{Subject: "task.x", Kind: event.KindTaskReceived, Actor: "kernel"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
}

// Rotation must trigger on the *accumulated* size across appends, not on any
// single line. With a threshold that no single event line reaches but a few do
// together, correct accounting (curBytes += lineLen) rotates after a few events;
// a regression to curBytes = lineLen (or any non-accumulating accounting) would
// keep the running total pinned at one line's size, so rotation would never fire
// and the journal would grow into one unbounded segment. Kills the
// `curBytes += …` → `curBytes = …` mutant, which the tiny-segment tests miss.
func TestRotate_AccountsForAccumulatedBytes(t *testing.T) {
	// Measure one event line under a threshold so large nothing rotates.
	probe := newTestJournal(t, 1<<30)
	appendOne(t, probe)
	psegs, err := listSegments(probe.dir)
	if err != nil || len(psegs) != 1 {
		t.Fatalf("probe listSegments = (%d, %v), want (1, nil)", len(psegs), err)
	}
	info, err := os.Stat(psegs[0].path)
	if err != nil {
		t.Fatalf("stat probe segment: %v", err)
	}
	lineSize := info.Size()
	_ = probe.Close()
	if lineSize <= 0 {
		t.Fatalf("probe line size = %d, want > 0", lineSize)
	}

	// Threshold ~3.5 lines: a single line (lineSize) is well under it, so rotation
	// can only happen by accumulation.
	segBytes := lineSize*3 + lineSize/2
	const nEvents = 12
	j := newTestJournal(t, segBytes)
	for i := 0; i < nEvents; i++ {
		appendOne(t, j)
	}

	segs, err := listSegments(j.dir)
	if err != nil {
		t.Fatalf("listSegments: %v", err)
	}
	if len(segs) < 2 {
		t.Fatalf("rotation never fired: %d events at ~%d B/line, threshold %d B (~3.5 lines) produced %d segment(s); "+
			"the running byte total must accumulate across appends", nEvents, lineSize, segBytes, len(segs))
	}
	// Rotation must not corrupt the chain, and every event must still read back.
	if err := j.Verify(); err != nil {
		t.Fatalf("Verify after accumulation-driven rotation: %v", err)
	}
	got, err := j.Tail(nEvents)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(got) != nEvents {
		t.Fatalf("Tail(%d) after rotation returned %d events", nEvents, len(got))
	}
}

// Tail(n) must return exactly the last n events when the gathered set overshoots
// n — i.e. when a single segment holds more than n events. The existing
// cross-segment test gathers exactly n (one event per tiny segment), so the trim
// line `collected = collected[len(collected)-n:]` never executes. Here one large
// segment holds all events, so Tail(2) gathers 5 and must trim to the last 2.
// Kills the `len(collected)-n` → `len(collected)+n` mutant (which would panic or
// mis-slice) and any off-by-one in the trim.
func TestTail_TrimsExcessToLastN(t *testing.T) {
	j := newTestJournal(t, 1<<30) // one big segment, no rotation
	const total = 5
	for i := 0; i < total; i++ {
		appendOne(t, j)
	}
	got, err := j.Tail(2)
	if err != nil {
		t.Fatalf("Tail(2): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Tail(2) returned %d events, want 2", len(got))
	}
	// Seqs are 0..4; the last two are 3 and 4, in order.
	if got[0].Seq != total-2 || got[1].Seq != total-1 {
		t.Errorf("Tail(2) seqs = %v, want [%d %d]", seqsOf(got), total-2, total-1)
	}
}
