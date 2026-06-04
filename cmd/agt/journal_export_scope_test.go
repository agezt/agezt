// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
)

// mkCorrEvent builds a standalone hash-valid event with a given correlation id
// (no inter-event chaining — a scoped cut is non-contiguous by design).
func mkCorrEvent(t *testing.T, seq int64, corr string) *event.Event {
	t.Helper()
	e, err := event.New(event.Spec{
		Subject:       "test.subject",
		Kind:          event.KindTaskReceived,
		Actor:         "test",
		CorrelationID: corr,
		Payload:       map[string]any{"n": seq},
	}, "id-"+strconv.FormatInt(seq, 10), seq, time.UnixMilli(1700000000000+seq), event.GenesisHash)
	if err != nil {
		t.Fatalf("event.New(seq=%d): %v", seq, err)
	}
	return e
}

func TestScopeCorrelation(t *testing.T) {
	cases := []struct {
		spec      string
		wantCorr  string
		wantLabel string
		wantOK    bool
	}{
		{"task:run-abc", "run-abc", "task:run-abc", true},
		{"run-bare", "run-bare", "task:run-bare", true},
		{"  task:run-x  ", "run-x", "task:run-x", true}, // trimmed
		{"task:", "", "", false},                        // empty correlation
		{"", "", "", false},                             // empty spec
		{"agent:01H", "", "", false},                    // not yet supported
		{"tenant:t1", "", "", false},
		{"skill:s1", "", "", false},
		{"memory:m1", "", "", false},
	}
	for _, c := range cases {
		corr, label, ok := scopeCorrelation(c.spec)
		if ok != c.wantOK || corr != c.wantCorr || label != c.wantLabel {
			t.Errorf("scopeCorrelation(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.spec, corr, label, ok, c.wantCorr, c.wantLabel, c.wantOK)
		}
	}
}

// TestVerifyScopedBundleEvents: a cut verifies on per-event hash + scope
// membership, ignores non-contiguity, and rejects tampering + foreign events.
func TestVerifyScopedBundleEvents(t *testing.T) {
	const corr = "run-SCOPE-1"
	a := mkCorrEvent(t, 3, corr)
	b := mkCorrEvent(t, 9, corr) // non-contiguous seqs — a real cut
	c := mkCorrEvent(t, 14, corr)

	// A valid, non-contiguous cut verifies fully (continuity is NOT required).
	if n, err := verifyScopedBundleEvents([]*event.Event{a, b, c}, corr); err != nil || n != 3 {
		t.Fatalf("valid cut: n=%d err=%v, want 3,nil", n, err)
	}

	// A tampered payload trips the per-event hash check.
	bad := mkCorrEvent(t, 9, corr)
	bad.Payload = json.RawMessage(`{"n":999}`) // mutate after hashing
	if n, err := verifyScopedBundleEvents([]*event.Event{a, bad, c}, corr); err == nil {
		t.Fatalf("tampered: want error, got n=%d nil", n)
	}

	// A foreign-correlation event smuggled into the cut is rejected.
	foreign := mkCorrEvent(t, 10, "run-OTHER")
	if n, err := verifyScopedBundleEvents([]*event.Event{a, foreign, c}, corr); err == nil || n != 1 {
		t.Fatalf("foreign: n=%d err=%v, want 1,error", n, err)
	}

	// Empty cut → trivially OK.
	if n, err := verifyScopedBundleEvents(nil, corr); err != nil || n != 0 {
		t.Fatalf("empty: n=%d err=%v, want 0,nil", n, err)
	}
}
