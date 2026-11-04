// SPDX-License-Identifier: MIT

package standing

// Provenance: The standing library's LIFECYCLE (W3.76x).
//
//             The library's data and its lifecycle now live together: the
//             daemon used to hold the four writes over the store it kept the
//             file port of — so the library's rules were in one place and its
//             lifecycle in another, and the daemon spelled a second vocabulary
//             for it. The service is the library's, and this file pins what it
//             answers.

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/event"
)

// The service answers the CONTRACT's port — the compiler says so, not a
// comment. (The assertion lives with the service; this file pins the BEHAVIOUR
// the port promises.)
var _ standingapi.Lifecycle = (*Service)(nil)

// TestTheLibrarysLifecycleRefusesWithoutAStore: a library with no store
// refuses every write with the CONTRACT's row (standingapi.ErrNoLibrary) —
// not a panic, and not a silent "nothing to do".
//
// The refusal is the contract's row because the absence is one fact that more
// than one layer has to name: the library raises it, the applications raise it
// (they are the ones that notice), and a caller compares with errors.Is.
func TestTheLibrarysLifecycleRefusesWithoutAStore(t *testing.T) {
	svc := NewService(Dependencies{})

	if _, err := svc.AddStanding(standingapi.Order{Name: "watch"}); !errors.Is(err, standingapi.ErrNoLibrary) {
		t.Errorf("the add answered %v, want %v", err, standingapi.ErrNoLibrary)
	}
	if _, err := svc.SetStandingEnabled("id", false); !errors.Is(err, standingapi.ErrNoLibrary) {
		t.Errorf("the pause answered %v, want %v", err, standingapi.ErrNoLibrary)
	}
	if _, _, err := svc.UpdateStanding("id", func(*standingapi.Order) {}); !errors.Is(err, standingapi.ErrNoLibrary) {
		t.Errorf("the edit answered %v, want %v", err, standingapi.ErrNoLibrary)
	}
	if _, err := svc.RemoveStanding("id"); !errors.Is(err, standingapi.ErrNoLibrary) {
		t.Errorf("the removal answered %v, want %v", err, standingapi.ErrNoLibrary)
	}
	// The read is nil-safe: a library with no store sees no orders rather than
	// a call into nothing.
	if got := svc.StandingList(); got != nil {
		t.Errorf("StandingList = %+v, want none from a library with no store", got)
	}
}

// TestTheLibrarysLifecycleWritesAndJournals: the lifecycle is the library's,
// and every transition it makes is JOURNALLED — that is what the daemon used
// to do, and the journal is why the library and its lifecycle belong
// together.
func TestTheLibrarysLifecycleWritesAndJournals(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "standing"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	journal := []event.Spec{}
	svc := NewService(Dependencies{Store: store, Publish: func(spec event.Spec) {
		journal = append(journal, spec)
	}})

	added, err := svc.AddStanding(standingapi.Order{
		Name:     "watch",
		Triggers: []standingapi.Trigger{{Type: standingapi.TriggerEvent, Subject: "deploy.*"}},
	})
	if err != nil {
		t.Fatalf("AddStanding: %v", err)
	}
	if added.ID == "" {
		t.Fatal("the library gave the order no id")
	}

	paused, err := svc.SetStandingEnabled(added.ID, false)
	if err != nil {
		t.Fatalf("SetStandingEnabled: %v", err)
	}
	if paused.Enabled {
		t.Fatal("the pause left the order enabled")
	}

	edited, found, err := svc.UpdateStanding(added.ID, func(o *standingapi.Order) { o.Name = "edited" })
	if err != nil || !found {
		t.Fatalf("UpdateStanding = %+v %v %v, want the order edited", edited, found, err)
	}
	if edited.Name != "edited" {
		t.Fatalf("the edited order = %+v, want the new name", edited)
	}

	removed, err := svc.RemoveStanding(added.ID)
	if err != nil || !removed {
		t.Fatalf("RemoveStanding = %v %v, want the order removed", removed, err)
	}

	// The journal tells the whole story: created, updated (paused), updated
	// (edited), removed — in that order, each with the library's own subject.
	kinds := []event.Kind{}
	for _, spec := range journal {
		kinds = append(kinds, spec.Kind)
	}
	want := []event.Kind{event.KindStandingCreated, event.KindStandingUpdated, event.KindStandingUpdated, event.KindStandingRemoved}
	if len(kinds) != len(want) {
		t.Fatalf("the journal holds %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("the journal holds %v, want %v", kinds, want)
		}
	}
	if journal[0].Subject != "standing."+added.ID || journal[3].Subject != "standing."+added.ID {
		t.Fatalf("the journal's subjects = %q / %q, want the order's own subject", journal[0].Subject, journal[3].Subject)
	}
	action := func(spec event.Spec) any {
		payload, _ := spec.Payload.(map[string]any)
		return payload["action"]
	}
	if action(journal[1]) != "paused" || action(journal[2]) != "edited" {
		t.Fatalf("the journal's actions = %v / %v, want paused then edited", action(journal[1]), action(journal[2]))
	}
}

// TestTheLibrarysLifecycleAnswersForAnUnknownOrder: an UNKNOWN id is not an
// error on the edit path — the bool says whether the order was there, which
// mirrors the schedule-edit path the daemon already used.
func TestTheLibrarysLifecycleAnswersForAnUnknownOrder(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "standing"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	journal := []event.Spec{}
	svc := NewService(Dependencies{Store: store, Publish: func(spec event.Spec) {
		journal = append(journal, spec)
	}})

	edited, found, err := svc.UpdateStanding("missing", func(*standingapi.Order) {})
	if err != nil || found || edited.ID != "" {
		t.Fatalf("UpdateStanding(missing) = %+v %v %v, want an empty order, false and no error", edited, found, err)
	}
	if len(journal) != 0 {
		t.Fatalf("an edit that changed nothing journalled %v", journal)
	}

	removed, err := svc.RemoveStanding("missing")
	if err != nil || removed {
		t.Fatalf("RemoveStanding(missing) = %v %v, want false and no error", removed, err)
	}
	if len(journal) != 0 {
		t.Fatalf("a removal that changed nothing journalled %v", journal)
	}
}

// TestTheLibrarysLifecycleNeedsNoPublisher: a host that does not want the
// journal still gets its answers — the lifecycle is not the bus's.
func TestTheLibrarysLifecycleNeedsNoPublisher(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "standing"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	svc := NewService(Dependencies{Store: store})
	if _, err := svc.AddStanding(standingapi.Order{
		Name:     "watch",
		Triggers: []standingapi.Trigger{{Type: standingapi.TriggerEvent, Subject: "deploy.*"}},
	}); err != nil {
		t.Fatalf("AddStanding: %v", err)
	}
	if got := svc.StandingList(); len(got) != 1 {
		t.Fatalf("StandingList = %+v, want the one order the library holds", got)
	}
}
