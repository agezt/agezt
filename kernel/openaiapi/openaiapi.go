// SPDX-License-Identifier: MIT

// OpenAI-compatible HTTP surface: redactErr + errRedactor + maxRequestBodyBytes + decodeBody + chatUsage.
// Code extracted from openaiapi.go during the Day-49 god-file split. Public API unchanged.
package openaiapi


import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	kernelauth "github.com/agezt/agezt/kernel/auth"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/redact"
)



// errRedactor scrubs known secret patterns (API keys, tokens, bearer creds) from
// error strings before they are returned to the HTTP client (VULN-012). Raw
// upstream provider / STT errors are echoed back on the upstream_error and
// stt_error paths; a pathological upstream error could surface a request fragment
// or credential. The recipient is the already-privileged operator, so this is
// defense-in-depth — but live HTTP responses bypass the journal redactor, so we
// scrub here too. The built-in pattern set is active without configured literals,
// and the Redactor is safe for concurrent use.
var errRedactor = redact.New()

// redactErr returns msg with secret-shaped spans masked. Safe on any string.
func redactErr(msg string) string { return errRedactor.Redact(msg) }

// maxRequestBodyBytes caps an HTTP request body (M198). The OpenAI-compatible
// surface is network-exposed and token-authed, but a token holder (or a
// compromised/buggy client) must not be able to OOM the daemon with a giant JSON
// body. 16 MiB is far above any legitimate chat/responses request.
const maxRequestBodyBytes = 16 << 20

// decodeBody reads and decodes a SIZE-BOUNDED JSON request body (M198). On
// failure it writes the appropriate error (413 over the limit, else 400) and
// returns false, so callers just `if !decodeBody(...) { return }`.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, "invalid_request_error",
				"request body exceeds the size limit")
		} else {
			writeErr(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body: "+err.Error())
		}
		return false
	}
	return true
}

// Engine is the slice of the kernel this server drives. An interface keeps the
// package testable with a fake (a canned RunWith that publishes token events on
// a real in-memory bus exercises the SSE path without a daemon).
type Engine interface {
	NewCorrelation() string
	SubjectForRun(corr string) string
	// RunModel runs the intent under the given correlation, honouring the
	// requested model (empty → the kernel's configured default). images carries
	// any attachment URLs parsed from a multimodal request (data: URLs or
	// http(s) URLs); nil for a text-only run. jsonMode requests structured JSON
	// output (M314), set from the request's response_format.
	RunModel(ctx context.Context, corr, intent, model string, images []string, jsonMode bool) (string, error)
	DefaultModel() string
	ModelIDs() []string
}

// UsageReporter is an optional Engine capability: report the REAL provider token
// usage for a completed run, summed across the run's LLM calls (folded from the
// journal's budget.consumed events). When an Engine implements it, the API's
// usage block reflects provider truth instead of the whitespace estimate. ok is
// false when no usage was recorded (a free/local/mock model, or the run had no
// priced call) — callers fall back to estimateUsage.
type UsageReporter interface {
	UsageFor(corr string) (promptTokens, completionTokens int, ok bool)
}

// chatUsage returns the usage block for a chat completion: the real provider
// usage when the engine can report it, else the whitespace estimate.
func chatUsage(eng Engine, corr, intent, answer string) map[string]any {
	if ur, ok := eng.(UsageReporter); ok {
		if pt, ct, ok := ur.UsageFor(corr); ok {
			return map[string]any{"prompt_tokens": pt, "completion_tokens": ct, "total_tokens": pt + ct}
		}
	}
	return estimateUsage(intent, answer)
}

// TenantResolver maps a tenant id to the Engine + bus that serve it. The daemon
// injects one (backed by the tenant registry) when multi-tenancy is enabled.
type TenantResolver func(tenant string) (Engine, *bus.Bus, error)

// TenantAuthorizer reports whether presented is the per-tenant credential of
// tenant id. It lets a scoped per-tenant token authorize requests against ONLY
// its own tenant, while the daemon admin token authorizes any tenant.
type TenantAuthorizer func(tenant, presented string) bool

// Server is the OpenAI-compatible HTTP surface.
type Server struct {
	eng      Engine
	bus      *bus.Bus
	verifier kernelauth.Verifier

	// resolve, when set, maps the X-Agezt-Tenant request header to a per-tenant
	// Engine + bus. Nil (or an empty header) means the primary engine/bus —
	// the unchanged single-tenant path.
	resolve TenantResolver

	// tenantAuth, when set, validates a per-tenant token against the tenant named
	// in the X-Agezt-Tenant header. Nil means only the admin token authorizes.
	tenantAuth TenantAuthorizer

	// transcriber, when set, backs POST /v1/audio/transcriptions. Nil means the
	// route reports "not configured" (no STT endpoint is wired).
	transcriber Transcriber
}

// Transcriber turns uploaded audio into text — satisfied by *stt.Client. Lets the
// OpenAI-compatible API expose /v1/audio/transcriptions without importing the STT
// package (kept dependency-light + testable with a fake).
type Transcriber interface {
	Transcribe(ctx context.Context, filename string, audio []byte) (string, error)
}
