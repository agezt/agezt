// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestJournalHead_EmptyJournalReportsZeroAndGenesisHash — clean
// install: head=0, hash=64-zero genesis. The kernel's "Head()
// returns -1 on empty" leak is clamped by the handler so the
// wire shape is friendly; the genesis hash (64 zeros) is the
// real prev_hash any first event would chain off of.
func TestJournalHead_EmptyJournalReportsZeroAndGenesisHash(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdJournalHead, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["head"]); got != 0 {
		t.Errorf("head = %d; want 0 on a fresh journal", got)
	}
	got, _ := res["hash"].(string)
	if len(got) != 64 {
		t.Errorf("hash = %q (len=%d); want 64-hex genesis on empty", got, len(got))
	}
	// Genesis is all zeros — any first event uses this as prev_hash.
	for _, ch := range got {
		if ch != '0' {
			t.Errorf("hash = %q; expected all zeros on empty (genesis)", got)
			break
		}
	}
}

// TestJournalHead_TracksAppends — after publishing N events,
// head reports N-1 (0-based) and hash is a 64-hex chain tail.
func TestJournalHead_TracksAppends(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	for i := 0; i < 3; i++ {
		if _, err := k.Bus().Publish(event.Spec{
			Subject: "test.head",
			Kind:    event.Kind("test.event"),
			Actor:   "test",
		}); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}
	res, err := c.Call(context.Background(), controlplane.CmdJournalHead, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["head"]); got != 2 {
		t.Errorf("head = %d; want 2 after three appends (0-based)", got)
	}
	hash, _ := res["hash"].(string)
	if len(hash) != 64 {
		t.Errorf("hash = %q (len=%d); want 64-hex chain-tail", hash, len(hash))
	}
}
