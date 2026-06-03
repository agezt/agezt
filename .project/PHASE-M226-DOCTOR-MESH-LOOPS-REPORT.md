# M226 — Surface refused mesh loops in `agt doctor`

## Why
The mesh loop guard (M209–M213) is a security control: when a peer hands this
node a cross-node run whose hop count already exceeds the limit, the REST API
refuses it with `508 Loop Detected` and journals a `mesh.loop_refused` event
(M210), breaking a federation cycle. But the aggregate was invisible — the event
sat in the journal, surfaced nowhere. An operator had no at-a-glance signal that
their node is repeatedly stopping loops (a misconfigured topology, or someone
probing the REST API with loop-y hop headers). `next.md` carried this exact
candidate: "refused-loop count in status/doctor."

## What
A new `checkMeshLoops` doctor check. Because `agt journal stats` already folds
the journal into a per-kind count (`by_kind`, M132), the data already exists —
no new kernel counter is needed. The check:

- calls `CmdJournalStats`, reads `by_kind["mesh.loop_refused"]`;
- **WARNs** when the count is non-zero, naming it and hinting at a
  federation-topology cycle (the local node is fine — it correctly stopped the
  loop — but a peer delegating back into it is the thing to fix);
- returns **no line at all** when the count is zero, so healthy and single-node
  doctor output is unchanged (same conditional-surfacing pattern as
  `checkMeshAuth` / `checkMeshHopLimit`).

The pure decision is split into `meshLoopCheck(byKind map[string]any)` so it is
unit-testable without a control-plane round-trip; `checkMeshLoops` is the thin
client wrapper that fetches the stats and delegates.

## Files
- `cmd/agt/doctor.go` — `checkMeshLoops` + `meshLoopCheck` + conditional
  registration + the `kernel/event` import (edited).
- `cmd/agt/doctor_meshloops_test.go` — 3 tests (new): none-refused stays quiet
  (nil map / kind absent / explicit zero), a non-zero count WARNs with the count
  and a hint, and the int/int64/float64 count forms all surface (JSON numbers
  arrive as float64 over the wire).

## Verification
- `go test ./cmd/agt/` — green; full suite **1739 → 1742** (+3), 66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./cmd/agt/` clean.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.
- **Live proof (gold standard, end-to-end against a running daemon):** started a
  daemon with `AGEZT_REST_ADDR` set; `POST /api/v1/runs` with
  `X-Agezt-Mesh-Hop: 9` (> the limit of 8) returned **HTTP 508**, journaling a
  `mesh.loop_refused`; `agt doctor` then showed
  `[WARN] mesh-loops : 1 mesh delegation loop(s) refused (incoming hop limit
  exceeded)`. The signal fired on a single-node daemon — exactly the case it is
  meant to catch. (The minted REST token was handled locally and never printed.)

## Scope notes
- The count is cumulative over the journal's retention (the journal is
  append-only, full-retention). A "since last boot" or windowed variant would
  need a timestamp filter — a later refinement if operators want recency.
- `agt status` was left unchanged; doctor is the right home for a
  conditional-on-problem security signal. A status line could follow if desired.
