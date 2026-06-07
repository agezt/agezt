# Agezt TypeScript SDK

The official TypeScript/JavaScript client for the
[Agezt](https://github.com/agezt/agezt) agentic OS. It talks to a running Agezt
daemon's native REST API (`/api/v1`) — the same governed kernel loop `agt run`
uses (Edict policy, the journal, cost governance) — over `fetch` with a bearer
token.

**Zero runtime dependencies** (uses the platform `fetch` + `ReadableStream`);
works in Node 18+ and modern browsers.

## Install

```bash
npm install @agezt/sdk
```

## Quick start

Start a daemon with the REST API enabled and note the token it prints:

```bash
AGEZT_REST_ADDR=127.0.0.1:8800 agezt
#   rest api : http://127.0.0.1:8800/api/v1  (Authorization: Bearer <token>)
```

```ts
import { Client } from "@agezt/sdk";

const c = new Client("http://127.0.0.1:8800", "<token>");

console.log((await c.health()).version);
console.log((await c.models()).default);

// Blocking run.
const result = await c.run("summarise the latest commits");
console.log(result.answer);

// Streaming run — tokens as the agent produces them.
for await (const ev of c.runStream("write a haiku about Go")) {
  if (ev.event === "token") process.stdout.write(String(ev.data.text ?? ""));
}

// The journaled event arc of a past run.
const arc = await c.getRun(result.correlation_id);
console.log(arc.count, "events");
```

## API

| Method | REST endpoint | Returns |
|---|---|---|
| `health()` | `GET /api/v1/health` | `Promise<Health>` |
| `models()` | `GET /api/v1/models` | `Promise<Models>` |
| `run(intent, model?)` | `POST /api/v1/runs` | `Promise<RunResult>` |
| `runStream(intent, model?)` | `POST /api/v1/runs` (SSE) | `AsyncGenerator<StreamEvent>` (`start`/`token`/`done`/`error`) |
| `getRun(correlationId)` | `GET /api/v1/runs/{id}` | `Promise<RunArc>` |

`new Client(baseUrl, token, { timeoutMs?, tenant? })` — pass `tenant` to target an
isolated tenant (sent as `X-Agezt-Tenant`) on a multi-tenant daemon.

Non-2xx responses throw `APIError` (`.status`, `.type`, `.detail`).

## Develop / test

The only dev dependency is TypeScript; tests use the Node built-in test runner
(`node:test`) against a `node:http` mock — no third-party test framework.

```bash
cd sdk/typescript
npm ci
npm test      # tsc (type-check + build) then node --test
```
