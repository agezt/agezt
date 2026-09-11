// SPDX-License-Identifier: MIT

// Catalog types + Provider credential helpers: types, ProviderCredentialName, IsProviderCredentialName, ProviderCredentialLookupNames, DuplicateCredentialEnvs, HasCredentials.
// Code extracted from types.go during the Day-57 god-file split. Public API unchanged.
package catalog


import (
	"strings"
)



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

// DuplicateCredentialEnvs returns env-var names used by more than one provider
// in this catalog. Callers use it to avoid treating a legacy bare vault entry as
// credentials for every provider that happens to share the same upstream env
// name. Real process env vars remain global; this only informs vault fallback.
func (c *Catalog) DuplicateCredentialEnvs() map[string]bool {
	out := map[string]bool{}
	if c == nil {
		return out
	}
	counts := map[string]int{}
	for _, p := range c.Providers {
		if p == nil {
			continue
		}
		seen := map[string]bool{}
		for _, env := range p.Env {
			env = strings.TrimSpace(env)
			if env == "" || seen[env] {
				continue
			}
			seen[env] = true
			counts[env]++
		}
	}
	for env, count := range counts {
		if count > 1 {
			out[env] = true
		}
	}
	return out
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

// Family is the wire-dialect family — what adapter (Anthropic Messages,
// OpenAI Chat Completions, Ollama /api/chat, etc.) the Governor uses
// to talk to this Provider.
type Family string

const (
	FamilyAnthropic        Family = "anthropic"
	FamilyOpenAI           Family = "openai"
	FamilyOpenAICompatible Family = "openai-compatible"
	FamilyGoogle           Family = "google"        // Generative Language API (API key)
	FamilyGoogleVertex     Family = "google-vertex" // Vertex AI (service-account OAuth)
	FamilyOllama           Family = "ollama"
	FamilyMistral          Family = "mistral"
	FamilyCohere           Family = "cohere"
	FamilyAWSBedrock       Family = "aws-bedrock"
	FamilyAzure            Family = "azure"
	FamilyUnknown          Family = "unknown"
)

// FamilyFromNPM maps an `@ai-sdk/*` package name to one of our known
// compat families. The mapping is conservative: anything not
// recognised falls to FamilyUnknown so the Governor can refuse it
// rather than guess wrong.