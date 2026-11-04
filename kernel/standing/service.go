// SPDX-License-Identifier: MIT

package standing

// Provenance: the standing library's LIFECYCLE (W3.76x).
//
// The orders' DATA and their MEANING belong together: the daemon used to hold
// the four writes (add, enable, update, remove) over the store it kept the
// file port of — so the library's rules lived in one place and the library's
// lifecycle in another, and the daemon spelled a second vocabulary for it.
//
// The service answers in the CONTRACT's names (`standingapi.Lifecycle`), so
// the applications and the agent-facing tool bind the library itself and the
// daemon holds only its FACE.

import (
	"errors"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/event"
)

// Dependencies are the ports the service needs to answer a verb.
type Dependencies struct {
	// Store is the durable standing-order library — the CONTRACT's store
	// port (W3.50). The library's FILE is the daemon's business: the
	// composition root opens it and hands the port over.
	//
	// A nil port means the daemon has no library, and the writes refuse
	// rather than call into nothing.
	Store standingapi.Store

	// Publish journals a transition on the host's bus. A nil publisher means
	// a host that does not want the journal — the lifecycle still answers.
	Publish func(event.Spec)
}

// Service is the standing library's lifecycle: the writes over the orders,
// journalling every transition.
//
// The service holds NO store of its own — the store port is the host's file —
// so a daemon that was given none holds a service that refuses.
type Service struct {
	store   standingapi.Store
	publish func(event.Spec)
}

// NewService builds the lifecycle over the host's store and bus.
func NewService(deps Dependencies) *Service {
	return &Service{store: deps.Store, publish: deps.Publish}
}

// The service answers the CONTRACT's port — the compiler says so, not a
// comment.
var _ standingapi.Lifecycle = (*Service)(nil)

// AddStanding validates and persists a standing order, journaling
// standing.created so the lifecycle is auditable (SPEC-16 §4).
func (s *Service) AddStanding(o standingapi.Order) (standingapi.Order, error) {
	if s.store == nil {
		return standingapi.Order{}, standingapi.ErrNoLibrary
	}
	saved, err := s.store.Add(o)
	if err != nil {
		return standingapi.Order{}, err
	}
	s.emit(event.Spec{
		Subject: "standing." + saved.ID, Kind: event.KindStandingCreated, Actor: "standing",
		Payload: map[string]any{"id": saved.ID, "name": saved.Name, "triggers": len(saved.Triggers)},
	})
	return saved, nil
}

// SetStandingEnabled pauses/resumes a standing order, journaling
// standing.updated.
func (s *Service) SetStandingEnabled(id string, enabled bool) (standingapi.Order, error) {
	if s.store == nil {
		return standingapi.Order{}, standingapi.ErrNoLibrary
	}
	o, err := s.store.SetEnabled(id, enabled)
	if err != nil {
		return standingapi.Order{}, err
	}
	state := "paused"
	if enabled {
		state = "resumed"
	}
	s.emit(event.Spec{
		Subject: "standing." + id, Kind: event.KindStandingUpdated, Actor: "standing",
		Payload: map[string]any{"id": id, "name": o.Name, "enabled": enabled, "action": state},
	})
	return o, nil
}

// UpdateStanding applies edits to a standing order's mutable fields,
// journalling standing.updated (action "edited") on success.
//
// Identity and lifecycle fields are protected by the store. An UNKNOWN id is
// not an error: the bool says whether the order was there (false + nil error
// for an unknown id, mirroring the schedule-edit path).
func (s *Service) UpdateStanding(id string, mutate func(*standingapi.Order)) (standingapi.Order, bool, error) {
	if s.store == nil {
		return standingapi.Order{}, false, standingapi.ErrNoLibrary
	}
	o, err := s.store.Update(id, mutate)
	if errors.Is(err, standingapi.ErrNotFound) {
		return standingapi.Order{}, false, nil
	}
	if err != nil {
		return standingapi.Order{}, false, err
	}
	s.emit(event.Spec{
		Subject: "standing." + id, Kind: event.KindStandingUpdated, Actor: "standing",
		Payload: map[string]any{"id": id, "name": o.Name, "action": "edited"},
	})
	return o, true, nil
}

// RemoveStanding deletes a standing order, journalling standing.removed when
// it existed. Returns whether it existed.
func (s *Service) RemoveStanding(id string) (bool, error) {
	if s.store == nil {
		return false, standingapi.ErrNoLibrary
	}
	o, _ := s.store.Get(id)
	removed, err := s.store.Remove(id)
	if err != nil {
		return false, err
	}
	if removed {
		s.emit(event.Spec{
			Subject: "standing." + id, Kind: event.KindStandingRemoved, Actor: "standing",
			Payload: map[string]any{"id": id, "name": o.Name},
		})
	}
	return removed, nil
}

// StandingList returns every standing order — the nil-safe read: a library
// with no store sees no orders rather than a call into nothing.
func (s *Service) StandingList() []standingapi.Order {
	if s.store == nil {
		return nil
	}
	return s.store.List()
}

// emit journals one transition on the host's bus.
func (s *Service) emit(spec event.Spec) {
	if s.publish == nil {
		return
	}
	s.publish(spec)
}
