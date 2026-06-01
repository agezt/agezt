// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
)

func mkChainEvent(t *testing.T, seq int64, prev string, payload any) *event.Event {
	t.Helper()
	e, err := event.New(event.Spec{
		Subject: "test.subject",
		Kind:    event.KindTaskReceived,
		Actor:   "test",
		Payload: payload,
	}, "id-"+strconv.FormatInt(seq, 10), seq, time.UnixMilli(1700000000000+seq), prev)
	if err != nil {
		t.Fatalf("event.New(seq=%d): %v", seq, err)
	}
	return e
}

func TestVerifyBundleEvents(t *testing.T) {
	e0 := mkChainEvent(t, 0, event.GenesisHash, map[string]any{"n": 0})
	e1 := mkChainEvent(t, 1, e0.Hash, map[string]any{"n": 1})
	e2 := mkChainEvent(t, 2, e1.Hash, map[string]any{"n": 2})

	// Intact chain verifies fully.
	if n, err := verifyBundleEvents([]*event.Event{e0, e1, e2}); err != nil || n != 3 {
		t.Fatalf("intact: n=%d err=%v, want 3,nil", n, err)
	}

	// A windowed slice (no genesis-linked first event) still verifies — only
	// per-event integrity + intra-slice continuity are checked.
	if n, err := verifyBundleEvents([]*event.Event{e1, e2}); err != nil || n != 2 {
		t.Fatalf("window: n=%d err=%v, want 2,nil", n, err)
	}

	// Tampered payload → hash mismatch at that event.
	e1bad := mkChainEvent(t, 1, e0.Hash, map[string]any{"n": 1})
	e1bad.Payload = json.RawMessage(`{"n":999}`) // mutate after hashing
	if n, err := verifyBundleEvents([]*event.Event{e0, e1bad, e2}); err == nil {
		t.Fatalf("tampered: want error, got n=%d nil", n)
	}

	// Gap (middle event dropped) → chain break detected at the gap.
	if n, err := verifyBundleEvents([]*event.Event{e0, e2}); err == nil || n != 1 {
		t.Fatalf("gap: n=%d err=%v, want 1,error", n, err)
	}

	// Empty slice → trivially OK.
	if n, err := verifyBundleEvents(nil); err != nil || n != 0 {
		t.Fatalf("empty: n=%d err=%v, want 0,nil", n, err)
	}
}

func TestCheckBundleCompleteness(t *testing.T) {
	e0 := mkChainEvent(t, 0, event.GenesisHash, map[string]any{"n": 0})
	e1 := mkChainEvent(t, 1, e0.Hash, map[string]any{"n": 1})
	e2 := mkChainEvent(t, 2, e1.Hash, map[string]any{"n": 2})
	full := []*event.Event{e0, e1, e2}

	// A manifest matching the full slice (head = last event) is complete.
	good := journalBundleManifest{
		Count: 3, FirstSeq: 0, LastSeq: 2, HeadSeq: 2, HeadHash: e2.Hash,
	}
	if err := checkBundleCompleteness(full, good); err != nil {
		t.Fatalf("complete bundle: unexpected error %v", err)
	}

	// Tail-truncated: drop the last event but keep the manifest claiming head=e2.
	// This is the omission attack — the prefix still chain-verifies, so only the
	// head check catches it.
	if err := checkBundleCompleteness([]*event.Event{e0, e1}, good); err == nil {
		t.Fatalf("tail-truncated bundle: want error, got nil")
	}

	// Hash mismatch at the head (manifest head_hash forged) → incomplete.
	forged := good
	forged.HeadHash = e1.Hash // claim a different head than the actual last event
	if err := checkBundleCompleteness(full, forged); err == nil {
		t.Fatalf("forged head_hash: want error, got nil")
	}

	// Empty bundle with a non-zero manifest count → error.
	if err := checkBundleCompleteness(nil, journalBundleManifest{Count: 5}); err == nil {
		t.Fatalf("empty vs count=5: want error, got nil")
	}
	// Genuinely empty (count 0) → OK.
	if err := checkBundleCompleteness(nil, journalBundleManifest{}); err != nil {
		t.Fatalf("empty bundle: unexpected error %v", err)
	}
}

func TestShortHash(t *testing.T) {
	if got := shortHash("abcdef"); got != "abcdef" {
		t.Errorf("short input should be unchanged, got %q", got)
	}
	long := "0123456789abcdef0123456789abcdef"
	if got := shortHash(long); got != "0123456789ab…" {
		t.Errorf("shortHash(long) = %q", got)
	}
}
