# Phase M572 — Official TypeScript SDK for the REST API

**Type:** Feature (SDK; completes the TS half of decision A4)
**Date:** 2026-06-07
**Branch:** `feat-typescript-sdk`

## Goal

Ship a first-class TypeScript/JavaScript client for Agezt's REST API (`/api/v1`),
mirroring the Python SDK (M571) so JS/TS apps (Node and browser) can drive the
daemon. Keep the runtime dependency-free (platform `fetch`); the only dev
dependency is TypeScript, and tests use Node's built-in test runner.

## What shipped

### `sdk/typescript/` (new)
- `package.json` — `@agezt/sdk` v1.0.0, ESM, `engines.node >=18`, **zero runtime
  dependencies**; devDeps: `typescript` + `@types/node`. `npm test` = `tsc`
  (type-check + build to `dist/`) then `node --test`.
- `src/client.ts` — `class Client(baseUrl, token, { timeoutMs=30000, tenant? })`:
  - `health()` / `models()` / `getRun(id)` (typed JSON GETs)
  - `run(intent, model?)` → `Promise<RunResult>` (blocking POST)
  - `runStream(intent, model?)` → `AsyncGenerator<StreamEvent>` parsed from the
    SSE `ReadableStream` (handles `\n\n` and `\r\n\r\n` frame separators,
    multi-line `data:`, `:` heartbeats)
  - bearer auth; optional `tenant` → `X-Agezt-Tenant`; per-request `AbortController`
    timeout; non-2xx → `APIError(status, type, detail)` (parses both the
    `{error:{type,message}}` and failed-run `{status,error}` shapes).
- `src/errors.ts` (`AgeztError`/`APIError`), `src/index.ts` (public exports),
  `tsconfig.json` (NodeNext, strict, declarations), `README.md`.

### `sdk/typescript/test/client.test.ts` (new — 7 tests, all pass)
`node:test` + `node:assert/strict` against a `node:http` mock: health, models,
sync run (+ model forwarding), failed run → `APIError(502)`, SSE streaming
(start/token×2 with a heartbeat/done + token reassembly), getRun, bad-token →
`APIError(401)`.

### Build / CI
- `.github/workflows/ci.yml`: a `typescript-sdk` job (`actions/setup-node` 22,
  npm cache, `npm ci` + `npm test`).
- `.gitignore`: `sdk/typescript/node_modules/` + `dist/` (the npm package is built
  in CI / on publish); `package-lock.json` IS committed for reproducible `npm ci`.

## Verification

- **Unit:** `npm test` in `sdk/typescript` — `tsc` clean, 7/7 node:test pass.
- **Full Go gate** unaffected: `GOMAXPROCS=3 go test ./... -p 2` exit 0 (77
  packages); no Go change; `go.mod` unchanged.
- **Runtime smoke (executable proof, criterion-7):** booted the real daemon with
  `AGEZT_REST_ADDR`, read the minted token, drove it with the compiled SDK:
  `health()` v1.0.0, `models()` mock, `run("ping")` → `completed`/`[echo]\nping`,
  `runStream("hello")` parsed the SSE (`start`/`done`), `getRun()` → 6-event arc,
  wrong token → `APIError(401)`; 0 panics.

## Counts

- Go packages 77, Go tests 2440 (unchanged — TS SDK is separate). TS SDK: 7
  node:test cases.
- No Go dependency added; the TS SDK has zero runtime dependencies.

## Out of scope (documented follow-ups)

- Publishing to npm (packaging ready; release is an ops step).
- A browser usage example / bundler guidance (the client is already
  browser-compatible via `fetch`).
