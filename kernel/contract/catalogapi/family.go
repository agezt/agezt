// SPDX-License-Identifier: MIT

package catalogapi

import "strings"

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
func FamilyFromNPM(npm string) Family {
	n := strings.TrimSpace(strings.ToLower(npm))
	// Handle non-Vercel namespaces explicitly before stripping the
	// @ai-sdk/ prefix — they don't share it.
	switch n {
	case "@openrouter/ai-sdk-provider":
		return FamilyOpenAICompatible
	}
	n = strings.TrimPrefix(n, "@ai-sdk/")
	switch n {
	case "anthropic":
		return FamilyAnthropic
	case "openai":
		return FamilyOpenAI
	case "openai-compatible":
		return FamilyOpenAICompatible
	// First-party Vercel AI SDK packages whose wire dialect is OpenAI
	// Chat Completions — same Bearer-auth + /v1/chat/completions shape,
	// just hosted under a different base URL. The catalog entry's
	// `api` field carries that base URL.
	case "groq",
		"xai",
		"cerebras",
		"togetherai",
		"deepinfra",
		"perplexity",
		"fireworks",
		"deepseek",
		"moonshotai":
		return FamilyOpenAICompatible
	case "google", "google-generative-ai":
		return FamilyGoogle
	case "google-vertex", "google-vertex/anthropic":
		return FamilyGoogleVertex
	case "ollama":
		return FamilyOllama
	case "mistral":
		return FamilyMistral
	case "cohere":
		return FamilyCohere
	case "amazon-bedrock":
		return FamilyAWSBedrock
	case "azure":
		return FamilyAzure
	}
	return FamilyUnknown
}

// FamilySupportsNativeJSONMode reports whether a provider family has a native
// structured-output (JSON mode) switch that Agezt sends (M311/M312):
// response_format for the OpenAI-shaped families, generationConfig.responseMimeType
// for Gemini, format=json for Ollama. Families without one (Anthropic, Cohere,
// the Anthropic-on-Bedrock path) ignore a JSON-mode request and rely on a
// prompt-instructed-JSON fallback instead. (Family-level: the rare
// Anthropic-on-Vertex case — claude-* models under google-vertex — is the one
// exception this doesn't capture; it has no native JSON mode.)
func FamilySupportsNativeJSONMode(f Family) bool {
	switch f {
	case FamilyOpenAI, FamilyOpenAICompatible, FamilyMistral, FamilyAzure,
		FamilyGoogle, FamilyGoogleVertex, FamilyOllama:
		return true
	}
	return false
}

// Family is the resolved Family for this provider, derived from NPM.
func (p *Provider) Family() Family { return FamilyFromNPM(p.NPM) }
