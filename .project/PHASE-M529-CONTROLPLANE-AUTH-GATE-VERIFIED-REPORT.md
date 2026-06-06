# M529 — Mutation-verify the control-plane primary-token auth gate (rigorous)

## Context
`kernel/controlplane` is the daemon's privileged admin API — the most security-critical
surface. It is large (~10,420 LOC across ~40 files, 71 test files), so a whole-package
`go-mutesting` run is intractable. This milestone rigorously verifies its security core,
`tokenIsPrimary` (the constant-time primary/admin-token check, M187), by hand-applied
negative control — upgrading the prior informal "verified out-of-band" note to a
reproducible result. Run with `GOMAXPROCS=3` (CPU-capped).

## tokenIsPrimary — verified solid
```
func (s *Server) tokenIsPrimary(presented string) bool {
	want := s.Token()
	if want == "" || presented == "" { return false }   // defense-in-depth blank guard
	return subtle.ConstantTimeCompare([]byte(presented), []byte(want)) == 1
}
```

Every meaningful operator mutated and run against `auth_test.go`:
- `want == "" → !=` — **killed** (the exact-match test: a non-empty server token would be
  forced to `return false`, rejecting the valid token).
- `presented == "" → !=` — **killed** (same path).
- `ConstantTimeCompare(...) == 1 → != 1` — **killed** (exact token rejected).
- guard `|| → &&` — **survives, and is EQUIVALENT**: `ConstantTimeCompare` returns 0 on a
  length mismatch, so the only case the blank guard uniquely protects (both `want` and
  `presented` empty, where ConstantTimeCompare of two empty slices returns 1 and would
  wrongly authorize) is *still* caught by `&&` (`"" == "" && "" == ""` → true → false).
  Every other (empty, non-empty) pairing yields `false` either way. Unkillable by
  construction.

So the primary-token gate's correctness — only the exact token authorizes; blank/unset
never does; the comparison is constant-time — is fully pinned.

## Scope honesty
This verifies the **auth gate**, not the entire control plane. The ~40 command handlers
(runs, schedule, edict overlay, tenant, logs, …) are covered by 71 test files but have not
been exhaustively mutation-tested — that is intractable at this package's size and is noted
as a boundary, not a claim of completeness. The single most security-critical primitive is
now rigorously verified.

## Verification / gate
- No code change; `go test ./kernel/controlplane/` passes (`GOMAXPROCS=3`).
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — thirty-five packages + the control-plane auth gate (M490–M529)
The control-plane primary-token gate, previously noted as verified out-of-band, is now
verified by explicit negative control: three meaningful mutants killed, one equivalent.
