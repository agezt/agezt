// SPDX-License-Identifier: MIT

package runtime_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/modules/providers/catalog"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestTheCatalogFaceIsTheContractsPorts: the kernel's own face is the
// contract's ports — the store and the syncer.
//
// The width is checked STATICALLY, on the accessors themselves: a narrowed
// face would still answer the read side, so only the declared return types
// can tell the two apart.
func TestTheCatalogFaceIsTheContractsPorts(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if got, want := reflect.TypeOf(k.CatalogStore).Out(0), reflect.TypeOf((*catalogapi.Store)(nil)).Elem(); got != want {
		t.Fatalf("the kernel's catalog store face is %v, want the contract's store port %v", got, want)
	}
	if got, want := reflect.TypeOf(k.CatalogSyncer).Out(0), reflect.TypeOf((*catalogapi.Syncer)(nil)).Elem(); got != want {
		t.Fatalf("the kernel's catalog syncer face is %v, want the contract's syncer port %v", got, want)
	}
	if got, want := reflect.TypeOf(k.Catalog).Out(0), reflect.TypeOf((*catalogapi.Catalog)(nil)); got != want {
		t.Fatalf("the kernel's catalog face is %v, want the contract's row %v", got, want)
	}
}

// TestTheCatalogPortsStayNilWithoutALibrary: a kernel opened without a
// catalog library hands over NIL ports — not a port wrapping nothing.
//
// The check is on the CONCRETE value: a nil *Store handed to an interface
// parameter is already a non-nil interface, and the surfaces would bind a
// store that is not there.
func TestTheCatalogPortsStayNilWithoutALibrary(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if got := k.CatalogStore(); got != nil {
		t.Fatalf("a kernel with no library must yield a nil store port, got %v", got)
	}
	if got := k.CatalogSyncer(); got != nil {
		t.Fatalf("a kernel with no library must yield a nil syncer port, got %v", got)
	}
}

// TestTheCatalogSnapshotIsNeverNil: a kernel with no library still answers the
// READ surface — with the EMPTY catalog, which is exactly what a daemon that
// has never synced knows.
//
// "No providers" is an honest answer; a nil snapshot would make every reader
// a potential panic.
func TestTheCatalogSnapshotIsNeverNil(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	cat := k.Catalog()
	if cat == nil {
		t.Fatal("the kernel must answer a catalog snapshot even without a library")
	}
	if len(cat.Providers) != 0 {
		t.Fatalf("the empty catalog must carry no providers, got %v", cat.Providers)
	}
	if _, m := cat.FindModel("anything"); m != nil {
		t.Fatal("the empty catalog must know no model")
	}
}

// TestTheCatalogRefusesToReloadWithoutALibrary: the reload reads the daemon's
// directory, so without a library it refuses — it does not panic, and it does
// not silently "succeed" with nothing.
func TestTheCatalogRefusesToReloadWithoutALibrary(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if _, err := k.ReloadCatalog(); err == nil {
		t.Fatal("reloading without a library must refuse")
	}
}

// TestTheCatalogPortsAnswerTheLiveLibrary: the ports are not shapes — they
// answer the LIVE rows. An order saved through the port has to be in the
// library the daemon opened.
func TestTheCatalogPortsAnswerTheLiveLibrary(t *testing.T) {
	base := t.TempDir()
	store := catalog.NewStore(filepath.Join(base, "catalog"))
	k, err := runtime.Open(runtime.Config{
		BaseDir:       base,
		Provider:      mock.New(),
		CatalogStore:  store,
		CatalogSyncer: catalog.NewSyncer(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if err := k.CatalogStore().SaveAPI([]byte(`{"p":{"id":"p","models":{"m":{"id":"m"}}}}`), "fixture"); err != nil {
		t.Fatalf("SaveAPI: %v", err)
	}
	cat, err := k.ReloadCatalog()
	if err != nil {
		t.Fatalf("ReloadCatalog: %v", err)
	}
	if _, m := cat.FindModel("m"); m == nil {
		t.Fatalf("the reload must answer the live rows, got %v", cat.Providers)
	}
	if _, m := k.Catalog().FindModel("m"); m == nil {
		t.Fatal("the kernel's snapshot must hold the reloaded rows")
	}
}
