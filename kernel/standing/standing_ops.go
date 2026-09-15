// SPDX-License-Identifier: MIT

// Package standing: CRUD + storage methods (Open + Add + SetEnabled +
// safeCall + Update + Remove + Get + List + Count + save).
// Split from standing.go during Day 211 god-file refactor (#47).
// Public API unchanged.
package standing

import (
	"fmt"
	"sort"
	"time"

	"github.com/agezt/agezt/kernel/jsonstore"
	"github.com/agezt/agezt/kernel/ulid"
)

func Open(dir string) (*Store, error) {
	s := &Store{now: time.Now}
	path, err := jsonstore.LoadFrom(dir, "standing.json", &s.orders)
	if err != nil {
		return nil, fmt.Errorf("standing: %w", err)
	}
	s.path = path
	return s, nil
}

// Add validates and persists a new enabled order, assigning an id + timestamps.
// The caller-supplied o.ID/Enabled/timestamps are ignored (kernel-assigned).
func (s *Store) Add(o Order) (Order, error) {
	if err := Validate(o); err != nil {
		return Order{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UnixMilli()
	o.ID = ulid.New()
	o.Enabled = true
	o.CreatedMS = now
	o.UpdatedMS = now
	cp := o
	s.orders = append(s.orders, &cp)
	if err := s.save(); err != nil {
		s.orders = s.orders[:len(s.orders)-1]
		return Order{}, err
	}
	return cp, nil
}

// SetEnabled pauses (false) or resumes (true) an order. Returns the new state
// and whether the id existed. Panics from save() are recovered and returned as errors
// so the mutex is never leaked.
func (s *Store) SetEnabled(id string, enabled bool) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, o := range s.orders {
		if o.ID == id {
			// Roll back the in-memory mutation if the durable write fails, so the
			// running view never diverges from disk on a transient save error.
			prevEnabled, prevUpdated := o.Enabled, o.UpdatedMS
			o.Enabled = enabled
			o.UpdatedMS = s.now().UnixMilli()
			// safeCall handles both panic (returns error) and non-panic (propagates err)
			// errors from save(), so the mutex is never leaked regardless of failure mode.
			if err := safeCall(s.save); err != nil {
				o.Enabled, o.UpdatedMS = prevEnabled, prevUpdated
				return Order{}, err
			}
			return *o, nil
		}
	}
	return Order{}, ErrNotFound
}

// safeCall runs fn under a panic-recovery defer. If fn panics, the panic is caught
// and returned as an error. If fn returns without panicking, the returned error (nil
// or non-nil) is passed through as-is. This lets callers handle both panic and
// non-panic failure modes with a single error-check.
func safeCall(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return fn()
}

// Update applies edits to an order's mutable fields via mutate, re-validates the
// result, and persists it. Identity and lifecycle fields the caller must not edit
// here — ID, CreatedMS, and Enabled (which has its own SetEnabled setter) — are
// preserved regardless of what mutate does; UpdatedMS is bumped. On a validation
// or save failure the in-memory order is rolled back so the running view never
// diverges from disk. Returns the updated order, or ErrNotFound for an unknown id.
// If mutate panics, the panic is recovered inside this method and returned as an
// error; the mutex is released and the store remains usable.
func (s *Store) Update(id string, mutate func(*Order)) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, o := range s.orders {
		if o.ID == id {
			snapshot := *o
			if err := safeCall(func() error { mutate(o); return nil }); err != nil {
				return Order{}, fmt.Errorf("standing: mutate panicked: %w", err)
			}
			// Protect identity + lifecycle fields from the mutator.
			o.ID, o.CreatedMS, o.Enabled = snapshot.ID, snapshot.CreatedMS, snapshot.Enabled
			o.UpdatedMS = s.now().UnixMilli()
			if err := Validate(*o); err != nil {
				*o = snapshot
				return Order{}, err
			}
			if err := s.save(); err != nil {
				*o = snapshot
				return Order{}, err
			}
			return *o, nil
		}
	}
	return Order{}, ErrNotFound
}

// Remove deletes an order. Returns whether it existed.
func (s *Store) Remove(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, o := range s.orders {
		if o.ID == id {
			removed := s.orders
			s.orders = append(append([]*Order{}, s.orders[:i]...), s.orders[i+1:]...)
			if err := s.save(); err != nil {
				s.orders = removed // restore: disk write failed, keep the order
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

// Get returns one order by id.
func (s *Store) Get(id string) (Order, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, o := range s.orders {
		if o.ID == id {
			return *o, true
		}
	}
	return Order{}, false
}

// List returns all orders, sorted by creation time then id (deterministic).
func (s *Store) List() []Order {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Order, 0, len(s.orders))
	for _, o := range s.orders {
		out = append(out, *o)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedMS != out[j].CreatedMS {
			return out[i].CreatedMS < out[j].CreatedMS
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Count returns the number of orders.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.orders)
}

func (s *Store) save() error {
	return jsonstore.Save(s.path, s.orders)
}
