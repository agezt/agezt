// SPDX-License-Identifier: MIT

package catalog

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/modules/providers/catalog"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The catalog family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins what that wiring buys: the snapshot the port reports is the
// daemon's own catalog, the store it hands over is the daemon's own store, and
// the refresh it runs is the daemon's own refresh. A port bound to nothing
// would report an empty catalog, and the routing that asks the port would
// answer "this model does not exist" for every model.
func TestTheCatalogPortsReachTheKernelCatalog(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(),
		CatalogStore: catalog.NewStore(filepath.Join(dir, "catalog"))})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)

	// The snapshot is the kernel's own.
	if got := ports.Snapshot(); got == nil {
		t.Fatal("the catalog port reports no snapshot — the daemon holds a catalog")
	}
	if ports.Snapshot() != k.Catalog() {
		t.Fatal("the catalog port reports a different snapshot than the kernel holds")
	}

	// The store is the kernel's own, and the daemon that has one can write.
	if ports.Store() == nil {
		t.Fatal("the catalog port reports no store — the daemon was given one")
	}
	if ports.Store() != k.CatalogStore() {
		t.Fatal("the catalog port hands over a different store than the kernel holds")
	}
}

// The refresh is the catalog's own: it re-reads the catalog from disk and
// rebuilds the live provider registry.
//
// The test pins the REPORT, not just the call: a refresh that reported "the
// providers were rebuilt" when nothing rebuilt them would tell the operator
// that a new credential is live when it is not.
func TestTheCatalogPortsRefreshThroughTheKernel(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(),
		CatalogStore: catalog.NewStore(filepath.Join(dir, "catalog"))})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	cat, providersReloaded, err := ports.Refresh()
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if cat == nil {
		t.Fatal("the refresh reports no catalog")
	}
	// The daemon was opened without an OnReload hook, so the provider registry
	// was NOT rebuilt — and the port must say so rather than claim otherwise.
	if providersReloaded {
		t.Fatal("the refresh claims the provider registry was rebuilt — no OnReload hook was configured")
	}
}

// A host with no kernel has no catalog directory.
//
// The port answers for that case in the family's own words: the snapshot is the
// EMPTY catalog (the same answer the kernel gives for a daemon that has never
// synced), the store and the syncer are absent, and the refresh REFUSES
// instead of refreshing nothing.
func TestTheCatalogPortsSurviveAHostWithNoKernel(t *testing.T) {
	ports := PortsFromKernel(nil)

	if got := ports.Snapshot(); got == nil {
		t.Fatal("a host with no kernel reports no catalog — the empty catalog IS the answer")
	}
	if got := len(ports.Snapshot().Providers); got != 0 {
		t.Fatalf("a host with no kernel reports %d providers", got)
	}
	if ports.Store() != nil {
		t.Fatal("a host with no kernel hands over a store")
	}
	if ports.Syncer() != nil {
		t.Fatal("a host with no kernel hands over a syncer")
	}
	if _, _, err := ports.Refresh(); !errors.Is(err, ErrNoLibrary) {
		t.Fatalf("a host with no kernel must refuse the refresh, got %v", err)
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheCatalogPortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the catalog port's %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	if port.NumMethod() == 0 {
		t.Fatal("the catalog port has no methods — the family cannot answer anything")
	}
}
