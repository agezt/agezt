// SPDX-License-Identifier: MIT

package controlplane

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/seatapi"
)

// fakeCatalog is a seat catalog that answers from a fixed set.
type fakeCatalog struct {
	valid map[string]bool
	asked []string
}

// Get returns one seat.
func (f *fakeCatalog) Get(id string) (seatapi.Seat, bool) {
	f.asked = append(f.asked, id)
	return seatapi.Seat{ID: id}, f.valid[id]
}

// Valid reports whether the seat may be dispatched under.
func (f *fakeCatalog) Valid(id string) bool {
	f.asked = append(f.asked, id)
	return f.valid[id]
}

// List returns the catalog.
func (f *fakeCatalog) List() []seatapi.Seat { return nil }

// Create adds a seat.
func (f *fakeCatalog) Create(spec seatapi.Seat) (seatapi.Seat, error) { return spec, nil }

// Delete removes a seat.
func (f *fakeCatalog) Delete(string) error { return nil }

// TestTheSeatValidatorRefusesEverySeatWhenTheCatalogIsMissing pins the
// missing-port end of the rule.
//
// The host holds the contract's port, and the port may be missing: the seat
// catalog is the daemon's file. A missing catalog must NOT become a panic on
// a nil interface.
//
// The answer mirrors what the port itself answers for the case that needs no
// catalog: an empty id means "no seat" and stays valid; a NAMED seat cannot be
// confirmed without a catalog, so it is refused.
func TestTheSeatValidatorRefusesEverySeatWhenTheCatalogIsMissing(t *testing.T) {
	valid := seatValidator(nil)
	if valid == nil {
		t.Fatal("the seat validator is nil — the workboard operations would call into nothing")
	}
	if !valid("") {
		t.Error("an empty seat id was refused without a catalog — a task that asks for no seat is not asking for an unknown one")
	}
	if !valid("  ") {
		t.Error("a blank seat id was refused without a catalog — the port trims, so a blank id is no seat")
	}
	for _, id := range []string{"ops", "default"} {
		if valid(id) {
			t.Errorf("seat %q was accepted without a catalog", id)
		}
	}
}

// TestTheSeatValidatorAsksTheCatalog pins the other end: a catalog that IS
// there is asked, not assumed. The validator is the port's own answer.
func TestTheSeatValidatorAsksTheCatalog(t *testing.T) {
	catalog := &fakeCatalog{valid: map[string]bool{"ops": true}}
	valid := seatValidator(catalog)
	if !valid("ops") {
		t.Error("the catalog said the seat is valid and the validator said no")
	}
	if valid("nope") {
		t.Error("the catalog said the seat is unknown and the validator said yes")
	}
	if len(catalog.asked) != 2 {
		t.Fatalf("the validator asked the catalog %d times, want 2", len(catalog.asked))
	}
}
