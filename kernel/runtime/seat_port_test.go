// SPDX-License-Identifier: MIT

package runtime_test

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/seatapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// fakeSeats is a seat catalog that only remembers what it was asked.
type fakeSeats struct {
	listed  int
	asked   string
	valid   bool
	created seatapi.Seat
}

// List returns one seat.
func (f *fakeSeats) List() []seatapi.Seat {
	f.listed++
	return []seatapi.Seat{{ID: "ops"}}
}

// Get returns one seat.
func (f *fakeSeats) Get(id string) (seatapi.Seat, bool) {
	f.asked = id
	return seatapi.Seat{ID: id}, true
}

// Valid reports whether the seat may be dispatched under.
func (f *fakeSeats) Valid(id string) bool {
	f.asked = id
	return f.valid
}

// Create records the seat.
func (f *fakeSeats) Create(spec seatapi.Seat) (seatapi.Seat, error) {
	f.created = spec
	return spec, nil
}

// Delete forgets the seat.
func (f *fakeSeats) Delete(string) error { return nil }

// TestTheKernelHoldsTheSeatPortItWasGiven pins the port the composition root
// hands over.
//
// The seat catalog is the DAEMON's file: the kernel is a module, and a module
// may not name another module's implementation. So the daemon opens the store
// and hands the kernel the contract's port, and the kernel must hold exactly
// that port — not open a store of its own, and not wrap the one it was given.
func TestTheKernelHoldsTheSeatPortItWasGiven(t *testing.T) {
	store := &fakeSeats{valid: true}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Seats: store})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	got := k.Seats()
	if got == nil {
		t.Fatal("the kernel holds no seat port — the composition root's catalog never arrived")
	}
	if got != seatapi.Store(store) {
		t.Fatalf("the kernel holds %T, want the very port it was given", got)
	}
	// The port is the one the host talks to, not a wrapper of the kernel's own.
	if _, ok := got.(*fakeSeats); !ok {
		t.Fatalf("the kernel wrapped the port it was given (%T)", got)
	}
}

// TestTheKernelDoesNotOpenASeatCatalogOfItsOwn pins the other half.
//
// The kernel used to open the seat store itself, which is why the kernel
// reached the seat module at all. Now the store is the composition root's
// business — so a kernel that was given no catalog holds NO port.
//
// The nil port must be visible as nil: a nil port that arrives as a non-nil
// interface is the typed-nil trap, and it would turn "no catalog" into a panic
// at the first call instead of an honest "not available".
func TestTheKernelDoesNotOpenASeatCatalogOfItsOwn(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	if got := k.Seats(); got != nil {
		t.Fatalf("the kernel opened a seat catalog of its own: %T", got)
	}
}
