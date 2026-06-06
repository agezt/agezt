# M535 — Mutation testing the file tool: pin the single-line read-range edge

## Context
**New front:** the `plugins/` tree (agent-invoked tools, channel adapters, provider
adapters) had been *fuzzed* (16 targets) but never *mutation-tested*. This corrects the
earlier "offline surface exhausted" assessment, which had only covered `kernel/`. First
target: `plugins/tools/file` (the workspace file tool — read/write/search/glob/replace
with path containment). Run with `GOMAXPROCS=3`. go-mutesting score 0.467, 162 survivors;
tree restored clean.

## Triage — path containment is solid (security core)
The security-critical part — `withinRoot` / `resolve` (no `..` escape, no absolute outside
root, no symlink escape, M252/M253) — is verified by negative control: removing the
`HasPrefix(rel, "..")` escape check, flipping the `rel == "."` root case, and negating the
`!withinRoot` check in `resolve` are ALL killed by the containment test suite
(`TestContainment_*`). The traversal boundary is genuinely pinned.

## The genuine gap (closed)
`doReadRange` rejects an inverted range with `if end < start { error }`. The range is
inclusive `[start, end]`, so a **single-line** read (`start == end`, e.g. "read lines 5-5")
is valid. `TestRead_LineRange` covers a multi-line range `[2,4]` and a start-only window;
`TestRead_LineRange_Errors` covers `end < start` (error) and past-EOF. But no test sits on
`start == end`, so `end < start → <=` survived — under it a single-line read is wrongly
rejected as "end_line is before start_line". (The `line < start` / `line > end` inclusive
selection edges and the `start < 1` clamp are already killed / equivalent respectively.)

## Fix
Extended `TestRead_LineRange` with a `[3,3]` single-line read: returns exactly `L3` under
the `[lines 3-3]` header, excluding neighbours.

## Negative control (manual, CPU-capped)
`end < start → <=`: FAIL (single-line read rejected). Restored byte-for-byte
(`git diff --ignore-all-space` on file.go empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Note — scope correction
The `plugins/` tree is substantial (providers, channels, tools, mcpbridge) and now in
scope for mutation testing. The file tool's path-containment security core is solid; this
closes a usability edge. Further plugin-tree targets (shell exec, http SSRF, channel HMAC
surrounding logic, mcpbridge protocol) remain — a genuine continuation, not padding.
