// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
)

func TestCatalogCredentialLookup_DuplicateBareVaultNeedsScopeOrEnv(t *testing.T) {
	cat := &catalog.Catalog{Providers: map[string]*catalog.Provider{
		"alpha": {ID: "alpha", Env: []string{"SHARED_API_KEY"}},
		"beta":  {ID: "beta", Env: []string{"SHARED_API_KEY"}},
	}}
	values := map[string]string{
		"SHARED_API_KEY": "legacy-key",
		catalog.ProviderCredentialName("alpha", "SHARED_API_KEY"): "alpha-key",
	}
	lookup := catalogCredentialLookup(cat, func(name string) string { return values[name] })

	alpha := cat.Providers["alpha"]
	beta := cat.Providers["beta"]
	if !alpha.HasCredentials(lookup) {
		t.Fatal("provider-scoped vault key should credential alpha")
	}
	if beta.HasCredentials(lookup) {
		t.Fatal("duplicate bare vault key should not credential beta")
	}

	t.Setenv("SHARED_API_KEY", "process-env-key")
	if !beta.HasCredentials(lookup) {
		t.Fatal("real process env should remain global for shared env names")
	}
}

func TestCatalogCredentialLookup_UniqueBareVaultStillWorks(t *testing.T) {
	cat := &catalog.Catalog{Providers: map[string]*catalog.Provider{
		"alpha": {ID: "alpha", Env: []string{"ALPHA_API_KEY"}},
	}}
	lookup := catalogCredentialLookup(cat, func(name string) string {
		if name == "ALPHA_API_KEY" {
			return "legacy-key"
		}
		return ""
	})
	if !cat.Providers["alpha"].HasCredentials(lookup) {
		t.Fatal("unique legacy bare vault key should remain a fallback")
	}
}
