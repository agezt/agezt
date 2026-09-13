// SPDX-License-Identifier: MIT

package compat

// Credentials resolution for compat providers: resolveAzureCreds +
// vertexCreds type + resolveVertexCreds + isTruthy + bedrockAuth type +
// resolveBedrockCreds. Carved out of compat.go during the Day 165
// god-file split so the main file can focus on Build + family/env +
// URL + wrap helpers.
// Public API unchanged.

import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/plugins/providers/bedrock"
)
func resolveAzureCreds(p *catalog.Provider, lookup CredLookup) (resource, key string, err error) {
	if lookup == nil {
		return "", "", fmt.Errorf("%w: azure provider %q requires resource + api-key env vars (%v)",
			ErrMissingCredentials, p.ID, p.Env)
	}
	for _, name := range p.Env {
		v := strings.TrimSpace(providerEnvLookup(p, lookup, name))
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

// vertexCreds is the resolved Vertex AI auth + routing config. Exactly
// one auth path is selected: service-account JSON (credsPath set) or the
// GCE/GKE metadata server (useMetadata true).
type vertexCreds struct {
	credsPath   string // service-account JSON path (empty when useMetadata)
	project     string // GOOGLE_VERTEX_PROJECT (may be empty — filled later)
	location    string // GOOGLE_VERTEX_LOCATION (required)
	useMetadata bool   // use the GCE/GKE metadata server for ambient creds
	metadataURL string // GOOGLE_VERTEX_METADATA_URL override (empty → default)
}

// resolveVertexCreds extracts the Vertex AI credentials from the catalog
// entry's env list. Two auth paths:
//
//	GOOGLE_APPLICATION_CREDENTIALS — path to service-account JSON (desktop/CI)
//	GOOGLE_VERTEX_USE_METADATA     — "1"/"true"/"on"/"yes" → GCE/GKE metadata
//	                                 server (ambient/Workload-Identity creds)
//
// Always required:
//
//	GOOGLE_VERTEX_LOCATION — region (e.g. "us-central1")
//
// Optional:
//
//	GOOGLE_VERTEX_PROJECT      — falls back to the SA JSON project_id, or to
//	                             the metadata server's project-id endpoint
//	GOOGLE_VERTEX_METADATA_URL — override the metadata base URL (proxy/sidecar;
//	                             implies metadata auth)
func resolveVertexCreds(p *catalog.Provider, lookup CredLookup) (vertexCreds, error) {
	if lookup == nil {
		return vertexCreds{}, fmt.Errorf("%w: vertex provider %q requires GOOGLE_APPLICATION_CREDENTIALS (service-account JSON) or GOOGLE_VERTEX_USE_METADATA=1 (GCE/GKE), plus GOOGLE_VERTEX_LOCATION (%v)",
			ErrMissingCredentials, p.ID, p.Env)
	}
	vc := vertexCreds{
		credsPath:   strings.TrimSpace(providerEnvLookup(p, lookup, "GOOGLE_APPLICATION_CREDENTIALS")),
		project:     strings.TrimSpace(providerEnvLookup(p, lookup, "GOOGLE_VERTEX_PROJECT")),
		location:    strings.TrimSpace(providerEnvLookup(p, lookup, "GOOGLE_VERTEX_LOCATION")),
		useMetadata: isTruthy(providerEnvLookup(p, lookup, "GOOGLE_VERTEX_USE_METADATA")),
		metadataURL: strings.TrimSpace(providerEnvLookup(p, lookup, "GOOGLE_VERTEX_METADATA_URL")),
	}
	// A metadata URL override implies metadata auth — operators pointing
	// at a proxy/sidecar shouldn't also have to flip the boolean.
	if vc.metadataURL != "" {
		vc.useMetadata = true
	}
	if vc.credsPath == "" && !vc.useMetadata {
		return vertexCreds{}, fmt.Errorf("%w: vertex provider %q needs GOOGLE_APPLICATION_CREDENTIALS (service-account JSON) or GOOGLE_VERTEX_USE_METADATA=1 for GCE/GKE ambient credentials",
			ErrMissingCredentials, p.ID)
	}
	if vc.location == "" {
		return vertexCreds{}, fmt.Errorf("%w: vertex provider %q needs GOOGLE_VERTEX_LOCATION (e.g. us-central1)",
			ErrMissingCredentials, p.ID)
	}
	return vc, nil
}

// isTruthy reports whether an env value means "on". Accepts the common
// boolean spellings operators reach for (1/true/on/yes/enable), case- and
// whitespace-insensitive. Empty or anything else is false.
func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes", "enable", "enabled":
		return true
	}
	return false
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
		if v := strings.TrimSpace(providerEnvLookup(p, lookup, name)); v != "" {
			auth.Region = v
			break
		}
	}

	// Walk the catalog env list for the bearer token.
	for _, name := range p.Env {
		if strings.HasSuffix(name, "_BEARER_TOKEN_BEDROCK") {
			if v := strings.TrimSpace(providerEnvLookup(p, lookup, name)); v != "" {
				auth.Bearer = v
			}
		}
	}
	// Also accept the bare name (catalog name drift safety net).
	if auth.Bearer == "" {
		auth.Bearer = strings.TrimSpace(providerEnvLookup(p, lookup, "AWS_BEARER_TOKEN_BEDROCK"))
	}

	// SigV4 fallback — only check when bearer is absent. Both being
	// set is a real operator scenario (they tried both at some
	// point); we prefer bearer silently.
	if auth.Bearer == "" {
		akid := strings.TrimSpace(providerEnvLookup(p, lookup, "AWS_ACCESS_KEY_ID"))
		secret := strings.TrimSpace(providerEnvLookup(p, lookup, "AWS_SECRET_ACCESS_KEY"))
		sess := strings.TrimSpace(providerEnvLookup(p, lookup, "AWS_SESSION_TOKEN"))
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
