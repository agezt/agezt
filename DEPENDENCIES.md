# Agezt — Justified Dependencies

> **Policy:** stdlib-first, justified-minimal (POLICY §1). Every external
> dependency on the **core kernel** must justify itself here. Plugins may use
> heavy deps freely (POLICY §1.2) without entries here.

## Core kernel

| Module path | Version | Used by | Why stdlib is insufficient |
|---|---|---|---|
| `lukechampine.com/blake3` | v1.4.1 | `kernel/event`, `kernel/journal` | DECISIONS B3 freezes BLAKE3 as the hash function for the event chain and content-addressing. The standard library has no BLAKE3 implementation. This is the canonical pure-Go implementation (MIT, well-maintained, no CGO). POLICY §1.3 pre-blesses a BLAKE3 implementation as an acceptable core dep. |
| `github.com/klauspost/cpuid/v2` | v2.0.9 | (transitive of `blake3`) | Pulled in by `lukechampine.com/blake3` for runtime SIMD-feature detection on amd64. Pure-Go, MIT. Not used directly by Agezt; removing it would mean forking `blake3`. |

## Tooling

*None.* The JSON Schema → Go codegen tool (`tools/jsonschemagen`) uses only
the standard library.

## CI / build

*None.* GitHub Actions provides the runtime; no extra modules are pulled.

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
