// SPDX-License-Identifier: MIT

// Package catalogapi is the contract layer's home for the provider catalog's
// rows: the description of one provider and the models it serves.
//
// The rows live here because the plugins that name them are PLUGINS, and a
// plugin may reach the contract and the platform layers and nothing else: a
// row a plugin has to name cannot live in a module's implementation package.
// The providers module keeps the names as aliases, and the catalog itself —
// the merge of the sources, the sync, the store — stays in the module.
//
// The rows' JSON field names mirror models.dev/api.json, because the catalog
// is fetched from there and unmarshalled directly. The wire is therefore a
// contract of its own, and it is pinned by a test.
package catalogapi

import "strings"

// Provider is one external service that can serve completions. Field
// names match models.dev/api.json so the JSON unmarshals directly.
type Provider struct {
	// ID is the stable lookup key, e.g. "anthropic", "openai", "ollama".
	ID string `json:"id"`
	// Name is the human label.
	Name string `json:"name"`
	// Env is the list of environment variable names that must be set
	// for this provider's credentials. Multiple entries mean any of
	// them is sufficient (e.g. ["ANTHROPIC_API_KEY", "CLAUDE_API_KEY"]).
	// Empty means the provider needs no credentials (local services).
	Env []string `json:"env,omitempty"`
	// NPM is the @ai-sdk/* package name the upstream catalog uses; we
	// repurpose it as a compatibility-family hint. See FamilyFromNPM.
	NPM string `json:"npm,omitempty"`
	// API is the base URL for the provider's HTTP endpoint.
	API string `json:"api,omitempty"`
	// Doc is the URL of the provider's documentation; surfaces in
	// `agt catalog list` so operators can find it.
	Doc string `json:"doc,omitempty"`
	// Models is the catalog of models this provider serves, keyed by
	// model ID.
	Models map[string]*Model `json:"models,omitempty"`
}

const providerCredentialPrefix = "provider:"

// ProviderCredentialName returns the vault key used for a credential scoped to a
// specific catalog provider. The underlying env var is still surfaced separately
// for compatibility with real process env vars and legacy vault entries.
func ProviderCredentialName(providerID, env string) string {
	providerID = strings.TrimSpace(providerID)
	env = strings.TrimSpace(env)
	if providerID == "" || env == "" {
		return env
	}
	return providerCredentialPrefix + providerID + ":" + env
}

// IsProviderCredentialName reports whether name is a provider-scoped vault key
// produced by ProviderCredentialName.
func IsProviderCredentialName(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), providerCredentialPrefix)
}

// ProviderCredentialLookupNames returns the lookup order for one provider/env
// pair: provider-scoped vault key first, then the legacy/global env name.
func ProviderCredentialLookupNames(providerID, env string) []string {
	env = strings.TrimSpace(env)
	scoped := ProviderCredentialName(providerID, env)
	if scoped == "" {
		return nil
	}
	if scoped == env {
		return []string{env}
	}
	return []string{scoped, env}
}

// HasCredentials reports whether any of the configured env-var names
// is set in env. Used by the Governor to filter the registry to
// providers we can actually call.
func (p *Provider) HasCredentials(lookup func(string) string) bool {
	if len(p.Env) == 0 {
		// No credentials required (local services like Ollama).
		return true
	}
	if lookup == nil {
		return false
	}
	for _, name := range p.Env {
		for _, lookupName := range ProviderCredentialLookupNames(p.ID, name) {
			if v := lookup(lookupName); v != "" {
				return true
			}
		}
	}
	return false
}
