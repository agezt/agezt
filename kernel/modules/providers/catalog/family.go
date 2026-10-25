// SPDX-License-Identifier: MIT

package catalog

// Provenance: Catalog family + model/cost helpers: the names of the rows and
//             the rules that live in the contract layer
//             (kernel/contract/catalogapi). The rows and their helpers moved
//             there with the provider row; this file keeps the names.

import (
	"github.com/agezt/agezt/kernel/contract/catalogapi"
)

// FamilySupportsNativeJSONMode reports whether a provider family has a native
// structured-output (JSON mode) switch that Agezt sends (M311/M312):
// response_format for the OpenAI-shaped families, generationConfig.responseMimeType
// for Gemini, format=json for Ollama. Families without one (Anthropic, Cohere,
// the Anthropic-on-Bedrock path) ignore a JSON-mode request and rely on a
// prompt-instructed-JSON fallback instead. (Family-level: the rare
// Anthropic-on-Vertex case — claude-* models under google-vertex — is the one
// exception this doesn't capture; it has no native JSON mode.)
func FamilySupportsNativeJSONMode(f Family) bool {
	return catalogapi.FamilySupportsNativeJSONMode(f)
}

// Model is one model the provider offers.
//
// The row and its helpers live in the contract layer, with the provider row;
// this package keeps the names.
type Model = catalogapi.Model

// Modalities captures the I/O surfaces the model supports.
type Modalities = catalogapi.Modalities

// Limit captures the model's token windows.
type Limit = catalogapi.Limit

// Cost is USD per-million-tokens. Cache fields are optional and only
// populated for providers that surface prompt-caching prices
// separately (Anthropic, OpenAI).
type Cost = catalogapi.Cost

// Catalog is the in-memory union of every loaded source (api.json +
// local.json + custom.json), with custom > local > api precedence
// already applied. Safe to read concurrently; rebuild with Reload.
//
// The row and its rules live in the contract layer (W3.51): the layers above
// this module — the kernel, the run executor, the governor — may not name it,
// and a rule that only exists inside the module cannot be reached by them.
// This package keeps the name, so every existing caller is unchanged.
type Catalog = catalogapi.Catalog
