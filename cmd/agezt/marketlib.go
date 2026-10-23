// SPDX-License-Identifier: MIT

package main

import (
	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/modules/skills/skill"
	"github.com/agezt/agezt/plugins/builtinmarket"
)

// builtinMarketLibrary builds the marketplace's built-in "Official" catalogue
// over the daemon's own market store.
//
// It is a named function (not an inline literal in runDaemon) because the
// wiring it performs is a CONTRACT with the marketplace: the daemon hands the
// built-in bundles over through the contract's port. A catalogue built over
// nothing would be EMPTY — the market would show no packs at all, and no
// test that builds the plugin by hand would notice. The composition root's
// own test therefore asserts that the catalogue really carries the built-in
// bundles.
func builtinMarketLibrary(store *market.Store) market.Library {
	return market.NewCompositeLibrary(builtinmarket.New(skill.NewBuiltins()), store)
}
