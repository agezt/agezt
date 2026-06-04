# M389 — Content-addressed artifact store (SPEC-04 §3.6, foundation slice)

## Context
SPEC-04 §3.6 ("Artifacts"): tool outputs that are files become content-addressed
(BLAKE3) artifacts, referenced in events (`RawRef`), retrievable via Storage;
large outputs are never inlined (SPEC-01 §10.2 threshold). This is a LARGE,
multi-component feature — a storage subsystem + a loop-side threshold offload +
the `RawRef` on `tool.result` + a retrieval surface. Per the goal's rule on
large work, this milestone takes the first **self-contained sub-slice**: the
content-addressed blob store itself. The remaining slices (loop offload, RawRef,
retrieval) build on it and are recorded as the next steps.

## What
- **`kernel/artifact`** (new) — a pure, dependency-light content-addressed blob
  store rooted at a directory. No kernel/bus coupling.
  - `Ref(data) string` — lowercase hex BLAKE3-256 (64 chars); the content
    address. `blake3` is already a project dependency (journal/skill), so
    `go.mod`/`go.sum` are unchanged.
  - `Put(data) (ref, err)` — idempotent + deduplicating (identical bytes written
    at most once); atomic write (temp + rename) so a crash never leaves a partial
    blob at the final path.
  - `Get(ref) ([]byte, err)` — re-verifies the bytes hash to the ref (the address
    IS the integrity proof) → `ErrCorrupt` on tamper/bit-rot, `ErrNotFound` if
    absent.
  - `Has` / `Size`. On disk `<dir>/<aa>/<ref>`, sharded by the ref's first byte.
  - `validRef` enforces 64-char lowercase hex BEFORE any path is built, so a
    caller-supplied ref can never traverse out of the store directory.

## Verification
- **`kernel/artifact/artifact_test.go`** (7 tests): Put/Get round-trip + Size;
  dedup (same bytes → same ref, one file on disk; different bytes → different
  ref); Has present/absent; corrupt-blob → `ErrCorrupt`; absent → `ErrNotFound`;
  malformed/hostile refs (`""`, `../../etc/passwd`, non-hex, over-long) → all
  `ErrBadRef`, never touching the filesystem; empty blob is a valid artifact.
- **Negative control:** (1) forcing `validRef` to always return true → the bad-ref
  test fails (and even crashes on `ref[:2]`, proving the guard is load-bearing);
  (2) disabling the integrity compare in `Get` → the tamper test fails
  (`err = nil, want ErrCorrupt`). Restored byte-identical.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2181** passing (was 2174; +7). No CHANGELOG — this is an internal foundation
  with no user-facing surface yet.

## Next slices (recorded for the SPEC-04 §3.6 multi-milestone effort)
1. Daemon wiring: open an artifact store under `~/.agezt/artifacts` and expose it
   to the agent loop.
2. Loop threshold-offload: when a `tool.result` output exceeds a size threshold
   (SPEC-01 §10.2), Put it and put a `raw_ref` (+ a small preview) on the event
   instead of the full bytes — bounding event size while preserving the output.
3. Retrieval: `agt artifact get <ref>` (and the web UI tool-call card linking the
   ref) so an offloaded output is fetchable.
