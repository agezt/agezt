// SPDX-License-Identifier: MIT

package roster

// The roster's SERVICE: the profile lifecycle, over the contract's host port.
//
// The verbs used to live in the daemon (kernel/runtime/accessors_roster.go):
// the daemon held the store, applied the write and published the event. The
// DATA was already here — the store, the normalization and the validation are
// the module's implementation — so the daemon was the only place that knew
// what a roster change MEANT: which event says what, and that a shipped
// guardian may be paused or retired but never removed.
//
// The service closes that split: the module that keeps the roster also keeps
// the operations over it, and the daemon holds the module's face
// (kernel/modules/agents/api.RosterService) and forwards to it.
//
// The store port stays the CONTRACT's host port (agentapi.Store): the file it
// lives in is the host's business, and the host may not name the module that
// keeps it. The service is built over that port by the composition root,
// which is the one place that knows both.

import (
	"errors"
	"fmt"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/agents/api"
)

// service is the roster's own face over the host's store port.
type service struct {
	store agentapi.Store
	deps  api.RosterDependencies
}

// NewService builds the roster's service over the host's ports.
//
// A nil store is not an error here: the roster REFUSES every write, and the
// refusal is the contract's row (agentapi.ErrNoRoster) — the same answer the
// daemon gives when it holds no roster.
func NewService(deps api.RosterDependencies) api.RosterService {
	return &service{store: deps.Store, deps: deps}
}

// storeOrRefusal returns the store, or the contract's refusal.
func (s *service) storeOrRefusal() (agentapi.Store, error) {
	if s.store == nil {
		return nil, agentapi.ErrNoRoster
	}
	return s.store, nil
}

// Add persists a named agent profile and announces its birth.
//
// The event is the roster's own: the subject names the agent, the kind says
// what happened and the payload carries the row's identity.
func (s *service) Add(p agentapi.Profile) (agentapi.Profile, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return agentapi.Profile{}, err
	}
	saved, err := store.Add(p)
	if err != nil {
		return agentapi.Profile{}, err
	}
	s.publish(event.Spec{
		Subject: "roster." + saved.Slug, Kind: event.KindRosterCreated, Actor: "roster",
		Payload: map[string]any{"id": saved.ID, "slug": saved.Slug, "name": saved.Name, "model": saved.Model},
	})
	return saved, nil
}

// SetEnabled pauses/resumes an agent profile and announces the change.
func (s *service) SetEnabled(ref string, enabled bool) (agentapi.Profile, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return agentapi.Profile{}, err
	}
	p, err := store.SetEnabled(ref, enabled)
	if err != nil {
		return agentapi.Profile{}, err
	}
	state := "paused"
	if enabled {
		state = "resumed"
	}
	s.publish(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "enabled": enabled, "action": state},
	})
	return p, nil
}

// SetRetired moves an agent to the graveyard (true) or revives it (false),
// announcing the change. Retiring also pauses the agent so it stops firing
// (M846). A graveyard agent is excluded from delegation (runSubAgent).
func (s *service) SetRetired(ref string, retired bool, reason ...string) (agentapi.Profile, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return agentapi.Profile{}, err
	}
	p, err := store.SetRetired(ref, retired, reason...)
	if err != nil {
		return agentapi.Profile{}, err
	}
	action := "revived"
	if retired {
		action = "retired"
	}
	s.publish(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "retired": retired, "reason": p.RetiredReason, "action": action},
	})
	return p, nil
}

// Update edits a profile's mutable fields via mutate and announces the edit.
//
// Identity/lifecycle fields are protected by the store. An unknown ref is not
// an error: it answers "no row, no problem" (the standing pattern).
func (s *service) Update(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return agentapi.Profile{}, false, err
	}
	p, err := store.Update(ref, mutate)
	if errors.Is(err, agentapi.ErrNotFound) {
		return agentapi.Profile{}, false, nil
	}
	if err != nil {
		return agentapi.Profile{}, false, err
	}
	s.publish(event.Spec{
		Subject: "roster." + p.Slug, Kind: event.KindRosterUpdated, Actor: "roster",
		Payload: map[string]any{"id": p.ID, "slug": p.Slug, "action": "edited"},
	})
	return p, true, nil
}

// Remove deletes an agent profile and announces its departure.
//
// Shipped guardians (System) are protected from a hard delete (M961): they
// are the daemon's own self-healing fleet. They can still be paused or
// retired.
func (s *service) Remove(ref string) (bool, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return false, err
	}
	if p, ok := store.Get(ref); ok && p.System {
		return false, fmt.Errorf("agent %q is a protected system guardian — pause or retire it instead of removing", p.Slug)
	}
	gone, ok, err := store.Remove(ref)
	if err != nil {
		return false, err
	}
	if ok {
		s.publish(event.Spec{
			Subject: "roster." + gone.Slug, Kind: event.KindRosterRemoved, Actor: "roster",
			Payload: map[string]any{"id": gone.ID, "slug": gone.Slug, "name": gone.Name},
		})
	}
	return ok, nil
}

// publish says what happened to an agent on the host's bus.
//
// The subject, the kind and the payload are the ROSTER's own: the host's port
// carries the event, but the roster decides what the event says.
//
// A host with no bus (no publish port) is quiet rather than broken — the
// agent still changed, and the caller still holds the answer.
func (s *service) publish(spec event.Spec) {
	if s.deps.Publish == nil {
		return
	}
	s.deps.Publish(spec)
}
