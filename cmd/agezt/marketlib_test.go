// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/marketapi"
	"github.com/agezt/agezt/kernel/market"
)

// TestTheBuiltinCatalogueIsWiredToTheRealBundles: the daemon hands the
// marketplace the embedded built-in bundles, so the market really offers the
// built-in skills — not an empty catalogue.
func TestTheBuiltinCatalogueIsWiredToTheRealBundles(t *testing.T) {
	store := market.NewStore(t.TempDir())
	lib := builtinMarketLibrary(store)
	if lib == nil {
		t.Fatal("the daemon must build the built-in catalogue")
	}
	packs := 0
	for _, m := range lib.Marketplaces() {
		packs += len(m.Packs)
	}
	if packs == 0 {
		t.Fatal("the built-in catalogue is empty — the daemon did not hand over the built-in bundles")
	}
}

// TestTheServedCatalogueIsTheContractsRows: the daemon serves the
// catalogue through the contract's port, so the packs it lists are the
// contract's rows — a plugin that built its own copy would drift.
func TestTheServedCatalogueIsTheContractsRows(t *testing.T) {
	store := market.NewStore(t.TempDir())
	lib := builtinMarketLibrary(store)
	for _, m := range lib.Marketplaces() {
		if m.FormatVersion != marketapi.FormatVersion {
			t.Errorf("the catalogue's format version = %d, want the contract's %d", m.FormatVersion, marketapi.FormatVersion)
		}
		for _, e := range m.Packs {
			var entry marketapi.MarketplaceEntry = e
			if entry.Name == "" {
				t.Errorf("the catalogue lists an entry with no name: %+v", e)
			}
		}
	}
}
