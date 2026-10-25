// SPDX-License-Identifier: MIT

package runtime_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/tasteapi"
	"github.com/agezt/agezt/kernel/modules/knowledge/taste"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestTheTasteFaceIsTheContractsPort: the kernel's own face is the
// contract's port.
//
// The width is checked STATICALLY, on the accessor itself: a narrowed face
// would still answer the read side, so only the declared return type can tell
// the two apart.
func TestTheTasteFaceIsTheContractsPort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if got, want := reflect.TypeOf(k.Taste).Out(0), reflect.TypeOf((*tasteapi.Store)(nil)).Elem(); got != want {
		t.Fatalf("the kernel's taste face is %v, want the contract's store port %v", got, want)
	}
}

// TestTheTastePortStaysNilWithoutALibrary: a kernel opened without an
// exemplar library hands over a NIL port — not a port wrapping nothing.
//
// The check is on the CONCRETE value: a nil *Store handed to an interface
// parameter is already a non-nil interface, and the run would then select
// exemplars from a store that is not there.
func TestTheTastePortStaysNilWithoutALibrary(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if got := k.Taste(); got != nil {
		t.Fatalf("a kernel with no library must yield a nil store port, got %v", got)
	}
}

// TestTheTastePortAnswersTheLiveLibrary: the port is not a shape — it answers
// the LIVE rows. An exemplar saved through the port has to be in the library
// the host opened.
func TestTheTastePortAnswersTheLiveLibrary(t *testing.T) {
	base := t.TempDir()
	store, err := taste.OpenStore(filepath.Join(base, "taste"))
	if err != nil {
		t.Fatalf("open taste store: %v", err)
	}
	k, err := runtime.Open(runtime.Config{
		BaseDir:    base,
		Provider:   mock.New(),
		TasteStore: store,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if k.Taste() == nil {
		t.Fatal("the kernel holds no exemplar port")
	}
	ex, err := k.Taste().Create(tasteapi.CreateSpec{Title: "seed", Body: "body"}, 1000)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, ok := k.Taste().Get(ex.ID)
	if !ok || got.ID != ex.ID || got.Title != "seed" {
		t.Fatalf("the port must answer the live rows, got %+v (%v)", got, ok)
	}
	if all := k.Taste().List(tasteapi.Filter{}); len(all) != 1 || all[0].ID != ex.ID {
		t.Fatalf("the port must answer the live list, got %+v", all)
	}
	if scoped := k.Taste().ForScope("", 0); len(scoped) != 1 {
		t.Fatalf("the port must answer the live scope selection, got %+v", scoped)
	}
}
