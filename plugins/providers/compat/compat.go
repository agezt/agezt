// SPDX-License-Identifier: MIT

// Package compat builds a wire `agent.Provider` from a `catalog.Provider`
// entry — no per-provider Go package needed. The Family hint on the
// catalog entry (derived from the upstream `npm` field, see
// kernel/catalog.FamilyFromNPM) picks the right adapter, the `api`
// field supplies the base URL, and the `env` list resolves
// credentials via the supplied lookup function.
//
// **Families supported today:**
//
//	FamilyAnthropic        → plugins/providers/anthropic (Messages API)
//	FamilyOllama           → plugins/providers/ollama    (/api/chat)
//	FamilyOpenAI           → plugins/providers/openai    (Chat Completions)
//	FamilyOpenAICompatible → plugins/providers/openai    (Chat Completions)
//	FamilyGoogle           → plugins/providers/google    (generateContent, API key)
//	FamilyMistral          → plugins/providers/openai    (Chat Completions, api.mistral.ai)
//	FamilyCohere           → plugins/providers/cohere    (v2/chat)
//	FamilyAzure            → plugins/providers/openai    (Chat Completions, api-key header,
//	                                                       resource+deployment URL builder)
//	FamilyAWSBedrock       → plugins/providers/bedrock   (Anthropic-on-Bedrock,
//	                                                       AWS_BEARER_TOKEN_BEDROCK auth)
//	FamilyGoogleVertex     → plugins/providers/vertex    (Gemini-on-Vertex,
//	                                                       service-account OAuth)
//
// Mistral and Azure are folded into the OpenAI adapter — both speak
// openai-shaped /chat/completions on the wire. Azure differs only in
// URL structure (resource-specific subdomain, deployment-in-path,
// ?api-version=...) and auth header (`api-key` instead of `Bearer`).
// The openai adapter's optional AuthHeader/AuthScheme fields handle
// the auth swap; compat builds the URL.
//
// Bedrock (M1.m) is bearer-token-only and Anthropic-body-shape-only.
// SigV4-signed requests and non-Anthropic vendor bodies (Mistral,
// Meta, Amazon Titan, Cohere, AI21, DeepSeek on Bedrock) land in
// M1.m.x.
//
// Vertex speaks the Gemini body shape and authenticates two ways:
// a service-account JSON key (GOOGLE_APPLICATION_CREDENTIALS) for
// desktop/CI, or the GCE/GKE instance metadata server
// (GOOGLE_VERTEX_USE_METADATA=1) for ambient production credentials on
// Compute Engine / GKE Workload Identity / Cloud Run. External/federated
// ADC remains unimplemented. Anthropic-on-Vertex (`claude-*` models via the
// :rawPredict endpoint) IS supported: the vertex adapter dispatches on the
// model id, so a claude-* model under the google-vertex family routes to the
// Anthropic Messages body automatically.
//
// **Every family in the catalog is now wired.** Adding a new
// downstream variant is one extra case branch + adapter; the daemon
// and the catalog don't change.
//
// `Build` returns a wrapped Provider whose `Name()` is the
// catalog provider id (e.g. "anthropic", "groq", "ollama-local"),
// not the wire-family name — so the Governor's registry sees one
// entry per catalog provider, not one per family.
package compat

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/plugins/providers/anthropic"
	"github.com/agezt/agezt/plugins/providers/bedrock"
	"github.com/agezt/agezt/plugins/providers/cohere"
	"github.com/agezt/agezt/plugins/providers/google"
	"github.com/agezt/agezt/plugins/providers/ollama"
	"github.com/agezt/agezt/plugins/providers/openai"
	"github.com/agezt/agezt/plugins/providers/vertex"
)


// ErrFamilyUnsupported is returned by Build when the catalog entry's
// family isn't yet wired (OpenAI, Google, etc. — see package docs).
var ErrFamilyUnsupported = errors.New("compat: provider family not yet supported")

// ErrMissingCredentials is returned when the catalog entry lists
// env-var credentials but none of them are set in the lookup.
var ErrMissingCredentials = errors.New("compat: no credentials available")

// ErrModelUnknown is returned when the requested modelID isn't in the
// provider's models map. Build is strict: we don't guess.
var ErrModelUnknown = errors.New("compat: model not in this provider's catalog entry")

// CredLookup is a strategy for resolving env-var names to values. The
// daemon passes os.Getenv; tests pass an in-memory map. Empty string
// means "not set"; HasCredentials uses the same convention.
type CredLookup func(name string) string

// Build constructs a Provider for the given catalog entry + model id.
//
//   - p:          a catalog.Provider (typically from Kernel.Catalog())
//   - modelID:    the model the agent loop will pass in
//     CompletionRequest.Model. Must exist in p.Models.
//   - lookup:     credential resolver; nil is treated as "no creds set"
//
// Returns (provider, modelID-used, error). The returned Provider's
// Name() reports the catalog provider id so the Governor's registry
// stays keyed on stable, catalog-aligned names.
func Build(p *catalog.Provider, modelID string, lookup CredLookup) (agent.Provider, string, error) {
	if p == nil {
		return nil, "", errors.New("compat: nil provider entry")
	}
	if modelID == "" {
		return nil, "", errors.New("compat: model id required")
	}
	if _, ok := p.Models[modelID]; !ok {
		return nil, "", fmt.Errorf("%w: %q has no model %q", ErrModelUnknown, p.ID, modelID)
	}

	// Resolve credentials. Local-family providers (no env list) skip
	// this step entirely; that's how Ollama-local works.
	apiKey := ""
	if len(p.Env) > 0 {
		if lookup == nil {
			return nil, "", fmt.Errorf("%w: %q needs one of %v", ErrMissingCredentials, p.ID, p.Env)
		}
		for _, name := range p.Env {
			for _, lookupName := range catalog.ProviderCredentialLookupNames(p.ID, name) {
				if v := strings.TrimSpace(lookup(lookupName)); v != "" {
					apiKey = v
					break
				}
			}
			if apiKey != "" {
				break
			}
		}
		if apiKey == "" {
			return nil, "", fmt.Errorf("%w: %q needs one of %v", ErrMissingCredentials, p.ID, p.Env)
		}
	}

	// Resolve the base URL. models.dev leaves `api` empty for some
	// vendors whose URL is well-known to their first-party AI SDK
	// package (anthropic, openai, mistral, gemini); compat carries
	// those defaults so operators don't have to add custom.json
	// entries just to get a working setup. See defaultBaseURL.
	base := strings.TrimSpace(p.API)
	if base == "" {
		base = defaultBaseURL(p.Family())
	}
	// For the well-known OpenAI-compatible vendors agezt already enumerates
	// (catalog.FamilyFromNPM), carry their stable base URL too (M230) — so a
	// `groq`/`xai`/`cerebras`/… provider works with just an API key, no
	// custom.json URL entry. An explicit catalog `api` still wins (set above);
	// this is only the empty-`api` fallback, and an unrecognised compat vendor
	// still hits the guard below.
	if base == "" && p.Family() == catalog.FamilyOpenAICompatible {
		base = compatVendorBaseURL(p.NPM)
	}
	switch p.Family() {
	case catalog.FamilyAnthropic:
		ap := anthropic.New(apiKey)
		ap.BaseURL = base
		ap.Endpoint = "" // force BaseURL-derived path
		ap.Model = modelID
		// Extended thinking (M318): opt-in via AGEZT_ANTHROPIC_THINKING_BUDGET.
		// Off by default (thinking costs extra tokens); the chain of thought is
		// captured into the response's ReasoningContent + ephemeral llm.reasoning
		// events (M317).
		if v := strings.TrimSpace(envLookup(lookup, "AGEZT_ANTHROPIC_THINKING_BUDGET")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				ap.ThinkingBudget = n
			}
		}
		return wrapNamed(p.ID, ap), modelID, nil
	case catalog.FamilyOllama:
		op := ollama.New()
		op.BaseURL = base
		op.Endpoint = "" // force BaseURL-derived path
		op.Model = modelID
		return wrapNamed(p.ID, op), modelID, nil
	case catalog.FamilyGoogle:
		// Gemini's generateContent API. Vertex (OAuth) is a separate
		// family (FamilyGoogleVertex) and falls through to the
		// unsupported branch below.
		gp := google.New(apiKey)
		gp.BaseURL = base
		gp.Endpoint = "" // force BaseURL-derived path
		gp.Model = modelID
		// Thinking (M319, 2.5-series): opt-in via AGEZT_GOOGLE_THINKING_BUDGET.
		// Off by default (thinking costs extra tokens, billed as output). A
		// positive value caps the thinking tokens; -1 asks Gemini for a
		// dynamic budget. The summaries land on ReasoningContent + ephemeral
		// llm.reasoning events (M317), same pipeline as DeepSeek-R1/Claude.
		if v := strings.TrimSpace(envLookup(lookup, "AGEZT_GOOGLE_THINKING_BUDGET")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n != 0 {
				gp.ThinkingBudget = n
			}
		}
		return wrapNamed(p.ID, gp), modelID, nil
	case catalog.FamilyOpenAI, catalog.FamilyOpenAICompatible:
		// One adapter, two families: real OpenAI and openai-compatible
		// (Groq, DeepSeek, Together, OpenRouter, xAI, Fireworks, …).
		// Both use Bearer auth + /v1/chat/completions; only base URL
		// and env-var name differ, and those come from the catalog.
		//
		// For openai-compatible specifically, refuse an empty `api`:
		// the adapter's default endpoint points at api.openai.com,
		// which would silently route an unknown vendor's traffic to the
		// wrong host. Known vendors (groq, xai, …) were already filled in
		// above by compatVendorBaseURL; this catches the rest. Operators
		// set the URL via custom.json.
		if p.Family() == catalog.FamilyOpenAICompatible && strings.TrimSpace(base) == "" {
			return nil, "", fmt.Errorf("%w: provider %q is openai-compatible but has no `api` URL in the catalog — add it via custom.json",
				ErrFamilyUnsupported, p.ID)
		}
		op := openai.New(apiKey)
		op.BaseURL = base
		op.Endpoint = "" // force BaseURL-derived path
		op.Model = modelID
		return wrapNamed(p.ID, op), modelID, nil
	case catalog.FamilyMistral:
		// api.mistral.ai/v1 is wire-identical to OpenAI Chat
		// Completions — Bearer auth, same body/response, same tool
		// shape. Reuse the openai adapter; the per-family default
		// base URL comes from defaultBaseURL.
		mp := openai.New(apiKey)
		mp.BaseURL = base
		mp.Endpoint = "" // force BaseURL-derived path
		mp.Model = modelID
		return wrapNamed(p.ID, mp), modelID, nil
	case catalog.FamilyCohere:
		// Cohere v2 /v2/chat — Bearer auth, openai-shaped messages,
		// but content-as-blocks on responses and nested usage. Its
		// own adapter handles the translation.
		cp := cohere.New(apiKey)
		cp.BaseURL = base
		cp.Endpoint = "" // force BaseURL-derived path
		cp.Model = modelID
		return wrapNamed(p.ID, cp), modelID, nil
	case catalog.FamilyGoogleVertex:
		// Vertex AI: Gemini body shape on the regional
		// aiplatform.googleapis.com endpoint, authenticated by either a
		// service-account JSON key or the GCE/GKE metadata server. The same
		// adapter also serves Anthropic-on-Vertex: a `claude-*` model id makes
		// vertex.Provider dispatch to the Anthropic Messages body on the
		// :rawPredict endpoint (no separate family needed).
		vc, err := resolveVertexCreds(p, lookup)
		if err != nil {
			return nil, "", err
		}
		var ts vertex.TokenMinter
		project := vc.project
		if vc.useMetadata {
			// Ambient credentials: the platform's metadata server mints
			// short-lived tokens — no key file on disk.
			mts := vertex.NewMetadataTokenSource(vc.metadataURL, nil)
			ts = mts
			// Fill the project from the same metadata server when the
			// operator didn't pin one (the fully ambient experience).
			if project == "" {
				id, perr := mts.ProjectID(context.Background())
				if perr != nil {
					return nil, "", fmt.Errorf("%w: vertex provider %q: no GOOGLE_VERTEX_PROJECT set and metadata project-id lookup failed: %v",
						ErrMissingCredentials, p.ID, perr)
				}
				project = id
			}
		} else {
			sa, err := vertex.LoadServiceAccountFile(vc.credsPath)
			if err != nil {
				return nil, "", fmt.Errorf("%w: %v", ErrMissingCredentials, err)
			}
			// Prefer the project_id baked into the SA JSON when the
			// env-supplied project is empty.
			if project == "" {
				project = sa.ProjectID
			}
			if project == "" {
				return nil, "", fmt.Errorf("%w: vertex provider %q needs a project (GOOGLE_VERTEX_PROJECT env or project_id in service-account JSON)",
					ErrMissingCredentials, p.ID)
			}
			sats, err := vertex.NewTokenSource(sa, vertex.CloudPlatformScope, nil)
			if err != nil {
				return nil, "", fmt.Errorf("%w: %v", ErrMissingCredentials, err)
			}
			ts = sats
		}
		vp := vertex.New(ts, project, vc.location)
		vp.BaseURL = strings.TrimSpace(p.API) // optional override
		vp.Model = modelID
		// Gemini thinking on Vertex (M320): opt-in via
		// AGEZT_GOOGLE_VERTEX_THINKING_BUDGET (distinct from the Generative
		// Language API's AGEZT_GOOGLE_THINKING_BUDGET — Vertex is a separate
		// billing/credential surface). Native-Gemini path only; -1 = dynamic.
		if v := strings.TrimSpace(envLookup(lookup, "AGEZT_GOOGLE_VERTEX_THINKING_BUDGET")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n != 0 {
				vp.ThinkingBudget = n
			}
		}
		return wrapNamed(p.ID, vp), modelID, nil
	case catalog.FamilyAWSBedrock:
		// Bedrock M1.m: bearer-token auth + Anthropic body shape only.
		// SigV4 signing and non-Anthropic vendor bodies (Mistral,
		// Meta, Amazon Titan, Cohere, AI21, DeepSeek) land in M1.m.x.
		auth, err := resolveBedrockCreds(p, lookup)
		if err != nil {
			return nil, "", err
		}
		bp := bedrock.New(auth.Bearer, auth.Region)
		bp.BaseURL = strings.TrimSpace(p.API) // optional override
		bp.Model = modelID
		// Switch to SigV4 if the operator's vault has IAM creds
		// (and no bearer token). bedrock.Provider's request paths
		// inspect both auth fields and pick bearer when set.
		if auth.SigV4 != nil {
			bp.SetSigV4Creds(auth.SigV4)
		}
		return wrapNamed(p.ID, bp), modelID, nil
	case catalog.FamilyAzure:
		// Azure OpenAI Service: openai-shaped body, but the URL is
		// resource+deployment-specific and auth is `api-key` (no
		// scheme prefix). The catalog `env` list carries two
		// credentials (resource name + API key); the standard apiKey
		// resolver above already grabbed *one* — find the other.
		resource, azKey, err := resolveAzureCreds(p, lookup)
		if err != nil {
			return nil, "", err
		}
		urlBase := strings.TrimSpace(p.API)
		if urlBase == "" {
			urlBase = "https://" + resource + ".openai.azure.com"
		}
		urlBase = strings.TrimRight(urlBase, "/")
		apiVersion := strings.TrimSpace(envLookup(lookup, "AGEZT_AZURE_API_VERSION"))
		if apiVersion == "" {
			// Latest GA-equivalent stable api-version at the project's
			// knowledge cutoff. Operators on cutting-edge previews
			// override via AGEZT_AZURE_API_VERSION.
			apiVersion = "2024-10-21"
		}
		// Escape the deployment name into the path and the api-version into
		// the query: a deployment id (or a catalog-supplied model id) bearing
		// a space, '/', '?' or '#' would otherwise produce a malformed URL or
		// smuggle an extra query parameter ahead of api-version. PathEscape
		// keeps ordinary alphanumeric Azure names byte-identical.
		fullURL := urlBase + "/openai/deployments/" + url.PathEscape(modelID) + "/chat/completions?api-version=" + url.QueryEscape(apiVersion)
		op := openai.New(azKey)
		op.Endpoint = fullURL // pinned: model+api-version+deployment baked in
		op.AuthHeader = "api-key"
		op.AuthScheme = "" // raw value, not Bearer
		op.Model = modelID
		return wrapNamed(p.ID, op), modelID, nil
	default:
		// Reached when a provider's npm package isn't classified by
		// catalog.FamilyFromNPM (e.g. a new @ai-sdk/<vendor> package agezt
		// doesn't enumerate yet — this is how DeepSeek and Moonshot looked
		// before they were wired). Most such providers speak the OpenAI API,
		// so point the operator at the generic-compat escape hatch rather than
		// claiming the case is impossible.
		return nil, "", fmt.Errorf("%w: provider %q has an unrecognised npm package (family=%q). If it speaks the OpenAI API (most do), set its npm to %q in custom.json to route it through the openai-compatible adapter",
			ErrFamilyUnsupported, p.ID, p.Family(), "openai-compatible")
	}
}

