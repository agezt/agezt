# M378 — OpenAI-compatible `GET /v1/models/{id}` retrieve (SPEC-15 §3 / SPEC-16 §1.1)

## SPEC audit (read-vs-code)
SPEC-16 §1.1 and SPEC-15 §3 require an **OpenAI-compatible API** so "any OpenAI
client, SDK, or IDE can drive Agezt as if it were OpenAI." The OpenAI Models API
has two operations: **list** (`GET /v1/models`) and **retrieve**
(`GET /v1/models/{id}`). The official OpenAI Python/JS SDKs issue the latter from
`client.models.retrieve(id)`, which clients and IDE integrations use for
capability probing before a chat call.

**Verified gap:** `kernel/openaiapi/openaiapi.go` registered the list route as
`mux.HandleFunc("/v1/models", …)` — an **exact** match in Go's `ServeMux`. A
request to `GET /v1/models/{id}` matched no handler and fell through to the
default `404 page not found` (plain text, not OpenAI-shaped). `responses.go`
shows `/v1/responses` is implemented, so the gap was specifically the single
model retrieve. Confirmed by grepping the mux: only `/v1/chat/completions`,
`/v1/responses`, `/v1/models` were registered. Offline-verifiable, priority-B
OpenAI-compat parity.

## What
- **`handleModelByID`** (`/v1/models/` subtree handler): `GET /v1/models/{id}`
  returns the OpenAI model object `{id, object:"model", created:0,
  owned_by:"agezt"}` when `{id}` is routable; a `404` with an OpenAI-shaped
  `{error:{message,type:"invalid_request_error"}}` when it isn't (so an SDK tells
  "unknown model" apart from "endpoint missing"); `405 Allow: GET` on a non-GET;
  `401` unauthenticated (same `auth` wrapper as the rest of the surface). The id
  may itself contain a slash (provider-prefixed ids like `anthropic/claude-…`),
  so everything after the prefix is the id, URL-unescaped.
- **`modelRoutable(eng, id)`** keeps retrieve in lockstep with the list: the
  routable set is exactly the default model + `eng.ModelIDs()` that
  `handleModels` advertises, so a retrieve never disagrees with the list.
- The list route stays an **exact** match; registering both `/v1/models` and
  `/v1/models/` lets the exact list win for `/v1/models` and the subtree serve
  `/v1/models/{id}` with no redirect.

## Verification
- **`kernel/openaiapi/models_retrieve_test.go`** (6 tests, httptest, no daemon):
  known id (incl. a provider-prefixed slash id) → model object; unknown →
  404 OpenAI shape; empty id (`/v1/models/`) → 404; the list route still exact;
  non-GET → 405 + Allow; unauthenticated → 401.
- **Negative control:** removed the `mux.HandleFunc("/v1/models/", …)`
  registration → known-id retrieve falls to the default `404 page not found`
  (plain text), failing both the status and JSON-parse assertions; restored the
  file byte-identical (`git diff` shows only the M378 addition).
- **Live daemon demo** (mock provider, fresh port): `GET /v1/models` →
  `{"id":"mock",…}`; `GET /v1/models/mock` → `200` `{"id":"mock","object":
  "model","owned_by":"agezt"}` (agrees with the list); `GET /v1/models/gpt-nope`
  → `404 {"error":{…,"type":"invalid_request_error"}}`; `POST` → `405 GET
  required`; no Authorization → `401 invalid_api_key`.
- `gofmt -l` clean; `go vet` clean; `GOOS=linux go build ./...` exit 0;
  `go.mod`/`go.sum` unchanged. Full suite **2146** passing (was 2140; +6),
  `go test ./...` 0 failures. CHANGELOG (Added, user-visible).

## Scope notes
- SPEC-16 §1.1 also lists `POST /v1/embeddings`. Deliberately NOT added here: a
  meaningful embeddings endpoint needs a real embedding-capable provider wired
  through the Governor (an external wire this offline pass can't verify). Faking
  it would invent a backend — recorded for a future provider-backed milestone,
  not closed.
- SPEC-16 §1.2 native-REST routes (intents/agents/journal/why/memory/skills/…)
  are served by the control plane (TCP/JSON-RPC) per DECISIONS B0 and surfaced by
  `agt`; the HTTP `kernel/restapi` is a deliberately thin read surface
  (`/healthz`, `/readyz`, `/metrics`, `/api/v1/{health,models,runs}`) — not a gap.
- Audited SPECs after this: 01-10, 13, 14, 15, 16 (§1 API, §2 test strategy
  largely = the existing property/contract suite, §3 config = env-based per
  B0c/stdlib-first, §4 standing-order DSL deferred with Chronos, §5 onboarding =
  `agt quickstart`). Remaining: 11, 12.
