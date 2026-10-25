// SPDX-License-Identifier: MIT

package catalog

// Provenance: Catalog types + Provider credential helpers: types,
//             ProviderCredentialName, IsProviderCredentialName,
//             ProviderCredentialLookupNames, DuplicateCredentialEnvs.
//             The rows and their rules live in the contract layer
//             (kernel/contract/catalogapi); this file keeps the names.

import (
	"github.com/agezt/agezt/kernel/contract/catalogapi"
)

// Provider is one external service that can serve completions.
//
// The row itself lives in the contract layer (kernel/contract/catalogapi): the
// plugins that name it are plugins, and a plugin may only reach the contract.
// This package keeps the name, so every existing caller is unchanged. The
// catalog — the merge of the sources, the sync, the store — stays here.
type Provider = catalogapi.Provider

// ProviderCredentialName returns the vault key used for a credential scoped to a
// specific catalog provider. The rule lives in the contract layer, with the
// row; this package keeps the name.
func ProviderCredentialName(providerID, env string) string {
	return catalogapi.ProviderCredentialName(providerID, env)
}

// IsProviderCredentialName reports whether name is a provider-scoped vault key
// produced by ProviderCredentialName.
func IsProviderCredentialName(name string) bool {
	return catalogapi.IsProviderCredentialName(name)
}

// ProviderCredentialLookupNames returns the lookup order for one provider/env
// pair: provider-scoped vault key first, then the legacy/global env name.
func ProviderCredentialLookupNames(providerID, env string) []string {
	return catalogapi.ProviderCredentialLookupNames(providerID, env)
}

// Family is the wire-dialect family — what adapter (Anthropic Messages,
// OpenAI Chat Completions, Ollama /api/chat, etc.) the Governor uses
// to talk to this Provider.
//
// The type and its constants live in the contract layer, with the row.
type Family = catalogapi.Family

const (
	FamilyAnthropic        = catalogapi.FamilyAnthropic
	FamilyOpenAI           = catalogapi.FamilyOpenAI
	FamilyOpenAICompatible = catalogapi.FamilyOpenAICompatible
	FamilyGoogle           = catalogapi.FamilyGoogle       // Generative Language API (API key)
	FamilyGoogleVertex     = catalogapi.FamilyGoogleVertex // Vertex AI (service-account OAuth)
	FamilyOllama           = catalogapi.FamilyOllama
	FamilyMistral          = catalogapi.FamilyMistral
	FamilyCohere           = catalogapi.FamilyCohere
	FamilyAWSBedrock       = catalogapi.FamilyAWSBedrock
	FamilyAzure            = catalogapi.FamilyAzure
	FamilyUnknown          = catalogapi.FamilyUnknown
)

// The family a provider belongs to is derived from its NPM package name —
// the rule lives in the contract layer, with the row (catalogapi.FamilyFromNPM).
