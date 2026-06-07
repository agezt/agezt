// SPDX-License-Identifier: MIT

package tenant_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/tenant"
)

// List enumerates tenants by scanning the root and keeping only entries that are
// BOTH directories AND valid tenant ids (`if !e.IsDir() || !ValidID(name)
// { continue }`). TestRegistry_ListReflectsDiskAndOpenState only ever creates
// valid tenants, so the exclusion of spurious root entries was unpinned — mutation
// testing (M502) showed `||`→`&&` survived, which would surface a stray file or an
// invalid-named directory as a "tenant". This plants both and asserts they are
// excluded.
func TestRegistry_ListExcludesSpuriousRootEntries(t *testing.T) {
	o := newFakeOpener()
	dir := t.TempDir()
	reg, err := tenant.New(dir, o.open)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := reg.Acquire("alpha", time.Now()); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// A non-directory file whose name is itself a valid id: excluded by !IsDir.
	if err := os.WriteFile(filepath.Join(dir, "strayfile"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write stray file: %v", err)
	}
	// A directory whose name is not a valid tenant id (uppercase): excluded by !ValidID.
	if err := os.MkdirAll(filepath.Join(dir, "UPPER"), 0o755); err != nil {
		t.Fatalf("mkdir invalid-name dir: %v", err)
	}

	list, err := reg.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != "alpha" {
		ids := make([]string, len(list))
		for i, in := range list {
			ids[i] = in.ID
		}
		t.Fatalf("List = %v, want only [alpha]; a non-dir file and an invalid-named dir must be excluded", ids)
	}
}
