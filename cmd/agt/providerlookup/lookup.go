// SPDX-License-Identifier: MIT

package providerlookup

import (
	"os"
	"strings"

	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/creds"
)

// ScopedVaultLookup returns a vault-lookup function that respects
// the scoped-vs-bare invariant: a bare env name (e.g. "OPENAI_API_KEY")
// is global ONLY when exactly one provider declares it; when two or
// more providers share the env name, a bare vault entry is refused
// so it cannot silently credential the wrong provider. The
// provider-scoped form (`provider:<id>:<env>`, produced by
// catalog.ProviderCredentialName) is always safe because the id
// disambiguates the two.
//
// vaultLookup may be nil — the function then returns "" for every
// name. This matches the test setup where the only credential
// under test is the process env.
func ScopedVaultLookup(cat *catalog.Catalog, vaultLookup func(string) string) func(string) string {
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

// CredentialLookup composes ScopedVaultLookup with os.Getenv. The
// process env is global by OS contract; only the vault (which we
// control) gets the per-provider scoping. This is the chain the
// catalog.Provider.HasCredentials check actually walks in
// production: vault first, env as the universal fallback.
func CredentialLookup(cat *catalog.Catalog, vaultLookup func(string) string) func(string) string {
	return creds.ChainLookup(ScopedVaultLookup(cat, vaultLookup), os.Getenv)
}
