// SPDX-License-Identifier: MIT

package board

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/boardapi"
)

// The board's store IS the contract's port: the board tool is a PLUGIN, so it
// takes the store through the port and never names this package. A
// compile-time assertion — the port is a promise, not a comment.
var _ boardapi.Store = (*Store)(nil)

// TestTheStoreAnswersThroughThePort: the store must work when it is HELD AS
// the contract's port — the tool never sees the concrete type, so a method
// that only worked on *Store would slip through every other test.
func TestTheStoreAnswersThroughThePort(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var port boardapi.Store = st

	posted, err := port.Post("help", "alice", "please look", 1000)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if posted.Topic != "help" || posted.From != "alice" || posted.Text != "please look" {
		t.Fatalf("posted message = %+v", posted)
	}

	read := port.Read("help", 10)
	if len(read) != 1 || read[0].ID != posted.ID {
		t.Fatalf("Read returned %+v", read)
	}

	// The wildcard recipient is the contract's value, and a broadcast lands in
	// every inbox but the sender's.
	broadcast, err := port.Broadcast("alice", "all hands", 1100)
	if err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	if broadcast.To != boardapi.Everyone {
		t.Fatalf("broadcast To = %q, want %q", broadcast.To, boardapi.Everyone)
	}
	inbox := port.Inbox("bob", 10, false)
	if len(inbox) != 1 || inbox[0].ID != broadcast.ID {
		t.Fatalf("bob's inbox = %+v", inbox)
	}
	if own := port.Inbox("alice", 10, false); len(own) != 0 {
		t.Fatalf("the sender must not see its own broadcast: %+v", own)
	}
}
