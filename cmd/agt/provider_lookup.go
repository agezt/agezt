// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"strings"

	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/creds"
)

func catalogScopedVaultLookup(cat *catalog.Catalog, vaultLookup func(string) string) func(string) string {
	if vaultLookup == nil {
		vaultLookup = func(string) string { return "" }
	}
	duplicateEnv := cat.DuplicateCredentialEnvs()
	return func(name string) string {
		name = strings.TrimSpace(name)
		if name == "" {
			return ""
		}
		if catalog.IsProviderCredentialName(name) || !duplicateEnv[name] {
			return vaultLookup(name)
		}
		return ""
	}
}

func catalogCredentialLookup(cat *catalog.Catalog, vaultLookup func(string) string) func(string) string {
	return creds.ChainLookup(catalogScopedVaultLookup(cat, vaultLookup), os.Getenv)
}
