// SPDX-License-Identifier: MIT

package controlplane

import (
	"path/filepath"
	"testing"

	providercatalog "github.com/agezt/agezt/kernel/modules/providers/catalog"
	"github.com/agezt/agezt/kernel/runtime"
)

// openCatalogKernel opens a kernel that HOLDS a catalog library.
//
// The catalog directory and its syncer are the DAEMON's (W3.51): the kernel
// holds the contract's ports, and a host that was given none holds nil ports
// — so a test that reloads, syncs or saves the catalog must open the library
// itself and hand the ports over.
func openCatalogKernel(t *testing.T, cfg runtime.Config) *runtime.Kernel {
	t.Helper()
	if cfg.CatalogStore == nil {
		cfg.CatalogStore = providercatalog.NewStore(filepath.Join(cfg.BaseDir, "catalog"))
	}
	if cfg.CatalogSyncer == nil {
		cfg.CatalogSyncer = providercatalog.NewSyncer()
	}
	k, err := runtime.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}
