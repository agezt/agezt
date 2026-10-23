// SPDX-License-Identifier: MIT

package cadence

import (
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/cadenceapi"
)

// The module's store IS the contract's port: the agent-facing schedule tool
// drives it through the contract (kernel/contract/cadenceapi), so the two
// must not drift. A compile-time assertion — a missing method is a build
// error, which is the point: the port is a promise, not a comment.
var _ cadenceapi.Store = (*Store)(nil)

// TestTheStoreAnswersThroughThePort: the store must actually work when it is
// held as the contract's port — the tool never sees the concrete type, so a
// method that only works on *Store would slip through every other test.
func TestTheStoreAnswersThroughThePort(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	var port cadenceapi.Store = st

	now := time.Unix(1000, 0).UTC()
	added, err := port.Add("do the thing", time.Minute, "gpt-test", "agent", now)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if added.ID == "" {
		t.Fatal("Add returned an entry without an id")
	}
	if added.IntervalSec != 60 {
		t.Fatalf("IntervalSec = %d, want 60", added.IntervalSec)
	}

	if _, err := port.SetAgent(added.ID, "worker"); err != nil {
		t.Fatalf("SetAgent: %v", err)
	}
	if _, err := port.SetAssure(added.ID, 2); err != nil {
		t.Fatalf("SetAssure: %v", err)
	}

	list := port.List()
	if len(list) != 1 {
		t.Fatalf("List returned %d entries, want 1", len(list))
	}
	if list[0].Agent != "worker" {
		t.Fatalf("listed Agent = %q, want worker", list[0].Agent)
	}
	if list[0].Assure != 2 {
		t.Fatalf("listed Assure = %d, want 2", list[0].Assure)
	}

	removed, err := port.Remove(added.ID)
	if err != nil || !removed {
		t.Fatalf("Remove = %v, %v; want true, nil", removed, err)
	}
	if len(port.List()) != 0 {
		t.Fatalf("the entry survived the remove")
	}
}
