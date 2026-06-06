# M511 — Mutation testing channel: pin SplitText's empty-buffer cut guard

## Context
Twenty-second package in the mutation pass: `kernel/channel` (the inbound/outbound
messaging normalization — fail-closed Allowlist, panic-recovery Guard, per-sender
conversation-history isolation, message SplitText). Run with `GOMAXPROCS=3` (CPU-capped).
Score 0.664, 79 survived; working tree restored clean after the run.

## Triage
Most survivors are **equivalent** mutants and were honestly left unpinned, not papered
over with brittle tests:
- `history.go` — the privacy/isolation logic (`sender != "" && p.Sender != sender`,
  the correlation-paired outbound gate, conversation-id/kind/empty-text filters, the
  `len(turns) <= 1` no-prior-context guard) is already killed by the existing
  isolation/limit/disabled tests. Its residual survivors are equivalent (e.g.
  `len(turns) > limit → >= limit`, where the trim at equality is a no-op).
- `split.go` — `breakAfter > 0 → >= 0` (breakAfter is only ever -1 or ≥1, never 0) and
  `breakAfter < len(cur) → <= len(cur)` (collapses to the same cut value when
  breakAfter == len(cur)) are equivalent.

## The genuine gap (closed)
`SplitText`'s cut trigger: `if units+ru > limit && len(cur) > 0`. The `len(cur) > 0`
guard prevents cutting against an **empty** buffer. The only way the buffer is empty at
the cut check is when a single character is wider than the limit (e.g. a 2-unit emoji at
limit 1) — then the char cannot be made to fit and is emitted as its own over-limit
piece. The existing emoji test uses limit 4 (where each char fits), so no test exercised
a sub-character limit, and `> 0 → >= 0` survived. Empirically confirmed non-equivalent:
`SplitText("😀😀", 1)` returns 2 pieces / 0 empty under the original, but 3 pieces /
**1 empty** under the mutant — a blank chunk that would make a channel send an empty
message (rejected by some platforms).

## Fix
Added `TestSplitText_NeverEmptyPiece` to `split_test.go`: across sub-character limits
(emoji/CJK at limit 1) no returned piece is empty and the rejoin stays lossless.

## Negative control (manual, CPU-capped)
- `len(cur) > 0 → len(cur) >= 0`: FAIL (a leading "" piece appears for `"😀😀"`).
Restored byte-for-byte (`git diff --ignore-all-space` on split.go empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — twenty-two packages (M490–M511)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus,
cadence, runtime, tenant, worldmodel, approval, memory, skill, standing, catalog, plugin,
webhook, channel — plus the controlplane primary-token auth gate verified solid. The
channel package's security core (Allowlist, sender isolation) was already solid; the one
non-equivalent gap was a robustness edge (never emit an empty chunk).
