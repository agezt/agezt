// SPDX-License-Identifier: MIT

// Package main thin-wraps the cmd/agt/providerlookup package so
// the call sites in check.go and quickstart.go can keep their
// existing `catalogCredentialLookup` name. Day 31 wiring: the
// inline implementation that lived here was a duplicate of
// providerlookup.CredentialLookup; the package is now actually
// exercised by the binary and the deadcode-check allowlist
// entry can go.
package main

import (
	"github.com/agezt/agezt/cmd/agt/providerlookup"
	"github.com/agezt/agezt/kernel/catalog"
)

// catalogCredentialLookup delegates to providerlookup.CredentialLookup.
// The previous inline copy (with its companion catalogScopedVaultLookup
// helper) was a duplicate of the package's ScopedVaultLookup + ChainLookup
// composition. Kept as a named wrapper so the existing check.go /
// quickstart.go callers and the TestCatalogCredentialLookup_* unit
// tests in provider_lookup_test.go compile unchanged.
func catalogCredentialLookup(cat *catalog.Catalog, vaultLookup func(string) string) func(string) string {
	return providerlookup.CredentialLookup(cat, vaultLookup)
}
