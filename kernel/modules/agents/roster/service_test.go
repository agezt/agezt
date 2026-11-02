// SPDX-License-Identifier: MIT

package roster_test

// The roster SERVICE's tests — the module's own face over the host's store
// port (W3.76n).
//
// The service took the roster's WRITES away from the daemon, so the tests
// that used to pin the daemon's copy now pin the module's: what an event says,
// when it is said, and the one rule that refuses a write outright — a shipped
// guardian may be paused or retired, never removed.

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/agents/api"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
)

// recorder collects what the service published.
type recorder struct{ specs []event.Spec }

// publish captures one event.
func (r *recorder) publish(spec event.Spec) { r.specs = append(r.specs, spec) }

// last returns the newest published spec.
func (r *recorder) last() event.Spec {
	if len(r.specs) == 0 {
		return event.Spec{}
	}
	return r.specs[len(r.specs)-1]
}

// kinds returns every published kind, in order.
func (r *recorder) kinds() []event.Kind {
	out := make([]event.Kind, 0, len(r.specs))
	for _, s := range r.specs {
		out = append(out, s.Kind)
	}
	return out
}

// newService opens a real roster store and a service over it.
func newService(t *testing.T) (api.RosterService, *recorder) {
	t.Helper()
	store, err := roster.Open(filepath.Join(t.TempDir(), "roster"))
	if err != nil {
		t.Fatalf("roster.Open: %v", err)
	}
	rec := &recorder{}
	return roster.NewService(api.RosterDependencies{Store: store, Publish: rec.publish}), rec
}

// TestTheServiceAnnouncesTheBirth: adding an agent says so, and says it the
// roster's way — the subject names the agent, the kind is the birth, and the
// payload carries the row's identity.
func TestTheServiceAnnouncesTheBirth(t *testing.T) {
	svc, rec := newService(t)

	saved, err := svc.Add(agentapi.Profile{Slug: "researcher", Name: "The Researcher", Model: "mock-model"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if saved.Slug != "researcher" || saved.ID == "" {
		t.Fatalf("Add answered %+v — a saved profile carries an id and its slug", saved)
	}
	spec := rec.last()
	if spec.Kind != event.KindRosterCreated {
		t.Errorf("published kind = %v, want %v", spec.Kind, event.KindRosterCreated)
	}
	if spec.Subject != "roster.researcher" {
		t.Errorf("published subject = %q, want roster.researcher", spec.Subject)
	}
	if spec.Actor != "roster" {
		t.Errorf("published actor = %q, want roster", spec.Actor)
	}
	payload, _ := spec.Payload.(map[string]any)
	if payload == nil {
		t.Fatalf("published payload = %v, want the profile's identity", spec.Payload)
	}
	if payload["slug"] != "researcher" || payload["id"] != saved.ID || payload["model"] != "mock-model" {
		t.Errorf("published payload = %v, want the id, slug and model of the saved profile", payload)
	}
}

// TestTheServiceAnnouncesThePauseAndTheResume: the pause and the resume are
// two different words on the same event — an operator reading the journal has
// to see which one happened.
func TestTheServiceAnnouncesThePauseAndTheResume(t *testing.T) {
	svc, rec := newService(t)
	if _, err := svc.Add(agentapi.Profile{Slug: "worker"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if _, err := svc.SetEnabled("worker", false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	spec := rec.last()
	if spec.Kind != event.KindRosterUpdated {
		t.Errorf("published kind = %v, want %v", spec.Kind, event.KindRosterUpdated)
	}
	payload, _ := spec.Payload.(map[string]any)
	if payload["action"] != "paused" || payload["enabled"] != false {
		t.Errorf("published payload = %v, want the paused action", payload)
	}

	if _, err := svc.SetEnabled("worker", true); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	payload, _ = rec.last().Payload.(map[string]any)
	if payload["action"] != "resumed" || payload["enabled"] != true {
		t.Errorf("published payload = %v, want the resumed action", payload)
	}
}

// TestTheServiceAnnouncesTheRetirementAndTheRevival: the graveyard and the
// way back out of it are two different words too.
func TestTheServiceAnnouncesTheRetirementAndTheRevival(t *testing.T) {
	svc, rec := newService(t)
	if _, err := svc.Add(agentapi.Profile{Slug: "worker"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	p, err := svc.SetRetired("worker", true, "left the project")
	if err != nil {
		t.Fatalf("SetRetired(true): %v", err)
	}
	if !p.Retired || p.RetiredReason != "left the project" {
		t.Fatalf("SetRetired answered %+v — the profile is retired and says why", p)
	}
	spec := rec.last()
	if spec.Kind != event.KindRosterUpdated {
		t.Errorf("published kind = %v, want %v", spec.Kind, event.KindRosterUpdated)
	}
	payload, _ := spec.Payload.(map[string]any)
	if payload["action"] != "retired" || payload["retired"] != true || payload["reason"] != "left the project" {
		t.Errorf("published payload = %v, want the retired action and the reason", payload)
	}

	if _, err := svc.SetRetired("worker", false); err != nil {
		t.Fatalf("SetRetired(false): %v", err)
	}
	payload, _ = rec.last().Payload.(map[string]any)
	if payload["action"] != "revived" || payload["retired"] != false {
		t.Errorf("published payload = %v, want the revived action", payload)
	}
}

// TestTheServiceAnnouncesTheEdit: an edit is an update with its own word.
func TestTheServiceAnnouncesTheEdit(t *testing.T) {
	svc, rec := newService(t)
	if _, err := svc.Add(agentapi.Profile{Slug: "worker"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	updated, ok, err := svc.Update("worker", func(p *agentapi.Profile) { p.Soul = "v2 soul" })
	if err != nil || !ok {
		t.Fatalf("Update = %v, %v, %v — want the edited profile", updated, ok, err)
	}
	if updated.Soul != "v2 soul" {
		t.Errorf("updated soul = %q, want v2 soul", updated.Soul)
	}
	spec := rec.last()
	if spec.Kind != event.KindRosterUpdated {
		t.Errorf("published kind = %v, want %v", spec.Kind, event.KindRosterUpdated)
	}
	payload, _ := spec.Payload.(map[string]any)
	if payload["action"] != "edited" {
		t.Errorf("published payload = %v, want the edited action", payload)
	}
}

// TestTheServiceAnswersNoRowForAnUnknownEdit: an unknown ref is not an
// error — it is "no row, no problem", and nothing is announced.
func TestTheServiceAnswersNoRowForAnUnknownEdit(t *testing.T) {
	svc, rec := newService(t)

	updated, ok, err := svc.Update("ghost", func(p *agentapi.Profile) { p.Soul = "x" })
	if err != nil {
		t.Fatalf("Update(ghost) error = %v, want none", err)
	}
	if ok {
		t.Error("Update(ghost) = found, want not found")
	}
	if updated.Slug != "" {
		t.Errorf("Update(ghost) answered %+v, want the empty profile", updated)
	}
	if len(rec.specs) != 0 {
		t.Errorf("an edit of nobody published %v — an event with no agent in it is a lie", rec.kinds())
	}
}

// TestTheServiceAnnouncesTheDeparture: a removal says so, and only when
// there was somebody to remove.
func TestTheServiceAnnouncesTheDeparture(t *testing.T) {
	svc, rec := newService(t)
	if _, err := svc.Add(agentapi.Profile{Slug: "worker"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	gone, err := svc.Remove("worker")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !gone {
		t.Error("Remove = false, want true — the agent was there")
	}
	spec := rec.last()
	if spec.Kind != event.KindRosterRemoved {
		t.Errorf("published kind = %v, want %v", spec.Kind, event.KindRosterRemoved)
	}
	payload, _ := spec.Payload.(map[string]any)
	if payload["slug"] != "worker" {
		t.Errorf("published payload = %v, want the removed agent's slug", payload)
	}

	// Removing nobody changes nothing and announces nothing.
	before := len(rec.specs)
	gone, err = svc.Remove("worker")
	if err != nil {
		t.Fatalf("Remove(again): %v", err)
	}
	if gone {
		t.Error("Remove(again) = true, want false — the agent was already gone")
	}
	if len(rec.specs) != before {
		t.Errorf("removing nobody published %v — a removal of nobody is not a removal", rec.kinds())
	}
}

// TestTheServiceRefusesToRemoveAShippedGuardian: the daemon's own
// self-healing fleet is protected from a hard delete (M961). It may be paused
// or retired — the two doors that still work.
//
// This rule used to live in the daemon; it is the roster's own rule about the
// roster's own rows, so it belongs to the module that keeps the rows.
func TestTheServiceRefusesToRemoveAShippedGuardian(t *testing.T) {
	svc, rec := newService(t)
	if _, err := svc.Add(agentapi.Profile{Slug: "doctor", System: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	gone, err := svc.Remove("doctor")
	if err == nil {
		t.Fatal("Remove(a system guardian) = nil error, want a refusal")
	}
	if gone {
		t.Error("Remove(a system guardian) = true, want false — the agent is still there")
	}
	if !strings.Contains(err.Error(), "protected system guardian") {
		t.Errorf("Remove(a system guardian) error = %q, want the guardian refusal", err)
	}
	if len(rec.specs) != 1 {
		t.Errorf("a refused removal published %v — the agent did not leave", rec.kinds()[1:])
	}

	// The doors that still work: paused and retired.
	if _, err := svc.SetEnabled("doctor", false); err != nil {
		t.Errorf("SetEnabled(a system guardian): %v — a guardian may be paused", err)
	}
	if _, err := svc.SetRetired("doctor", true, "stand down"); err != nil {
		t.Errorf("SetRetired(a system guardian): %v — a guardian may be retired", err)
	}
}

// TestTheServiceRefusesEveryWriteWithoutAStore: a host that opened no roster
// has no agents to change, and the refusal is the contract's row — the same
// answer whoever asks.
func TestTheServiceRefusesEveryWriteWithoutAStore(t *testing.T) {
	svc := roster.NewService(api.RosterDependencies{})

	if _, err := svc.Add(agentapi.Profile{Slug: "worker"}); !errors.Is(err, agentapi.ErrNoRoster) {
		t.Errorf("Add error = %v, want %v", err, agentapi.ErrNoRoster)
	}
	if _, err := svc.SetEnabled("worker", true); !errors.Is(err, agentapi.ErrNoRoster) {
		t.Errorf("SetEnabled error = %v, want %v", err, agentapi.ErrNoRoster)
	}
	if _, err := svc.SetRetired("worker", true); !errors.Is(err, agentapi.ErrNoRoster) {
		t.Errorf("SetRetired error = %v, want %v", err, agentapi.ErrNoRoster)
	}
	if _, _, err := svc.Update("worker", func(p *agentapi.Profile) {}); !errors.Is(err, agentapi.ErrNoRoster) {
		t.Errorf("Update error = %v, want %v", err, agentapi.ErrNoRoster)
	}
	if _, err := svc.Remove("worker"); !errors.Is(err, agentapi.ErrNoRoster) {
		t.Errorf("Remove error = %v, want %v", err, agentapi.ErrNoRoster)
	}
}

// TestTheServiceStaysQuietWithoutABus: a host with no bus is not broken. The
// agent still changed and the caller still holds the answer — the journal is
// simply not there to hear it.
func TestTheServiceStaysQuietWithoutABus(t *testing.T) {
	store, err := roster.Open(filepath.Join(t.TempDir(), "roster"))
	if err != nil {
		t.Fatalf("roster.Open: %v", err)
	}
	svc := roster.NewService(api.RosterDependencies{Store: store})

	saved, err := svc.Add(agentapi.Profile{Slug: "worker"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if saved.Slug != "worker" {
		t.Errorf("Add answered %+v, want the saved profile", saved)
	}
	if _, ok := store.Get("worker"); !ok {
		t.Error("the agent is not in the store — a quiet bus is not a failed write")
	}
}

// TestTheServiceKeepsTheStoreSoleOwner: the service writes THROUGH the
// contract's port, so the rows it answers are the store's rows and not a
// second copy of them.
func TestTheServiceKeepsTheStoreSoleOwner(t *testing.T) {
	store, err := roster.Open(filepath.Join(t.TempDir(), "roster"))
	if err != nil {
		t.Fatalf("roster.Open: %v", err)
	}
	svc := roster.NewService(api.RosterDependencies{Store: store})

	saved, err := svc.Add(agentapi.Profile{Slug: "worker"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, ok := store.Get("worker")
	if !ok {
		t.Fatal("the store does not hold the agent the service added")
	}
	if got.ID != saved.ID {
		t.Errorf("the store holds %q, want the service's %q", got.ID, saved.ID)
	}
}
