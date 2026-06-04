// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// seedJournal writes n chain-linked events into <home>/journal and returns them.
func seedJournal(t *testing.T, home string, n int) {
	t.Helper()
	j, err := journal.Open(filepath.Join(home, "journal"), journal.Options{})
	if err != nil {
		t.Fatalf("open source journal: %v", err)
	}
	defer j.Close()
	for i := 0; i < n; i++ {
		if _, err := j.Append(event.Spec{
			Subject: "x", Kind: event.KindHalt, Actor: "test",
			Payload: map[string]any{"i": i},
		}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
}

// branchedCount opens a restored home's journal, verifies its chain, and returns
// (head seq, event count).
func branchedCount(t *testing.T, home string) (int64, int) {
	t.Helper()
	j, err := journal.Open(filepath.Join(home, "journal"), journal.Options{})
	if err != nil {
		t.Fatalf("open branched journal: %v", err)
	}
	defer j.Close()
	if err := j.Verify(); err != nil {
		t.Errorf("branched chain did not verify: %v", err)
	}
	seq, _ := j.Head()
	count := 0
	_ = j.Range(func(*event.Event) error { count++; return nil })
	return seq, count
}

// TestRestore_PointInTime_BySeq: replaying the source journal up to seq 3 yields
// a fresh home holding exactly the genesis→3 prefix (4 events) that verifies —
// SPEC-09 §5 point-in-time restore, the journal-as-time-machine.
func TestRestore_PointInTime_BySeq(t *testing.T) {
	src := t.TempDir()
	seedJournal(t, src, 6) // seq 0..5
	to := filepath.Join(t.TempDir(), "branch")

	var out, errb bytes.Buffer
	if code := cmdRestore([]string{"--at", "3", "--home", src, "--to", to}, &out, &errb); code != 0 {
		t.Fatalf("exit=%d err=%s", code, errb.String())
	}
	seq, count := branchedCount(t, to)
	if seq != 3 || count != 4 {
		t.Errorf("branched head seq=%d count=%d, want seq 3 / 4 events", seq, count)
	}
	// The source must be untouched (still 6 events).
	if s, c := branchedCount(t, src); s != 5 || c != 6 {
		t.Errorf("source mutated: head seq=%d count=%d, want 5/6", s, c)
	}
}

// TestRestore_PointInTime_SeqBeyondHeadRestoresAll: a cutoff past the head
// restores the whole chain (clamp, not error).
func TestRestore_PointInTime_SeqBeyondHeadRestoresAll(t *testing.T) {
	src := t.TempDir()
	seedJournal(t, src, 4)
	to := filepath.Join(t.TempDir(), "branch")
	var out, errb bytes.Buffer
	if code := cmdRestore([]string{"--at", "100", "--home", src, "--to", to}, &out, &errb); code != 0 {
		t.Fatalf("exit=%d err=%s", code, errb.String())
	}
	if seq, count := branchedCount(t, to); seq != 3 || count != 4 {
		t.Errorf("seq=%d count=%d, want all 4 events (head seq 3)", seq, count)
	}
}

// TestRestore_PointInTime_Timestamp: a future timestamp restores everything; a
// past one restores nothing (clean error) — exercises the timestamp branch.
func TestRestore_PointInTime_Timestamp(t *testing.T) {
	src := t.TempDir()
	seedJournal(t, src, 3)

	// Future cutoff → all events.
	toAll := filepath.Join(t.TempDir(), "all")
	var o1, e1 bytes.Buffer
	if code := cmdRestore([]string{"--at", "2099-01-01T00:00:00Z", "--home", src, "--to", toAll}, &o1, &e1); code != 0 {
		t.Fatalf("future-ts exit=%d err=%s", code, e1.String())
	}
	if _, count := branchedCount(t, toAll); count != 3 {
		t.Errorf("future timestamp restored %d events, want 3", count)
	}

	// Past cutoff → nothing matches → exit 1, no journal written.
	toNone := filepath.Join(t.TempDir(), "none")
	var o2, e2 bytes.Buffer
	if code := cmdRestore([]string{"--at", "2000-01-01T00:00:00Z", "--home", src, "--to", toNone}, &o2, &e2); code != 1 {
		t.Errorf("past-ts exit=%d want 1 (no events at or before)", code)
	}
}

// TestRestore_PointInTime_Validation: argument-shape errors are usage errors (2)
// before any work.
func TestRestore_PointInTime_Validation(t *testing.T) {
	cases := [][]string{
		{"--at", "3"},      // missing --to
		{"--to", "/tmp/x"}, // --to without --at
		{"--at", "3", "--to", "/tmp/x", "bundle.tgz"}, // --at with a positional archive
		{"--at", "not-a-seq-or-time", "--to", "/tmp/x"},
	}
	for _, args := range cases {
		var out, errb bytes.Buffer
		if code := cmdRestore(args, &out, &errb); code != 2 {
			t.Errorf("args %v: exit=%d want 2 (usage); err=%s", args, code, errb.String())
		}
	}
}

// TestRestore_PointInTime_RefusesNonEmptyTo: the target must be a fresh home;
// an existing journal is never clobbered.
func TestRestore_PointInTime_RefusesNonEmptyTo(t *testing.T) {
	src := t.TempDir()
	seedJournal(t, src, 3)
	to := t.TempDir()
	seedJournal(t, to, 1) // target already has a journal

	var out, errb bytes.Buffer
	if code := cmdRestore([]string{"--at", "2", "--home", src, "--to", to}, &out, &errb); code != 1 {
		t.Errorf("exit=%d want 1 (refuse non-empty --to)", code)
	}
}
