// SPDX-License-Identifier: MIT

package runtime_test

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/datalakeapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// fakeLake is a data lake that only remembers what it was asked.
type fakeLake struct {
	listed  int
	created []string
}

// ListCollections returns no collection.
func (f *fakeLake) ListCollections() []datalakeapi.CollectionInfo {
	f.listed++
	return nil
}

// CreateCollection records the collection.
func (f *fakeLake) CreateCollection(sc datalakeapi.Schema, actor string) (datalakeapi.Schema, error) {
	f.created = append(f.created, sc.Name)
	return sc, nil
}

// DropCollection forgets the collection.
func (f *fakeLake) DropCollection(string) error { return nil }

// Schema reports that no collection exists.
func (f *fakeLake) Schema(string) (datalakeapi.Schema, bool) { return datalakeapi.Schema{}, false }

// Insert records nothing.
func (f *fakeLake) Insert(coll string, fields map[string]any, actor string) (datalakeapi.Record, error) {
	return datalakeapi.Record{}, nil
}

// Get reports that no record exists.
func (f *fakeLake) Get(coll, id string) (datalakeapi.Record, error) {
	return datalakeapi.Record{}, datalakeapi.ErrNotFound
}

// Update records nothing.
func (f *fakeLake) Update(coll, id string, patch map[string]any, actor string) (datalakeapi.Record, error) {
	return datalakeapi.Record{}, nil
}

// Delete removes nothing.
func (f *fakeLake) Delete(coll, id string) error { return nil }

// Query returns no record.
func (f *fakeLake) Query(coll string, q datalakeapi.Query) ([]datalakeapi.Record, error) {
	return nil, nil
}

// TestTheKernelHoldsTheLakePortItWasGiven pins the port the composition root
// hands over.
//
// The data lake is the DAEMON's file: the kernel is a module, and a module
// may not name another module's implementation. So the daemon opens the lake
// and hands the kernel the contract's port, and the kernel must hold exactly
// that port — not open a lake of its own, and not wrap the one it was given.
func TestTheKernelHoldsTheLakePortItWasGiven(t *testing.T) {
	lake := &fakeLake{}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Lake: lake})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	got := k.DataLake()
	if got == nil {
		t.Fatal("the kernel holds no lake port — the composition root's lake never arrived")
	}
	if got != datalakeapi.Lake(lake) {
		t.Fatalf("the kernel holds %T, want the very port it was given", got)
	}
	// The port is the one the host talks to, not a wrapper of the kernel's own.
	if _, ok := got.(*fakeLake); !ok {
		t.Fatalf("the kernel wrapped the port it was given (%T)", got)
	}
}

// TestTheKernelDoesNotOpenADataLakeOfItsOwn pins the other half.
//
// The kernel used to open the lake itself, which is why the kernel reached
// the datalake module at all. Now the lake is the composition root's
// business — so a kernel that was given no lake holds NO port.
//
// The nil port must be visible as nil: a nil port that arrives as a non-nil
// interface is the typed-nil trap, and it would turn "no lake" into a panic
// at the first call instead of an honest "not available".
func TestTheKernelDoesNotOpenADataLakeOfItsOwn(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	if got := k.DataLake(); got != nil {
		t.Fatalf("the kernel opened a data lake of its own: %T", got)
	}
}
