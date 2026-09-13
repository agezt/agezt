// SPDX-License-Identifier: MIT

package compat

// Compatibility helpers: IsSupportedFamily + envLookup + providerEnvLookup.
// Carved out of compat.go during the Day 183 god-file split so the
// main file can focus on error vars + CredLookup + Build.
// Public API unchanged.

import (
	"github.com/agezt/agezt/kernel/catalog"
)

// IsSupportedFamily reports whether Build will accept the given family
// in this build. Used by the daemon's auto-pick to skip catalog
// entries we can't talk to.
func IsSupportedFamily(f catalog.Family) bool {
	switch f {
	case catalog.FamilyAnthropic,
		catalog.FamilyOllama,
		catalog.FamilyOpenAI,
		catalog.FamilyOpenAICompatible,
		catalog.FamilyGoogle,
		catalog.FamilyMistral,
		catalog.FamilyCohere,
		catalog.FamilyAzure,
		catalog.FamilyAWSBedrock,
		catalog.FamilyGoogleVertex:
		return true
	}
	return false
}

// envLookup is a nil-safe wrapper around CredLookup. compat's resolver
// is allowed to be nil for local-family providers; helpers that want
// an env var (Azure api-version, etc.) need a single-call accessor
// that doesn't panic.
func envLookup(lookup CredLookup, name string) string {
	if lookup == nil {
		return ""
	}
	return lookup(name)
}

func providerEnvLookup(p *catalog.Provider, lookup CredLookup, name string) string {
	if lookup == nil {
		return ""
	}
	for _, lookupName := range catalog.ProviderCredentialLookupNames(p.ID, name) {
		if v := lookup(lookupName); v != "" {
			return v
		}
	}
	return ""
}

// resolveAzureCreds extracts the Azure resource name and API key
// from the catalog entry's env list. Azure providers carry two
// credentials, not one — the standard "first non-empty wins" loop
// in Build only grabs one. We look at the env-var *names* to tell
// which is which (cohere/openai/anthropic only need one cred so they
// don't run this path).
//
// Recognised pairs (in priority order):
//
//	AZURE_RESOURCE_NAME                    + AZURE_API_KEY
//	AZURE_COGNITIVE_SERVICES_RESOURCE_NAME + AZURE_COGNITIVE_SERVICES_API_KEY
//
// Operators can also force a complete URL via the catalog `api`
// field in custom.json, which bypasses the resource-name lookup;
// in that case only the API key needs to be set.
