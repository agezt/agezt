# Agezt — Justified Dependencies

> **Policy:** stdlib-first, justified-minimal (POLICY §1). Every external
> dependency on the **core kernel** must justify itself here. Plugins may use
> heavy deps freely (POLICY §1.2) without entries here.

## Core Go module inventory

This table mirrors the current resolved module graph from `go list -m all` after `go mod tidy`. `go mod tidy` currently makes no module-file changes. `go mod why -m` shows the active direct roots are `plugins/channels/nostr` (`btcec`, `websocket`) and `plugins/channels/email` (`go-imap`); several other entries are graph-only/transitive but remain allowlisted because `tools/depscheck` intentionally checks every module returned by `go list -m all`.

| Module path | Version | Used by | Why stdlib is insufficient / status |
|---|---:|---|---|
| `github.com/btcsuite/btcd/btcec/v2` | v2.5.0 | `plugins/channels/nostr` | Secp256k1/BTC-compatible elliptic-curve primitives are not provided by the Go standard library; Nostr identity/signature support needs them. |
| `github.com/coder/websocket` | v1.8.15 | `plugins/channels/nostr` | WebSocket protocol support is not in the Go standard library; Nostr relay connectivity needs it. |
| `github.com/emersion/go-imap/v2` | v2.0.0-beta.8 | `plugins/channels/email` | IMAP protocol support is not in the Go standard library; used for inbound email-channel functionality. |
| `lukechampine.com/blake3` | v1.4.1 | `kernel/event`, `kernel/journal` | DECISIONS B3 freezes BLAKE3 as the hash function for the event chain and content-addressing. The standard library has no BLAKE3 implementation. This is the canonical pure-Go implementation (MIT, well-maintained, no CGO). POLICY §1.3 pre-blesses a BLAKE3 implementation as an acceptable core dep. |
| `github.com/btcsuite/btcd/chainhash/v2` | v2.0.0 | Indirect via current manifest deps | Transitive dependency; retained only while required by direct deps. |
| `github.com/decred/dcrd/crypto/blake256` | v1.1.0 | Indirect via current manifest deps | Transitive dependency; retained only while required by direct deps. |
| `github.com/decred/dcrd/dcrec/secp256k1/v4` | v4.4.1 | Indirect via current manifest deps | Transitive dependency; retained only while required by direct deps. |
| `github.com/emersion/go-message` | v0.18.2 | Indirect via `go-imap` | Transitive dependency for mail/message parsing. |
| `github.com/emersion/go-sasl` | v0.0.0-20241020182733-b788ff22d5a6 | Indirect via `go-imap` | Transitive dependency for SASL authentication mechanisms used by IMAP flows. |
| `github.com/klauspost/cpuid/v2` | v2.0.9 | Indirect via `blake3` | Pulled in by `lukechampine.com/blake3` for runtime SIMD-feature detection on amd64. Pure-Go, MIT. Not used directly by Agezt; removing it would mean forking `blake3`. |
| `github.com/davecgh/go-spew` | v1.1.1 | Indirect test dependency via `testify` | Transitive assertion/debug formatting dependency. Retained only while `testify` remains in the resolved module graph. |
| `github.com/pmezard/go-difflib` | v1.0.0 | Indirect test dependency via `testify` | Transitive diff dependency for assertion output. Retained only while `testify` remains in the resolved module graph. |
| `github.com/stretchr/testify` | v1.8.0 | Current resolved module graph | Test assertion helper. Verify whether active tests still import it before release; remove with `go mod tidy` if unused. |
| `github.com/yuin/goldmark` | v1.4.13 | Indirect via Go tooling modules | Markdown parser pulled by tooling/test-support module graph. Retained only while required by resolved modules. |
| `golang.org/x/crypto` | v0.0.0-20210921155107-089bfa567519 | Indirect via current deps/tooling | Extended crypto primitives outside the Go standard library. Retained only while required by resolved modules. |
| `golang.org/x/mod` | v0.8.0 | Indirect via Go tooling modules | Module-file parsing/version support for Go tooling. Retained only while required by resolved modules. |
| `golang.org/x/net` | v0.6.0 | Indirect via current deps/tooling | Extended network protocol support outside the Go standard library. Retained only while required by resolved modules. |
| `golang.org/x/sync` | v0.1.0 | Indirect via Go tooling modules | Extended concurrency primitives. Retained only while required by resolved modules. |
| `golang.org/x/sys` | v0.5.0 | Indirect via current deps/tooling | OS syscall/platform support outside the Go standard library. Retained only while required by resolved modules. |
| `golang.org/x/term` | v0.5.0 | Indirect via current deps/tooling | Terminal support outside the Go standard library. Retained only while required by resolved modules. |
| `golang.org/x/text` | v0.14.0 | Indirect via current deps/tooling | Unicode/text processing support outside the Go standard library. Retained only while required by resolved modules. |
| `golang.org/x/tools` | v0.6.0 | Indirect via Go tooling modules | Go analysis/tooling support. Retained only while required by resolved modules. |
| `golang.org/x/xerrors` | v0.0.0-20190717185122-a985d3407aa7 | Indirect via Go tooling modules | Legacy extended error support used by older Go tooling modules. Retained only while required by resolved modules. |
| `gopkg.in/yaml.v3` | v3.0.1 | Indirect test/tooling dependency | YAML parsing support outside the Go standard library. Retained only while required by resolved modules. |

## Tooling

*None.* The JSON Schema → Go codegen tool (`tools/jsonschemagen`) uses only
the standard library.

## CI / build

*None (Go).* GitHub Actions provides the runtime; no extra Go modules are pulled.

The Web UI (`frontend/`, decision A4) is a React + Vite app whose **npm**
dependencies are a build-time concern only: Vite compiles them into the static
bundle committed at `kernel/webui/dist/`, which the daemon `go:embed`s. None of
them are Go modules — `go.mod`/`go.sum` and `tools/depscheck/allowlist.txt` are
untouched, and `go:embed` is stdlib — so the core "justified-minimal Go deps"
posture is unaffected. npm deps live under `frontend/package.json` and are out of
scope for this table (they ship no runtime Go code; Node is never run at runtime).

---

## Adding a dependency

A dependency MAY be added only when **all five** POLICY §1.1 conditions hold:

1. The standard library is genuinely insufficient.
2. Writing it ourselves would cost significant time or introduce more bugs.
3. It is pure-Go (no CGO in the core; CGO only behind an optional build tag).
4. It is reasonably maintained and has an MIT-compatible license.
5. It is justified in one line in the table above **AND** added to the
   allowlist in `tools/depscheck/allowlist.txt` (CI fails otherwise).

## Anti-list (do not add to core)

These belong in plugins, never the core (POLICY §1.2):

- Browser automation (chromedp, playwright-go) → `plugin-tool-browser`
- Vector math / ANN libraries → `plugin-memory-flintvector` (or own embedded HNSW in `kernel/memory` at M2)
- Container runtimes / Docker clients → `plugin-warden-container` / deploy tooling
- Audio/video codecs → ambient plugins
- LLM SDKs (anthropic, openai, ollama clients) → respective `plugin-provider-*` (each provider plugin owns its dialect translation, SPEC-15)

## Notes on superseded text

- POLICY §1.3 lists "gRPC + protobuf" as illustrative accepted deps. This is
  **superseded by DECISIONS B0**: transport is stdio + JSON-RPC 2.0; the
  contract is JSON Schema (`.project/agezt-contract.jsonc`). Neither gRPC nor
  protobuf will appear in this file.
