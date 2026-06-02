// SPDX-License-Identifier: MIT

package journal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
)

func newTestJournal(t *testing.T, segBytes int64) *Journal {
	t.Helper()
	dir := t.TempDir()
	j, err := Open(dir, Options{
		SegmentBytes: segBytes,
		Now:          func() time.Time { return time.UnixMilli(1_700_000_000_000) },
		IDGen:        sequentialIDs(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { j.Close() })
	return j
}

// sequentialIDs returns an IDGen that produces deterministic, ULID-shaped
// strings ("01H..." prefix kept; only the tail varies). Determinism makes
// hash-chain tests reproducible.
func sequentialIDs() func() string {
	var n int
	return func() string {
		n++
		return fmt.Sprintf("01HQRSTUVWXYZ0123456789%04d", n)
	}
}

func TestTail_ReturnsLastNInOrderAcrossSegments(t *testing.T) {
	// Tiny segment threshold so the events spread across many segments — Tail
	// must stitch the newest segments back together in seq order.
	j := newTestJournal(t, 200)
	const total = 50
	for i := 0; i < total; i++ {
		if _, err := j.Append(event.Spec{
			Subject: fmt.Sprintf("ev.%d", i), Kind: event.KindTaskReceived, Actor: "kernel",
		}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}
	// Confirm rotation actually happened (otherwise the test proves nothing).
	if segs, _ := listSegments(j.dir); len(segs) < 3 {
		t.Fatalf("expected multiple segments, got %d", len(segs))
	}

	got, err := j.Tail(5)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("Tail(5) returned %d events, want 5", len(got))
	}
	// Last 5 are seqs 45..49, in order.
	for i, e := range got {
		wantSeq := int64(total - 5 + i)
		if e.Seq != wantSeq {
			t.Errorf("Tail[%d].Seq = %d, want %d (must be seq-ordered)", i, e.Seq, wantSeq)
		}
	}
}

func TestTail_EdgeCases(t *testing.T) {
	j := newTestJournal(t, 0)
	// Empty journal.
	if got, err := j.Tail(10); err != nil || len(got) != 0 {
		t.Fatalf("Tail on empty = (%v, %v), want ([], nil)", got, err)
	}
	for i := 0; i < 3; i++ {
		if _, err := j.Append(event.Spec{Subject: "x", Kind: event.KindTaskReceived, Actor: "kernel"}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	// n <= 0 → nil.
	if got, _ := j.Tail(0); got != nil {
		t.Errorf("Tail(0) = %v, want nil", got)
	}
	// n larger than total → all, in order.
	got, _ := j.Tail(100)
	if len(got) != 3 || got[0].Seq != 0 || got[2].Seq != 2 {
		t.Errorf("Tail(100) = %d events (seqs %v), want all 3 in order", len(got), seqsOf(got))
	}
}

func seqsOf(evs []*event.Event) []int64 {
	out := make([]int64, len(evs))
	for i, e := range evs {
		out[i] = e.Seq
	}
	return out
}

func TestAppend_ChainsAndPersists(t *testing.T) {
	j := newTestJournal(t, 0)

	a, err := j.Append(event.Spec{Subject: "task.x", Kind: event.KindTaskReceived, Actor: "kernel"})
	if err != nil {
		t.Fatalf("Append a: %v", err)
	}
	b, err := j.Append(event.Spec{Subject: "task.x", Kind: event.KindTaskCompleted, Actor: "kernel"})
	if err != nil {
		t.Fatalf("Append b: %v", err)
	}
	if a.Seq != 0 || b.Seq != 1 {
		t.Errorf("seq drift: a=%d b=%d", a.Seq, b.Seq)
	}
	if a.PrevHash != event.GenesisHash {
		t.Errorf("first event prev_hash %s, want genesis", a.PrevHash)
	}
	if b.PrevHash != a.Hash {
		t.Errorf("chain broken: b.prev=%s a.hash=%s", b.PrevHash, a.Hash)
	}
	seq, head := j.Head()
	if seq != 1 || head != b.Hash {
		t.Errorf("Head() = (%d,%s), want (1,%s)", seq, head, b.Hash)
	}
}

func TestVerify_ClearChain(t *testing.T) {
	j := newTestJournal(t, 0)
	for i := range 10 {
		_, err := j.Append(event.Spec{
			Subject: fmt.Sprintf("task.%d", i),
			Kind:    event.KindTaskReceived,
			Actor:   "kernel",
			Payload: map[string]int{"i": i},
		})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if err := j.Verify(); err != nil {
		t.Errorf("Verify clean chain: %v", err)
	}
}

func TestVerify_DetectsTamper(t *testing.T) {
	j := newTestJournal(t, 0)
	for range 3 {
		_, err := j.Append(event.Spec{Subject: "x", Kind: event.KindHalt, Actor: "kernel"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	// Tamper with the middle event in the segment.
	path := filepath.Join(j.dir, fmt.Sprintf("%0*d%s", segmentDigits, 1, segmentExt))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(data), `"actor":"kernel"`, `"actor":"attacker"`, 1)
	if tampered == string(data) {
		t.Fatal("test setup failure: no actor field to tamper")
	}
	if err := os.WriteFile(path, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	// Reopen and verify — must detect.
	reopened, err := Open(j.dir, Options{})
	if err == nil {
		reopened.Close()
		t.Fatal("Open should detect tamper at recovery time, got nil error")
	}
	if !errors.Is(err, ErrChainBreak) {
		t.Errorf("got err=%v, want ErrChainBreak wrapped", err)
	}
}

func TestRotation(t *testing.T) {
	// Tiny segments force rotation every few events.
	j := newTestJournal(t, 200)
	for i := range 20 {
		_, err := j.Append(event.Spec{
			Subject: "x",
			Kind:    event.KindHalt,
			Actor:   "kernel",
			Payload: map[string]int{"i": i},
		})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	entries, err := os.ReadDir(j.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Errorf("expected rotation, got %d segment(s)", len(entries))
	}
	if err := j.Verify(); err != nil {
		t.Errorf("Verify across rotation: %v", err)
	}
}

func TestRecovery_FromExistingSegments(t *testing.T) {
	dir := t.TempDir()

	// Phase 1: write 5 events, close.
	j1, err := Open(dir, Options{IDGen: sequentialIDs(), Now: func() time.Time { return time.UnixMilli(1_700_000_000_000) }})
	if err != nil {
		t.Fatalf("Open phase1: %v", err)
	}
	var lastHash string
	for i := range 5 {
		ev, err := j1.Append(event.Spec{
			Subject: fmt.Sprintf("e.%d", i),
			Kind:    event.KindTaskReceived,
			Actor:   "kernel",
		})
		if err != nil {
			t.Fatalf("append phase1 %d: %v", i, err)
		}
		lastHash = ev.Hash
	}
	if err := j1.Close(); err != nil {
		t.Fatal(err)
	}

	// Phase 2: reopen, head must match.
	j2, err := Open(dir, Options{})
	if err != nil {
		t.Fatalf("Open phase2: %v", err)
	}
	defer j2.Close()
	seq, head := j2.Head()
	if seq != 4 {
		t.Errorf("recovered seq=%d, want 4", seq)
	}
	if head != lastHash {
		t.Errorf("recovered head=%s, want %s", head, lastHash)
	}

	// Append one more, ensure it chains from the recovered head.
	ev, err := j2.Append(event.Spec{Subject: "after", Kind: event.KindHalt, Actor: "kernel"})
	if err != nil {
		t.Fatalf("append phase2: %v", err)
	}
	if ev.PrevHash != lastHash {
		t.Errorf("post-recovery prev_hash=%s, want %s", ev.PrevHash, lastHash)
	}
	if ev.Seq != 5 {
		t.Errorf("post-recovery seq=%d, want 5", ev.Seq)
	}
}

func TestRange_IteratesInOrder(t *testing.T) {
	j := newTestJournal(t, 0)
	for i := range 6 {
		_, err := j.Append(event.Spec{Subject: "x", Kind: event.KindHalt, Actor: "kernel", Payload: map[string]int{"i": i}})
		if err != nil {
			t.Fatal(err)
		}
	}
	var got []int64
	err := j.Range(func(e *event.Event) error {
		got = append(got, e.Seq)
		return nil
	})
	if err != nil {
		t.Fatalf("Range: %v", err)
	}
	want := []int64{0, 1, 2, 3, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d", len(got), len(want))
	}
	for i, s := range want {
		if got[i] != s {
			t.Errorf("seq[%d] = %d, want %d", i, got[i], s)
		}
	}
}

func TestRange_StopsOnError(t *testing.T) {
	j := newTestJournal(t, 0)
	for range 4 {
		_, _ = j.Append(event.Spec{Subject: "x", Kind: event.KindHalt, Actor: "kernel"})
	}
	stop := errors.New("stop")
	count := 0
	err := j.Range(func(e *event.Event) error {
		count++
		if count == 2 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) {
		t.Errorf("got err=%v, want stop", err)
	}
	if count != 2 {
		t.Errorf("count=%d, want 2", count)
	}
}

func TestRequiredFields_PropagateError(t *testing.T) {
	j := newTestJournal(t, 0)
	_, err := j.Append(event.Spec{Subject: "", Kind: event.KindHalt, Actor: "kernel"})
	if err == nil {
		t.Fatal("expected error for empty subject")
	}
	// Failed append must NOT advance seq or head.
	seq, head := j.Head()
	if seq != -1 || head != event.GenesisHash {
		t.Errorf("seq/head moved after failed append: seq=%d head=%s", seq, head)
	}
}

// collectEvents reads every event from a journal in seq order.
func collectEvents(t *testing.T, j *Journal) []*event.Event {
	t.Helper()
	var out []*event.Event
	if err := j.Range(func(e *event.Event) error {
		clone := *e
		out = append(out, &clone)
		return nil
	}); err != nil {
		t.Fatalf("Range: %v", err)
	}
	return out
}

func TestRestore_RoundTripBootsCleanly(t *testing.T) {
	src := newTestJournal(t, 0)
	for i := range 4 {
		if _, err := src.Append(event.Spec{
			Subject: fmt.Sprintf("task.%d", i), Kind: event.KindTaskReceived, Actor: "kernel",
		}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	events := collectEvents(t, src)
	wantSeq, wantHash := src.Head()

	dst := t.TempDir()
	gotSeq, gotHash, err := Restore(filepath.Join(dst, "journal"), events)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if gotSeq != wantSeq || gotHash != wantHash {
		t.Fatalf("restored head (%d,%s) != source (%d,%s)", gotSeq, gotHash, wantSeq, wantHash)
	}

	// Re-open the restored journal: it must boot, verify, and report the head.
	j, err := Open(filepath.Join(dst, "journal"), Options{})
	if err != nil {
		t.Fatalf("Open restored: %v", err)
	}
	defer j.Close()
	if err := j.Verify(); err != nil {
		t.Fatalf("restored journal fails Verify: %v", err)
	}
	if s, h := j.Head(); s != wantSeq || h != wantHash {
		t.Errorf("restored head (%d,%s) != source (%d,%s)", s, h, wantSeq, wantHash)
	}
	if got := collectEvents(t, j); len(got) != len(events) {
		t.Errorf("restored %d events, want %d", len(got), len(events))
	}
}

func TestRestore_RefusesNonEmpty(t *testing.T) {
	src := newTestJournal(t, 0)
	_, _ = src.Append(event.Spec{Subject: "x", Kind: event.KindTaskReceived, Actor: "kernel"})
	events := collectEvents(t, src)

	// Restore into an already-populated dir (the source's own dir).
	if _, _, err := Restore(src.dir, events); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("Restore into non-empty: err=%v, want ErrNotEmpty", err)
	}
}

func TestRestore_RefusesWindowedExport(t *testing.T) {
	src := newTestJournal(t, 0)
	for i := range 3 {
		_, _ = src.Append(event.Spec{Subject: fmt.Sprintf("t%d", i), Kind: event.KindTaskReceived, Actor: "kernel"})
	}
	events := collectEvents(t, src)

	// Drop the genesis event → slice starts at seq 1.
	if _, _, err := Restore(t.TempDir(), events[1:]); !errors.Is(err, ErrNotFullExport) {
		t.Fatalf("Restore windowed: err=%v, want ErrNotFullExport", err)
	}
	// Empty slice → also not a full export.
	if _, _, err := Restore(t.TempDir(), nil); !errors.Is(err, ErrNotFullExport) {
		t.Fatalf("Restore empty: err=%v, want ErrNotFullExport", err)
	}
}

func TestRestore_RefusesTamperedChain(t *testing.T) {
	src := newTestJournal(t, 0)
	for i := range 3 {
		_, _ = src.Append(event.Spec{Subject: fmt.Sprintf("t%d", i), Kind: event.KindTaskReceived, Actor: "kernel"})
	}
	events := collectEvents(t, src)
	events[1].Payload = []byte(`{"tampered":true}`) // break event 1's hash

	dst := filepath.Join(t.TempDir(), "journal")
	if _, _, err := Restore(dst, events); !errors.Is(err, ErrChainBreak) {
		t.Fatalf("Restore tampered: err=%v, want ErrChainBreak", err)
	}
	// Nothing should have been written (validation precedes any disk write).
	if entries, _ := os.ReadDir(dst); len(entries) != 0 {
		t.Errorf("tampered restore wrote %d file(s), want 0", len(entries))
	}
}
