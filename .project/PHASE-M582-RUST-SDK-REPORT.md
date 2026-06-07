# PHASE M582 — Official Rust SDK (`sdk/rust`, crate `agezt`)

**Status:** DONE — local, gated, ready for branch/PR.
**Scope:** Completes decision **A4** ("SDKs in Go/TS/Python/Rust"). Go (plugin
SDK + `sdk/`), Python (sync + async, M571/M576), TypeScript (M572) already
shipped; this adds the **Rust** quarter. All four SDKs now exist.

## What shipped

A zero-runtime-dependency Rust client for the daemon's native REST API
(`/api/v1`), mirroring the Python/TS SDKs method-for-method:

| Method | REST | Returns |
|---|---|---|
| `health()` | `GET /api/v1/health` | `Health` |
| `models()` | `GET /api/v1/models` | `Models` |
| `run(intent, Option<model>)` | `POST /api/v1/runs` | `RunResult` |
| `run_stream(intent, Option<model>)` | `POST /api/v1/runs` (SSE) | `RunStream: Iterator<Item = Result<StreamEvent>>` |
| `get_run(id)` | `GET /api/v1/runs/{id}` | `RunArc` |

- `Client::new(base_url, token)` + builder `.with_timeout(Duration)` /
  `.with_tenant(id)` (→ `X-Agezt-Tenant`). Bearer auth.
- Error model: non-2xx → `Error::Api { status, kind, message }` (understands both
  `{"error":{"type","message"}}` and the failed-run `{"status","error"}` body);
  transport/IO → `Error::Transport`. Same mapping as Py/TS.
- `StreamEvent.data` and `RunArc.events` are the SDK's public `Value` (small JSON
  type) with ergonomic accessors (`str`, `get`, `as_i64`, `as_array`, …).

## Why std-only (zero deps)

The whole project is "stdlib-first, one dependency." The Python SDK is
`urllib`+`json`; the TS SDK is platform `fetch`. The Rust SDK matches that promise
rather than pulling `reqwest`+`serde`+`tokio`:

- **`src/json.rs`** — a recursive-descent JSON parser + serializer + `Value`
  (strings with `\uXXXX` + surrogate pairs, int/float numbers, nested
  arrays/objects; rejects trailing/truncated input).
- **`src/http.rs`** — a minimal HTTP/1.1 client over `std::net::TcpStream`:
  one request per connection (`Connection: close`), and a `BodyReader: Read` that
  decodes **Content-Length**, **chunked transfer-encoding** (for SSE), and
  read-to-EOF framings transparently.
- `#![forbid(unsafe_code)]`.

**Scope boundary (documented, honest):** plain `http://` only — std ships no TLS
and the SDK takes no deps. This matches the daemon's loopback deployment
(`AGEZT_REST_ADDR=127.0.0.1:8800`); reach `https` via a TLS-terminating proxy.
`Target::parse` rejects `https://` with a clear message rather than silently
failing.

## Tests (24 total, all green)

- **13 unit** (`src/*.rs` `#[cfg(test)]`): JSON scalars/escapes/unicode/nesting/
  round-trip/rejection, URL `Target` parsing (host/port/path, https + bad-port +
  empty-host rejection), status-line parsing, run-body building, error mapping
  (structured + failed-run + non-JSON), percent-encoding.
- **8 integration** (`tests/client.rs`): a mock daemon on a real
  `std::net::TcpListener` (no third-party HTTP/test deps), speaking
  Content-Length JSON and **chunked** SSE. Mirrors the TS `node:http` mock:
  - `health`, `models`, `get_run` (arc count/events)
  - `run` forwards a distinct model and returns the answer (proves forwarding —
    the mock echoes the parsed model; a non-default value can only come from the
    request)
  - failed run (`intent:"boom"`) → `Error::Api(502)` with the failed-run message
  - `run_stream` → `start/token/token/done`, tokens reassembled to `"hello"`
    across **separate HTTP chunks** + an interleaved `: heartbeat` comment
  - bad token → `Error::Api(401, "unauthorized")`
  - tenant header transmitted (mock reflects `X-Agezt-Tenant` into `version`)
- **3 doc-tests** (`cargo test` compiles the lib/client examples; the `Value`
  example runs).

## Gate

- `cargo build` clean; `cargo test` → 13 + 8 + 3 = **24 passed, 0 failed** (no
  compiler warnings).
- `cargo fmt --check` clean (verified on the final tree). NOTE: this Windows
  dev box has a flaky `cargo.exe`/`rustfmt.exe` shared-library load (intermittent
  exit 127 — an environment issue, never an exit-1 diff); a clean exit-0 check was
  obtained on the finalized sources. The CI `rust-sdk` job re-runs it on Linux.
- `clippy` not run locally (component non-functional on this toolchain); the
  compiler is warning-clean. Not added as a CI gate (mirrors Py/TS, which gate
  compile+test only).
- Go tree **untouched** — full Go suite still 80 pkgs green; `go.mod` unchanged.

## Wiring

- `sdk/rust/{Cargo.toml,Cargo.lock,README.md,src/*,tests/*}`. `Cargo.lock`
  committed (trivial — zero deps) for reproducible builds, mirroring the
  committed TS `package-lock.json`.
- `.gitignore`: `/sdk/rust/target/`.
- CI: new **`rust-sdk`** job (`dtolnay/rust-toolchain@stable` + rustfmt;
  `cargo fmt --check` + `cargo test --verbose`). 20th CI check.
- `CHANGELOG.md` Unreleased "Added" entry (M582).

## Follow-ups (not done — need a steer)

- Publish to crates.io (ops/secrets, like PyPI/npm publish).
- An async client (tokio/async-std) — would add deps; the blocking client is the
  zero-dep baseline. Defer unless asked.
