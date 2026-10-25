// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/modules/providers/catalog"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestOpenAppKernelWiresTheCatalogLibrary: the catalog directory and its
// syncer are the DAEMON's (W3.51) — the kernel holds the contract's ports and
// the daemon opens the files.
//
// The daemon end: the ports are live, and the catalog is a DIRECTORY under the
// daemon's base directory — a provider saved through the port has to land in
// the file the next start reads.
func TestOpenAppKernelWiresTheCatalogLibrary(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if k.CatalogStore() == nil {
		t.Fatal("the daemon did not wire the catalog library — the kernel holds no store port")
	}
	if k.CatalogSyncer() == nil {
		t.Fatal("the daemon did not wire the catalog syncer — the kernel holds no syncer port")
	}

	if err := k.CatalogStore().SaveAPI([]byte(`{"p":{"id":"p","models":{"m":{"id":"m"}}}}`), "fixture"); err != nil {
		t.Fatalf("SaveAPI: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "catalog", "api.json")); err != nil {
		t.Fatalf("the daemon did not open the library's own directory: %v", err)
	}
}

// TestTheCatalogLibraryIsTheInjectedOne: the daemon builds its residents
// over the library it was HANDED (W3.51) — not over one it opened behind the
// caller's back. A caller that injects a library must have its library used,
// or the injection would be a lie.
func TestTheCatalogLibraryIsTheInjectedOne(t *testing.T) {
	dir := t.TempDir()
	store := catalog.NewStore(filepath.Join(dir, "injected"))
	k, err := openAppKernel(kernelruntime.Config{
		BaseDir:      dir,
		Provider:     mock.New(),
		CatalogStore: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if err := k.CatalogStore().SaveAPI([]byte(`{"p":{"id":"p","models":{"m":{"id":"m"}}}}`), "fixture"); err != nil {
		t.Fatalf("SaveAPI: %v", err)
	}
	// The injected library holds the file — the daemon did not open a second
	// directory of its own.
	if _, err := os.Stat(filepath.Join(dir, "injected", "api.json")); err != nil {
		t.Fatalf("the daemon saved into its own directory, not the injected one: %v", err)
	}
}
