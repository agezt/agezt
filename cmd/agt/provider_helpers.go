// SPDX-License-Identifier: MIT
//
// cmd/agt provider helpers (loadCatalogIfAny, plural).
// Extracted from provider.go during Day 211 god-file refactor (#74).
// Public API unchanged.
package main

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/catalog"
)

func loadCatalogIfAny(stderr io.Writer) (*catalog.Catalog, error) {
	base, err := paths.BaseDir()
	if err != nil {
		return nil, err
	}
	store := catalog.NewStore(base + "/catalog")
	cat, err := store.Load()
	if err != nil {
		// Non-fatal — the operator just won't see provider grouping.
		fmt.Fprintf(stderr, "warning: catalog load failed (continuing without grouping): %v\n", err)
		return nil, err
	}
	if cat == nil || len(cat.Providers) == 0 {
		return nil, nil
	}
	return cat, nil
}
func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}
