# M198 — Bound HTTP request bodies on the network-exposed API surfaces

## Why
Two network-facing HTTP surfaces decoded their request body with no size limit:

- `kernel/restapi` — `POST /api/v1/runs` (`handleRunsRoot`)
- `kernel/openaiapi` — `POST /v1/chat/completions` (`handleChat`) and
  `POST /v1/responses` (`handleResponses`)

Each did `json.NewDecoder(r.Body).Decode(&req)` directly. `json.Decoder` reads
from the body as it parses; with a large enough value (e.g. one giant string)
the decode pulls the whole body into memory. An authenticated client could
therefore stream an unbounded request and drive unbounded memory growth — the
HTTP analogue of the framed-read flooding already fixed for the plugin host
(M177), the MCP bridge (M185), and the control plane (M188).

Both surfaces sit **behind authentication** (the routes are wrapped in
`s.auth(...)`; restapi already uses `subtle.ConstantTimeCompare` with an
empty-token-fails-closed check). So this is **defense-in-depth against an
authenticated DoS**, not a pre-auth hole — severity MEDIUM, consistent with how
the earlier framed-read caps were scoped.

## What
- `kernel/restapi/restapi.go` — added `errors` import and
  `const maxRequestBodyBytes = 16 << 20` (16 MiB). `handleRunsRoot` now wraps the
  body in `http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)` before decoding,
  and on a `*http.MaxBytesError` (detected via `errors.As`) returns
  `413 Request Entity Too Large` (`request_too_large`) instead of a generic 400.
- `kernel/openaiapi/openaiapi.go` — added `errors` import, the same
  `maxRequestBodyBytes` const, and a shared `decodeBody(w, r, v) bool` helper that
  applies the cap, maps an over-limit body to `413` (`invalid_request_error`), maps
  other decode errors to `400`, and returns `false` so the caller stops. `handleChat`
  now decodes via `decodeBody`.
- `kernel/openaiapi/responses.go` — `handleResponses` decodes via the same
  `decodeBody`, so the Responses surface inherits the identical cap and 413 path.

16 MiB comfortably exceeds any legitimate intent/prompt/instructions payload while
capping a single request's body allocation. The limit is intentionally a plain
constant (not env-tunable): it is a safety backstop, not an operational knob.

## Tests
- `kernel/restapi/request_oversize_test.go`
  - `TestRunsRoot_OversizedBody` — an authenticated `POST /api/v1/runs` whose body
    exceeds `maxRequestBodyBytes` returns `413`.
  - `TestRunsRoot_NormalBodyStillWorks` — an ordinary small request still returns 200
    (no regression).
- `kernel/openaiapi/request_oversize_test.go`
  - `TestChat_OversizedBody` — an authenticated `POST /v1/chat/completions` with an
    over-limit body returns `413`. Because the cap lives in the shared `decodeBody`
    helper used by both chat and responses, this exercises the common guard for both
    OpenAI-surface endpoints.
  - `TestChat_NormalBodyStillWorks` — a normal small chat request still returns 200.

All four use the existing in-process httptest harnesses (`do` / `newServer`,
`newAPIServer` + `Handler()`), driving the real auth + routing + decode path.

## Verification
- `go test ./...` — 1617 passing (1613 + 4 new), 0 failing.
- `go vet ./kernel/restapi/ ./kernel/openaiapi/` — clean.
- `gofmt -l` (CRLF-normalized) clean on all touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only: `errors`, `net/http`).
- Local commit only (no push); standard trailer.

## Files
- `kernel/restapi/restapi.go` — body cap + 413 on `handleRunsRoot`.
- `kernel/openaiapi/openaiapi.go` — `maxRequestBodyBytes`, `decodeBody` helper, `handleChat`.
- `kernel/openaiapi/responses.go` — `handleResponses` via `decodeBody`.
- `kernel/restapi/request_oversize_test.go` — new.
- `kernel/openaiapi/request_oversize_test.go` — new.

## Scope note
This bounds the JSON request *body*. Streaming responses (SSE) are unchanged. The
non-HTTP framed surfaces (plugin host, mcpbridge, control plane) were already
bounded in M177/M185/M188; this completes the same guarantee for the HTTP front
doors.
