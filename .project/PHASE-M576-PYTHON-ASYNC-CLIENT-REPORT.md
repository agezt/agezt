# Phase M576 — Python SDK asyncio client (`AsyncClient`)

**Type:** Feature (SDK; completes sync+async parity for the Python client)
**Date:** 2026-06-07
**Branch:** `feat-python-async-client`

## Goal

Give the Python SDK (M571) an asyncio client so async Python code can drive a
running Agezt daemon without blocking the event loop — matching the TypeScript
SDK (M572), which is inherently async. Still standard-library only (no aiohttp),
preserving the SDK's zero-dependency story.

## What shipped

### `sdk/python/agezt/aio.py` (new) — `AsyncClient`
Mirrors `agezt.Client` method-for-method, every call awaitable:
- `await health()`, `await models()`, `await run(intent, model=None) → RunResult`,
  `await get_run(id)`, and `async for ev in run_stream(intent, model=None)`
  yielding `StreamEvent`s (`start`/`token`/`done`/`error`).
- `async with` support (`__aenter__`/`__aexit__`) + `aclose()` (no-op — the
  client holds no persistent connection; provided for symmetry).
- Same constructor `(base_url, token, timeout=30.0, tenant=None)`; `base_url`,
  `token`, `timeout`, `tenant` exposed as read-only properties.

**Implementation (stdlib-only, genuinely non-blocking):** the async layer reuses
the synchronous `Client`'s request building, error mapping (`APIError`), and SSE
parsing *verbatim* — it governs only *when* that work runs, not *how* the
protocol is spoken. Unary calls run in the default thread executor
(`loop.run_in_executor`). The streaming run drives the sync generator in a worker
thread and bridges each parsed event to the event loop through an
`asyncio.Queue` (`call_soon_threadsafe`), with a unique end-of-stream sentinel; a
mid-stream error is routed through the queue and re-raised in the consuming
coroutine. So `await`/`async for` never block the loop.

### Exports + docs
- `agezt/__init__.py`: `AsyncClient` added to the public API + `__all__`, with an
  asyncio usage example in the package docstring.
- `sdk/python/README.md`: new "Asyncio" section + API note.

### Tests — `sdk/python/tests/test_aio.py` (new; 7 tests)
`unittest.IsolatedAsyncioTestCase` against the SAME stdlib `http.server` mock as
the sync tests (imports `_Handler` from `test_client`, no duplication). Covers:
health+models, blocking run (+ model forwarding), failure → `APIError` (502),
streaming (`start`/token/token/`done`, reassembled answer, heartbeat ignored),
`get_run`, bad-token 401, and `async with` returning a usable client.

## Verification

- **SDK tests (as CI runs them):** `cd sdk/python && python -m unittest discover
  -s tests` → **14 tests OK** (7 sync + 7 async). No third-party deps, no `pip
  install`. (The error-path tests emit a benign `ResourceWarning` about
  `HTTPError` cleanup — pre-existing in the sync client's error path; a warning,
  not a failure; `unittest` returns OK.)
- **No Go code touched** → Go build/test tree unaffected; `go.mod`/`go.sum`
  unchanged. `__pycache__`/`*.pyc` stay gitignored (verified not staged).

## Counts

- Go packages/tests unchanged (80 / 2463 — this is a Python-only change).
- Python SDK tests: 7 → **14**.

## Out of scope (documented follow-ups)

- Publishing the SDK to PyPI (needs release secrets/ops).
- A pure-`asyncio`-sockets transport (current design wraps the proven sync
  transport in an executor — correct and zero-dep; a native socket client would
  remove the worker threads but re-implement HTTP/TLS/SSE by hand).
