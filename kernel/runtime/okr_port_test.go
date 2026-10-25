// SPDX-License-Identifier: MIT

package runtime_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/okrapi"
	"github.com/agezt/agezt/kernel/modules/work/okr"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestTheOKRFaceIsTheContractsPort: the kernel's own face is the contract's
// port.
//
// The width is checked STATICALLY, on the accessor itself: a narrowed face
// would still answer the read side, so only the declared return type can tell
// the two apart.
func TestTheOKRFaceIsTheContractsPort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if got, want := reflect.TypeOf(k.OKR).Out(0), reflect.TypeOf((*okrapi.Store)(nil)).Elem(); got != want {
		t.Fatalf("the kernel's objective face is %v, want the contract's store port %v", got, want)
	}
}

// TestTheOKRPortStaysNilWithoutALibrary: a kernel opened without an objective
// library hands over a NIL port — not a port wrapping nothing.
//
// The check is on the CONCRETE value: a nil *Store handed to an interface
// parameter is already a non-nil interface, and the first write would then
// call into a store that is not there.
func TestTheOKRPortStaysNilWithoutALibrary(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if got := k.OKR(); got != nil {
		t.Fatalf("a kernel with no library must yield a nil store port, got %v", got)
	}
}

// TestTheObjectiveSurfacesRefuseWithoutALibrary: a kernel with no library
// REFUSES the objective writes — it does not panic, and it does not answer
// "nothing to do".
func TestTheObjectiveSurfacesRefuseWithoutALibrary(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if _, err := k.CreateObjective("corr", okrapi.CreateSpec{Title: "t"}); err == nil {
		t.Fatal("creating an objective must refuse without a library")
	}
	if _, err := k.AddObjectiveKeyResult("corr", "o-1", "kr", 1); err == nil {
		t.Fatal("adding a key result must refuse without a library")
	}
	if _, err := k.LinkObjectiveTask("corr", "o-1", "kr-1", "t-1"); err == nil {
		t.Fatal("linking a task must refuse without a library")
	}
	if _, err := k.ArchiveObjective("corr", "o-1"); err == nil {
		t.Fatal("archiving an objective must refuse without a library")
	}
}

// TestTheOKRPortAnswersTheLiveLibrary: the port is not a shape — it answers
// the LIVE rows. An objective saved through the port has to be in the library
// the host opened.
func TestTheOKRPortAnswersTheLiveLibrary(t *testing.T) {
	base := t.TempDir()
	store, err := okr.OpenStore(filepath.Join(base, "okr"))
	if err != nil {
		t.Fatalf("open okr store: %v", err)
	}
	k, err := runtime.Open(runtime.Config{
		BaseDir:  base,
		Provider: mock.New(),
		OKRStore: store,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if k.OKR() == nil {
		t.Fatal("the kernel holds no objective port")
	}
	o, err := k.OKR().Create(okrapi.CreateSpec{Title: "seed"}, 1000)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, ok := k.OKR().Get(o.ID)
	if !ok || got.ID != o.ID || got.Title != "seed" {
		t.Fatalf("the port must answer the live rows, got %+v (%v)", got, ok)
	}
	if all := k.OKR().List(okrapi.Filter{IncludeArchived: true}); len(all) != 1 || all[0].ID != o.ID {
		t.Fatalf("the port must answer the live list, got %+v", all)
	}
}
