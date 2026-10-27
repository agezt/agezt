// SPDX-License-Identifier: MIT

package market

// Provenance: the marketplace cache's publish guarantee (W5.4) — a publish
//             that fails must leave the cached marketplace exactly as it was,
//             because the cache is the only copy the operator has of a
//             catalogue that was fetched once.

import "testing"

// TestSaveMarketplaceKeepsThePreviousCacheWhenThePublishFails: the publish
// fails while the staging tree is being written, and the previous cache is
// still there afterwards.
//
// The old code deleted the live tree and then renamed the staged one over it,
// so a failure in between left the cache EMPTY — the content was deleted, not
// moved, and could not be recovered.
func TestSaveMarketplaceKeepsThePreviousCacheWhenThePublishFails(t *testing.T) {
	store := NewStore(t.TempDir())
	mp := Marketplace{Name: "acme", FormatVersion: FormatVersion}

	if err := store.SaveMarketplace("acme", mp, []Pack{{Name: "good", Version: "1.0.0"}}); err != nil {
		t.Fatalf("the first publish: %v", err)
	}

	// A pack whose name carries a path separator cannot be written into the
	// staging tree, so the publish fails.
	bad := []Pack{{Name: "sub/dir", Version: "2.0.0"}}
	if err := store.SaveMarketplace("acme", mp, bad); err == nil {
		t.Fatal("the publish of an unwritable pack must fail")
	}

	// The previous cache must still be there, whole.
	p, err := store.CachedPack("acme", "good")
	if err != nil {
		t.Fatalf("the previous cache is gone: %v", err)
	}
	if p.Name != "good" || p.Version != "1.0.0" {
		t.Fatalf("the cached pack = %+v, want the published one", p)
	}

	// And the listing must offer the published marketplace only — the failed
	// attempt's staging tree is not a marketplace.
	list, err := store.CachedMarketplaces()
	if err != nil {
		t.Fatalf("CachedMarketplaces: %v", err)
	}
	if len(list) != 1 || list[0].Name != "acme" {
		t.Fatalf("the cached marketplaces = %+v, want exactly acme", list)
	}
}

// TestSaveMarketplaceReplacesTheWholeCache: a successful publish replaces the
// tree — the packs of the previous catalogue are gone, not merged in.
func TestSaveMarketplaceReplacesTheWholeCache(t *testing.T) {
	store := NewStore(t.TempDir())
	mp := Marketplace{Name: "acme", FormatVersion: FormatVersion}

	if err := store.SaveMarketplace("acme", mp, []Pack{{Name: "old", Version: "1.0.0"}}); err != nil {
		t.Fatalf("the first publish: %v", err)
	}
	if err := store.SaveMarketplace("acme", mp, []Pack{{Name: "new", Version: "2.0.0"}}); err != nil {
		t.Fatalf("the second publish: %v", err)
	}
	if _, err := store.CachedPack("acme", "new"); err != nil {
		t.Fatalf("the new pack is missing: %v", err)
	}
	if _, err := store.CachedPack("acme", "old"); err == nil {
		t.Fatal("the previous catalogue's pack is still cached — the tree was not replaced")
	}
}
