# Phase M571 — Official Python SDK for the REST API

**Type:** Feature (SDK; realizes the Python half of decision A4)
**Date:** 2026-06-07
**Branch:** `feat-python-sdk`

## Goal

Ship a first-class Python client for Agezt's native REST API (`/api/v1`), opening
the daemon to the Python ecosystem. Hold to the project's stdlib-first ethos: the
client and its tests use the Python standard library only — no third-party
dependencies, and CI runs the tests with no `pip install`.

## What shipped

### `sdk/python/` (new)
- `pyproject.toml` — package `agezt` v1.0.0, `requires-python >=3.9`, **zero
  runtime dependencies**.
- `agezt/client.py` — `Client(base_url, token, timeout=30, tenant=None)`:
  - `health()` → `GET /api/v1/health`
  - `models()` → `GET /api/v1/models`
  - `run(intent, model=None)` → `POST /api/v1/runs` (blocking) → `RunResult`
    (correlation_id, model, status, answer)
  - `run_stream(intent, model=None)` → `POST /api/v1/runs` with
    `Accept: text/event-stream`, an iterator of `StreamEvent`
    (`start`/`token`/`done`/`error`) parsed from SSE (handles multi-line `data:`
    and `:` heartbeats)
  - `get_run(correlation_id)` → `GET /api/v1/runs/{id}`
  - bearer-token auth; optional `tenant` → `X-Agezt-Tenant`; non-2xx →
    `APIError(status, type, message)` (parses both the `{"error":{type,message}}`
    shape and the failed-run `{"status":"failed","error":"…"}` shape).
- `agezt/errors.py` — `AgeztError` / `APIError`.
- `agezt/__init__.py`, `README.md`.

### `sdk/python/tests/test_client.py` (new — 7 tests, all pass)
`unittest` against a stdlib `http.server` mock: health, models, sync run (+ model
forwarding), failed run → `APIError(502)`, SSE streaming (start/token×2 with a
heartbeat/done, token reassembly), get_run, and bad-token → `APIError(401)`.

### CI
- `.github/workflows/ci.yml`: a `python-sdk` job (`actions/setup-python`,
  `python -m unittest discover -s tests`). No `pip install` — stdlib only.

## Verification

- **Unit:** `python -m unittest` in `sdk/python` — 7/7 pass.
- **Full Go gate** unaffected: `GOMAXPROCS=3 go test ./... -p 2` exit 0 (77
  packages); no Go files changed; `go.mod`/`go.sum` unchanged; the new
  `frontend-dist-in-sync` and existing jobs are untouched.
- **Runtime smoke (executable proof, criterion-7):** booted the real daemon with
  `AGEZT_REST_ADDR` set, read the minted bearer token from the banner, and drove
  it with the SDK: `health()` → v1.0.0; `models()` → mock; `run("ping")` →
  `completed` / `[echo]\nping`; `run_stream("hello")` parsed the SSE
  (`start`/`done`); `get_run()` → 6-event arc; wrong token → `APIError(401)`;
  0 panics.

## Counts

- Go packages 77, Go tests 2440 (unchanged — the SDK is Python, separate from the
  Go test count). Python SDK: 7 unittest cases.
- No Go dependency added; the Python SDK has zero runtime dependencies.

## Out of scope (documented follow-ups)

- TypeScript SDK (the other A4 SDK) — its own follow-up.
- An async client (`asyncio`/`aiohttp`) — the current client is synchronous +
  a streaming generator, dependency-free.
- Publishing to PyPI (packaging is ready; release is an ops step).
