# M521 — Mutation testing meshctx: pin MaxHopsConfig's raw + validOverride returns

## Context
Thirty-first package in the mutation pass: `kernel/meshctx` (the cross-node delegation
hop-count carrier and the operator-tunable hop-limit config). Run with `GOMAXPROCS=3`
(CPU-capped). go-mutesting score 0.667, 16 survivors; working tree restored clean.

## Triage
The `WithHop` negative clamp and `Hop` default-zero are pinned by their tests. The
*effective* hop limit is thoroughly bounded by `TestMaxHopsFromEnv` — it covers unset,
valid, the min edge `1`, `0`, negative, the cap `maxConfigurableHops`, over-cap, garbage,
and whitespace. So `MaxHopsConfig`'s integer-range guard (`n >= 1 && n <= 64`) is already
solid.

## The genuine gap (closed)
`MaxHopsConfig` returns three values — `(effective, raw, validOverride)` — but every test
went through `MaxHopsFromEnv`, which **discards `raw` and `validOverride`**. So those two
returns were entirely unpinned; mutation testing left all three `validOverride` results
alive (confirmed by hand-applied negative control against the existing suite):
- `return MaxHops, raw, false → true` (the over-cap / zero / garbage fall-back path).
- `return MaxHops, "", true → false` (unset).
- `return n, raw, true → false` (valid override).

`validOverride` is the signal `agt doctor` uses to flag a typo'd override (e.g.
`AGEZT_MESH_MAX_HOPS=100`, above the cap) that silently fell back to the default. If the
flag were stuck `true`, the operator would believe a rejected setting had taken effect —
the loop guard still works (default 8), but the diagnostic that would catch the
misconfiguration is defeated.

## Fix
Added `TestMaxHopsConfig_RawAndValidity`: asserts all three returns across unset / valid /
at-cap / over-cap / zero / garbage / whitespace — pinning that `raw` is the trimmed input
and `validOverride` is true only for an in-range integer (or unset).

## Negative control (manual, CPU-capped)
The three `validOverride` mutants each FAIL under the new test. Restored byte-for-byte
(`git diff --ignore-all-space` on meshctx.go empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — thirty-one packages (M490–M521)
…ulid, artifact, reflect, meshctx — plus the controlplane primary-token auth gate verified
solid. The gap class here: a multi-return function whose secondary returns (a diagnostic
flag) are dropped by the only caller the tests exercise.
