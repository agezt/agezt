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
// Vertex (M1.n) is service-account-OAuth-only (via
// GOOGLE_APPLICATION_CREDENTIALS) with Gemini body shape only.
// ADC / workload-identity / GCE metadata server and Anthropic-on-
// Vertex (`@ai-sdk/google-vertex/anthropic`, :rawPredict endpoint)
// land in M1.n.x.
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
			if v := strings.TrimSpace(lookup(name)); v != "" {
				apiKey = v
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
		// Vertex AI: service-account OAuth + Gemini body shape, on
		// the regional aiplatform.googleapis.com endpoint.
		// `@ai-sdk/google-vertex/anthropic` (Anthropic-on-Vertex via
		// :rawPredict) lands in M1.n.x.
		credsPath, project, location, err := resolveVertexCreds(p, lookup)
		if err != nil {
			return nil, "", err
		}
		sa, err := vertex.LoadServiceAccountFile(credsPath)
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
		ts, err := vertex.NewTokenSource(sa, vertex.CloudPlatformScope, nil)
		if err != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrMissingCredentials, err)
		}
		vp := vertex.New(ts, project, location)
		vp.BaseURL = strings.TrimSpace(p.API) // optional override
		vp.Model = modelID
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
		fullURL := urlBase + "/openai/deployments/" + modelID + "/chat/completions?api-version=" + apiVersion
		op := openai.New(azKey)
		op.Endpoint = fullURL // pinned: model+api-version+deployment baked in
		op.AuthHeader = "api-key"
		op.AuthScheme = "" // raw value, not Bearer
		op.Model = modelID
		return wrapNamed(p.ID, op), modelID, nil
	default:
		return nil, "", fmt.Errorf("%w: family=%q provider=%q (M1.n wired every catalog family — anthropic + ollama + openai + openai-compatible + google + mistral + cohere + azure + aws-bedrock + google-vertex. This branch should be unreachable for any models.dev catalog entry; if you see it, the catalog has a new family the wire layer doesn't recognise)",
			ErrFamilyUnsupported, p.Family(), p.ID)
	}
}

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
func resolveAzureCreds(p *catalog.Provider, lookup CredLookup) (resource, key string, err error) {
	if lookup == nil {
		return "", "", fmt.Errorf("%w: azure provider %q requires resource + api-key env vars (%v)",
			ErrMissingCredentials, p.ID, p.Env)
	}
	for _, name := range p.Env {
		v := strings.TrimSpace(lookup(name))
		if v == "" {
			continue
		}
		switch {
		case strings.HasSuffix(name, "_RESOURCE_NAME"):
			resource = v
		case strings.HasSuffix(name, "_API_KEY"):
			key = v
		}
	}
	if key == "" {
		return "", "", fmt.Errorf("%w: azure provider %q needs an *_API_KEY env var (one of %v)",
			ErrMissingCredentials, p.ID, p.Env)
	}
	// Resource is only required when the catalog `api` field is
	// empty — operators with a full custom URL don't need it.
	if resource == "" && strings.TrimSpace(p.API) == "" {
		return "", "", fmt.Errorf("%w: azure provider %q needs a *_RESOURCE_NAME env var (one of %v) or an `api` URL in custom.json",
			ErrMissingCredentials, p.ID, p.Env)
	}
	return resource, key, nil
}

// resolveVertexCreds extracts the Vertex AI credentials from the
// catalog entry's env list. M1.n supports service-account auth via
// GOOGLE_APPLICATION_CREDENTIALS (path to a JSON key file). Workload
// identity, ADC, and the GCE metadata server land in M1.n.x.
//
// Required env vars:
//
//	GOOGLE_APPLICATION_CREDENTIALS — path to service-account JSON
//	GOOGLE_VERTEX_LOCATION         — region (e.g. "us-central1")
//
// Optional:
//
//	GOOGLE_VERTEX_PROJECT — falls back to project_id in the JSON file
func resolveVertexCreds(p *catalog.Provider, lookup CredLookup) (credsPath, project, location string, err error) {
	if lookup == nil {
		return "", "", "", fmt.Errorf("%w: vertex provider %q requires GOOGLE_APPLICATION_CREDENTIALS + GOOGLE_VERTEX_LOCATION env vars (%v)",
			ErrMissingCredentials, p.ID, p.Env)
	}
	credsPath = strings.TrimSpace(lookup("GOOGLE_APPLICATION_CREDENTIALS"))
	project = strings.TrimSpace(lookup("GOOGLE_VERTEX_PROJECT"))
	location = strings.TrimSpace(lookup("GOOGLE_VERTEX_LOCATION"))
	if credsPath == "" {
		return "", "", "", fmt.Errorf("%w: vertex provider %q needs GOOGLE_APPLICATION_CREDENTIALS (path to service-account JSON; M1.n doesn't yet support ADC/workload-identity — those land in M1.n.x)",
			ErrMissingCredentials, p.ID)
	}
	if location == "" {
		return "", "", "", fmt.Errorf("%w: vertex provider %q needs GOOGLE_VERTEX_LOCATION (e.g. us-central1)",
			ErrMissingCredentials, p.ID)
	}
	// project may be empty here — compat will fall back to the
	// project_id baked into the service-account JSON.
	return credsPath, project, location, nil
}

// bedrockAuth carries whichever auth path the operator's environment
// supplied. Exactly one of {Bearer, SigV4} is populated by
// resolveBedrockCreds; the bedrock.Provider chooses between them at
// request time.
type bedrockAuth struct {
	Bearer string
	SigV4  *bedrock.SigV4Creds // nil when bearer path
	Region string
}

// resolveBedrockCreds extracts AWS auth + region from the catalog
// entry's env list. Two paths are supported (M1.m.x):
//
//   - **Bearer token** (AWS_BEARER_TOKEN_BEDROCK). Long-lived preview
//     credential; not all operators have access. Simpler wire-time
//     setup; preferred when available.
//   - **SigV4 static** (AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY,
//     optional AWS_SESSION_TOKEN for STS temp creds). Every Bedrock
//     account has this; works everywhere IAM does.
//
// If both are present in the vault, bearer wins — it's a one-header
// path with no per-request signing, less to go wrong.
//
// AWS_REGION (or AWS_DEFAULT_REGION) is required because Bedrock's
// host is regional. Operators can also pin a full `api` URL via
// custom.json to point at a region directly (region-required check
// is skipped in that case, but SigV4 still needs region for the
// credential scope, so SigV4 + no-region + custom-api remains an
// error).
func resolveBedrockCreds(p *catalog.Provider, lookup CredLookup) (bedrockAuth, error) {
	if lookup == nil {
		return bedrockAuth{}, fmt.Errorf("%w: bedrock provider %q requires AWS_BEARER_TOKEN_BEDROCK *or* (AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY), plus AWS_REGION (%v)",
			ErrMissingCredentials, p.ID, p.Env)
	}

	var auth bedrockAuth

	// Region first — needed by both auth paths.
	for _, name := range []string{"AWS_REGION", "AWS_DEFAULT_REGION"} {
		if v := strings.TrimSpace(lookup(name)); v != "" {
			auth.Region = v
			break
		}
	}

	// Walk the catalog env list for the bearer token.
	for _, name := range p.Env {
		if strings.HasSuffix(name, "_BEARER_TOKEN_BEDROCK") {
			if v := strings.TrimSpace(lookup(name)); v != "" {
				auth.Bearer = v
			}
		}
	}
	// Also accept the bare name (catalog name drift safety net).
	if auth.Bearer == "" {
		auth.Bearer = strings.TrimSpace(lookup("AWS_BEARER_TOKEN_BEDROCK"))
	}

	// SigV4 fallback — only check when bearer is absent. Both being
	// set is a real operator scenario (they tried both at some
	// point); we prefer bearer silently.
	if auth.Bearer == "" {
		akid := strings.TrimSpace(lookup("AWS_ACCESS_KEY_ID"))
		secret := strings.TrimSpace(lookup("AWS_SECRET_ACCESS_KEY"))
		sess := strings.TrimSpace(lookup("AWS_SESSION_TOKEN"))
		if akid != "" && secret != "" {
			auth.SigV4 = &bedrock.SigV4Creds{
				AccessKeyID:     akid,
				SecretAccessKey: secret,
				SessionToken:    sess,
			}
		}
	}

	if auth.Bearer == "" && auth.SigV4 == nil {
		return bedrockAuth{}, fmt.Errorf("%w: bedrock provider %q needs either AWS_BEARER_TOKEN_BEDROCK *or* (AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY) in the vault (env names searched: %v)",
			ErrMissingCredentials, p.ID, p.Env)
	}

	// Region required when (a) no `api` override pin, OR (b) SigV4 is
	// the auth path (signing needs region regardless of URL pin).
	regionRequired := strings.TrimSpace(p.API) == "" || auth.SigV4 != nil
	if auth.Region == "" && regionRequired {
		return bedrockAuth{}, fmt.Errorf("%w: bedrock provider %q needs AWS_REGION (or AWS_DEFAULT_REGION). SigV4 requires region for credential scope; bearer-token requires it for the host URL unless `api` override is set",
			ErrMissingCredentials, p.ID)
	}
	return auth, nil
}

// compatVendorBaseURL returns the stable OpenAI-compatible v1 base URL for a
// recognised vendor, keyed on the npm package the same way
// catalog.FamilyFromNPM classifies it — so the URL table and the family table
// agree on what counts as a known vendor. Returns "" for anything else, which
// keeps the empty-`api` guard in Build active for genuinely-unknown
// openai-compatible providers (M230).
//
// These URLs are the vendors' documented OpenAI-compatible roots. An operator
// can always override via the catalog `api` field (custom.json), which takes
// precedence, so a vendor that moves its endpoint is a one-line fix, not a
// rebuild.
func compatVendorBaseURL(npm string) string {
	n := strings.TrimSpace(strings.ToLower(npm))
	if n == "@openrouter/ai-sdk-provider" {
		return "https://openrouter.ai/api/v1"
	}
	switch strings.TrimPrefix(n, "@ai-sdk/") {
	case "groq":
		return "https://api.groq.com/openai/v1"
	case "xai":
		return "https://api.x.ai/v1"
	case "cerebras":
		return "https://api.cerebras.ai/v1"
	case "togetherai":
		return "https://api.together.xyz/v1"
	case "deepinfra":
		return "https://api.deepinfra.com/v1/openai"
	case "perplexity":
		return "https://api.perplexity.ai"
	case "fireworks":
		return "https://api.fireworks.ai/inference/v1"
	}
	return ""
}

// defaultBaseURL returns the well-known base URL for a family when the
// catalog's `api` field is empty. Only families with a single,
// universally-correct host get a default. The openai-compatible *family* has no
// single host (many vendors share it), so it returns "" here — per-vendor URLs
// come from compatVendorBaseURL instead, and a vendor with neither is caught by
// the empty-api guard in Build.
func defaultBaseURL(f catalog.Family) string {
	switch f {
	case catalog.FamilyAnthropic:
		// Includes the version segment: the anthropic adapter appends only
		// "/messages" (the @ai-sdk/anthropic convention models.dev follows).
		return "https://api.anthropic.com/v1"
	case catalog.FamilyOpenAI:
		return "https://api.openai.com/v1"
	case catalog.FamilyGoogle:
		return "https://generativelanguage.googleapis.com"
	case catalog.FamilyOllama:
		return "http://localhost:11434"
	case catalog.FamilyMistral:
		return "https://api.mistral.ai/v1"
	case catalog.FamilyCohere:
		return "https://api.cohere.com"
	}
	return ""
}

// FirstModelID returns a deterministic "first model" choice for a
// catalog entry: alphabetically smallest. The daemon uses this when
// AGEZT_MODEL is unset — operators get a working default rather than
// an error, and the choice is reproducible.
func FirstModelID(p *catalog.Provider) string {
	if p == nil || len(p.Models) == 0 {
		return ""
	}
	best := ""
	for id := range p.Models {
		if best == "" || id < best {
			best = id
		}
	}
	return best
}

// namedProvider wraps an inner agent.Provider so the Name() it reports
// matches the catalog provider id instead of the wire-family default
// ("anthropic", "ollama"). The Governor's registry is keyed on Name();
// keeping it aligned with the catalog id is what lets `agt catalog
// list` and the daemon's logs use the same identifier.
type namedProvider struct {
	name  string
	inner agent.Provider
}

func (n *namedProvider) Name() string { return n.name }
func (n *namedProvider) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	return n.inner.Complete(ctx, req)
}

// namedStreamingProvider is the streaming-aware variant of
// namedProvider. It's returned by wrapNamed when the inner provider
// implements agent.StreamingProvider, so type-asserting on the
// wrapped value preserves the inner's streaming capability.
//
// This is split into a sibling type rather than always implementing
// StreamingProvider on namedProvider because Go's interface
// satisfaction is structural — if namedProvider always had a
// CompleteStream method, every caller would see it as a
// StreamingProvider even when the inner doesn't support streaming.
// Two types lets the type assertion at the call site mean exactly
// what it says.
type namedStreamingProvider struct {
	namedProvider
	streamingInner agent.StreamingProvider
}

func (n *namedStreamingProvider) CompleteStream(ctx context.Context, req agent.CompletionRequest, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	return n.streamingInner.CompleteStream(ctx, req, onChunk)
}

// wrapNamed returns a wrapper that preserves the inner provider's
// capabilities. Always implements agent.Provider; additionally
// implements agent.StreamingProvider if the inner does.
func wrapNamed(name string, p agent.Provider) agent.Provider {
	if sp, ok := p.(agent.StreamingProvider); ok {
		return &namedStreamingProvider{
			namedProvider:  namedProvider{name: name, inner: p},
			streamingInner: sp,
		}
	}
	return &namedProvider{name: name, inner: p}
}
