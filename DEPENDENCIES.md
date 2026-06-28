# Agezt — Justified Dependencies

> **Policy:** stdlib-first, justified-minimal (POLICY §1). Every external
> dependency on the **core kernel** must justify itself here. Plugins may use
> heavy deps freely (POLICY §1.2) without entries here.

## Core Go module inventory

This table mirrors the current build list reported by `go list -m all` against `go.mod`/`go.sum` after `go mod tidy`. `tools/depscheck` enforces that every module in the build list is present in `tools/depscheck/allowlist.txt`, so this table and the allowlist must stay in lock-step.

Entries are grouped by role:

* **Direct** — listed in the top-level `require ()` of `go.mod`, with at least
  one source import.
* **Indirect** — listed in the `require ()` block with `// indirect`, pulled
  in transitively by a direct dep, with at least one transitive import path.
* **Graph-only** — in the build list reported by `go list -m all`, but no
  source file in the repo imports them. These appear because Go's module
  resolution (`MVS`) walks every `// indirect` reference transitively, and
  earlier code paths left stale entries that `go mod tidy` does not auto-
  strip. They are documented for `depscheck` completeness; removing them
  is tracked as a follow-up (UPD-002).

### Direct deps (4)

| Module path | Version | Used by | Why stdlib is insufficient / status |
|---|---:|---|---|
| `github.com/btcsuite/btcd/btcec/v2` | v2.5.0 | `plugins/channels/nostr` | Secp256k1/BTC-compatible elliptic-curve primitives are not in the Go standard library; Nostr identity / signature support needs them. Pure-Go, MIT. |
| `github.com/coder/websocket` | v1.8.15 | `plugins/channels/nostr` | WebSocket protocol support is not in the Go standard library; Nostr relay connectivity needs it. |
| `golang.org/x/net` | v0.6.0 | `plugins/tools/browser` | Public Suffix List for eTLD+1 cookie partitioning; the browser tool's cookie jar uses it to prevent cross-site cookie leaks (VULN psl-fix). Already a transitive dep via `go-message` → `x/text` → `x/tools`; promoted to direct for `publicsuffix.List`. |
| `github.com/emersion/go-imap/v2` | v2.0.0-beta.8 | `plugins/channels/email` | IMAP protocol support is not in the Go standard library; used for inbound email-channel functionality. |
| `lukechampine.com/blake3` | v1.4.1 | `kernel/event`, `kernel/journal`, `kernel/artifact` | DECISIONS B3 freezes BLAKE3 as the hash function for the event chain and content addressing. The standard library has no BLAKE3 implementation. This is the canonical pure-Go implementation (MIT, well-maintained, no CGO). POLICY §1.3 pre-blesses a BLAKE3 implementation as an acceptable core dep. |

### Indirect deps (6, all reachable from a Direct dep above)

`go mod why -m` confirms each of these is reached by a Direct dep at
build time. Versions are the Minimum Version Selection (MVS) result.

| Module path | Version | Pulled in by | Why stdlib is insufficient / status |
|---|---:|---|---|
| `github.com/btcsuite/btcd/chainhash/v2` | v2.0.0 | `btcec/v2/schnorr` (via Nostr) | Hash type used by the BTC signing primitives Nostr depends on. Not in stdlib. |
| `github.com/decred/dcrd/crypto/blake256` | v1.1.0 | `btcec/v2/schnorr`, `dcrec/secp256k1/v4/schnorr` | BLAKE-256 used inside the secp256k1 signing path. Not in stdlib. |
| `github.com/decred/dcrd/dcrec/secp256k1/v4` | v4.4.1 | `btcec/v2` | Decred's secp256k1 implementation — what `btcec/v2` itself wraps for some operations. Not in stdlib. |
| `github.com/emersion/go-message` | v0.18.2 | `go-imap/v2/imapclient` | RFC 5322 mail/message parsing used by IMAP flows. Not in stdlib. |
| `github.com/emersion/go-sasl` | v0.0.0-20241020182733-b788ff22d5a6 | `go-imap/v2/imapclient` | SASL authentication mechanisms (PLAIN, OAUTHBEARER, …) used by IMAP. Not in stdlib. |
| `github.com/klauspost/cpuid/v2` | v2.0.9 | `blake3/guts` | Runtime SIMD-feature detection on amd64 used by blake3's hot path. Pure-Go, MIT. Not used directly by Agezt; removing it would mean forking `blake3`. |

### Transitive test deps of upstream packages (MVS-walked, not Agezt imports)

These modules appear in `go list -m all` because Go's MVS walks every
transitive reference, but **`go mod why -m all` reports `(main module does
not need)` for each of them** — no source file in Agezt imports them.
Investigation (UPD-002 closed): they are test-time-only dependencies of
Agezt's direct / indirect deps, specifically declared in their own
`go.mod` files. Examples from `go mod graph`:

* `btcec/v2@v2.5.0` declares `testify@v1.8.0`, `go-spew@v1.1.1`,
  `go-difflib@v1.0.0`, `yaml.v3@v3.0.1` as its own test deps.
* `go-message@v0.18.2` declares `x/text@v0.14.0`; `x/text@v0.14.0`
  transitively pulls `x/tools@v0.6.0`, which pulls `goldmark`, `x/mod`,
  `x/net`, `x/sys`, `x/sync`, and a chain of older toolchain versions
  (`x/crypto`, `x/xerrors`, etc.).

These entries are **metadata only** — they live in the MVS walk output
and in transient `/go.mod h1:` lines during `go mod tidy`, but **none
of them is compiled into the Agezt binary** (verified by `grep -a -F` on
the built `agezt.exe`: zero matches for `stretchr/testify`, `goldmark`,
`yaml.v3`, or any of the `golang.org/x/*` modules). They are kept in
`tools/depscheck/allowlist.txt` because that tool enforces
`go list -m all` membership; trimming the allowlist would break the CI
gate without changing what the binary contains.

`tools/depscheck` and `go mod tidy` together enforce the invariants the
rest of the build relies on; we tolerate this 14-entry transitives list
rather than fork upstream packages.

| Module path | Version | Source upstream (per `go mod graph`) |
|---|---:|---|
| `github.com/davecgh/go-spew` | v1.1.1 | `btcec/v2@v2.5.0` (test dep) |
| `github.com/pmezard/go-difflib` | v1.0.0 | `btcec/v2@v2.5.0` (test dep) |
| `github.com/stretchr/testify` | v1.8.0 | `btcec/v2@v2.5.0` (test dep) |
| `gopkg.in/yaml.v3` | v3.0.1 | `btcec/v2@v2.5.0` (test dep) |
| `github.com/yuin/goldmark` | v1.4.13 | `golang.org/x/tools@v0.6.0` (toolchain dep) |
| `golang.org/x/crypto` | v0.0.0-20210921155107-089bfa567519 | `golang.org/x/mod@v0.6.0-dev.0.20220419223038-86c51ed26bb4` (legacy toolchain) |
| `golang.org/x/mod` | v0.8.0 | `x/text@v0.14.0` and `x/tools@v0.6.0` (toolchain dep) |
| `golang.org/x/sync` | v0.1.0 | `x/tools@v0.6.0` (toolchain dep) |
| `golang.org/x/sys` | v0.5.0 | `x/text@v0.14.0`, `x/net@v0.6.0`, etc. (platform shim) |
| `golang.org/x/term` | v0.5.0 | `x/net@v0.6.0` (toolchain dep) |
| `golang.org/x/text` | v0.14.0 | `go-message@v0.18.2` (used by IMAP plugin) |
| `golang.org/x/tools` | v0.6.0 | `x/text@v0.14.0` (toolchain dep) |
| `golang.org/x/xerrors` | v0.0.0-20190717185122-a985d3407aa7 | legacy toolchain dep, no longer reachable from the active path |

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
- The 14 "Transitive test deps of upstream packages" entries are not stale
  build-list noise — they are legitimate test-time deps of `btcec/v2` (used
  by the Nostr plugin) and `go-message` (used by the email plugin). They are
  documented in the transitive test deps table above so the audit and the
  reviewer can verify the chain from `go mod graph` to the entry. We
  tolerate them rather than fork upstream packages to drop test deps that
  Agezt never compiles.
