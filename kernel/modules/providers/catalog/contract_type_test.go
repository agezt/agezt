// SPDX-License-Identifier: MIT

package catalog

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/catalogapi"
)

// The module's rows ARE the contract's: the plugins that name them (the
// compatibility adapters) are PLUGINS, and a plugin may only reach the
// contract. Asserted as function types — an assignment would also accept a
// copy with the same fields, while a function type requires identity.
var _ func(catalogapi.Provider) = func(Provider) {}
var _ func(catalogapi.Model) = func(Model) {}
var _ func(catalogapi.Family) = func(Family) {}
var _ func(catalogapi.Modalities) = func(Modalities) {}
var _ func(catalogapi.Limit) = func(Limit) {}
var _ func(catalogapi.Cost) = func(Cost) {}
var _ func(catalogapi.Catalog) = func(Catalog) {}
var _ func(catalogapi.Meta) = func(Meta) {}
var _ func(catalogapi.SyncResult) = func(SyncResult) {}

// The module's store and syncer answer the CONTRACT's ports (W3.51): the
// daemon opens the catalog directory and hands the kernel the ports, and the
// kernel may not name this module.
//
// The compile-time assertions are the point: a port that grew a verb would
// break HERE, in the module that implements it — not in the daemon.
var _ catalogapi.Store = (*Store)(nil)
var _ catalogapi.Syncer = (*Syncer)(nil)

// TestTheRulesAreTheContractsRules: the module's names are one-line
// delegations to the contract's rules, and they must agree — a second copy
// of the mapping would drift silently, and the mapping decides which wire
// dialect a provider is spoken to.
//
// The family mapping itself is the contract's alone: the module no longer
// uses it, so it keeps no copy of the name.
func TestTheRulesAreTheContractsRules(t *testing.T) {
	for _, npm := range []string{
		"", "@ai-sdk/anthropic", "@ai-sdk/openai", "@ai-sdk/openai-compatible",
		"@openrouter/ai-sdk-provider", "@ai-sdk/groq", "@ai-sdk/deepseek",
		"@ai-sdk/google", "@ai-sdk/google-vertex", "@ai-sdk/ollama",
		"@ai-sdk/mistral", "@ai-sdk/cohere", "@ai-sdk/amazon-bedrock",
		"@ai-sdk/azure", "@ai-sdk/unknown", "@AI-SDK/Anthropic",
	} {
		if got, want := (&Provider{ID: "p", NPM: npm}).Family(), catalogapi.FamilyFromNPM(npm); got != want {
			t.Errorf("Provider.Family() for %q = %q, the contract says %q", npm, got, want)
		}
	}
	for _, f := range []Family{
		FamilyAnthropic, FamilyOpenAI, FamilyOpenAICompatible, FamilyGoogle,
		FamilyGoogleVertex, FamilyOllama, FamilyMistral, FamilyCohere,
		FamilyAWSBedrock, FamilyAzure, FamilyUnknown,
	} {
		if got, want := FamilySupportsNativeJSONMode(f), catalogapi.FamilySupportsNativeJSONMode(f); got != want {
			t.Errorf("FamilySupportsNativeJSONMode(%q) = %v, the contract says %v", f, got, want)
		}
	}
}

// TestTheCredentialNamesAreTheContractsNames: the vault key shape is read by
// the credential store, so the two must agree.
func TestTheCredentialNamesAreTheContractsNames(t *testing.T) {
	for _, tc := range []struct{ provider, env string }{
		{"anthropic", "ANTHROPIC_API_KEY"},
		{"", "ANTHROPIC_API_KEY"},
		{"anthropic", ""},
		{"  ", "  "},
	} {
		if got, want := ProviderCredentialName(tc.provider, tc.env), catalogapi.ProviderCredentialName(tc.provider, tc.env); got != want {
			t.Errorf("ProviderCredentialName(%q,%q) = %q, the contract says %q", tc.provider, tc.env, got, want)
		}
		got := ProviderCredentialLookupNames(tc.provider, tc.env)
		want := catalogapi.ProviderCredentialLookupNames(tc.provider, tc.env)
		if len(got) != len(want) {
			t.Errorf("ProviderCredentialLookupNames(%q,%q) = %v, the contract says %v", tc.provider, tc.env, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("ProviderCredentialLookupNames(%q,%q) = %v, the contract says %v", tc.provider, tc.env, got, want)
				break
			}
		}
	}
	if IsProviderCredentialName("provider:anthropic:ANTHROPIC_API_KEY") != catalogapi.IsProviderCredentialName("provider:anthropic:ANTHROPIC_API_KEY") {
		t.Error("IsProviderCredentialName disagrees with the contract")
	}
}
