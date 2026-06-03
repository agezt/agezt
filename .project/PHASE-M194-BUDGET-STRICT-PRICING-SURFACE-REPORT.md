# M194 — Operator surface for strict pricing (env + `agt budget`)

## Why
M193 added the `StrictPricing` governor gate (refuse unpriced models rather than charge
$0), but only as a programmatic `Config` field — there was no way for an operator to
turn it on, and no way to see whether their daemon's spend is protected from the
unpriced-model bypass. This milestone makes the feature usable and visible.

## What
- **Operator config** — `AGEZT_PRICING_STRICT=on` wires `Config.StrictPricing` in the
  daemon (`cmd/agezt`), mirroring the existing `AGEZT_MODEL_STRICT` pattern. Off by
  default. Registered in `controlplane.configEnvVars` so `agt config show` reports it
  (an inventory test enforces that every env var the daemon reads is listed).
- **Snapshot + control plane** — `BudgetSnapshot.StrictPricing` carries the posture out
  of the governor; `handleBudget` adds `strict_pricing` to the `CmdBudget` response.
- **`agt budget` render** — a spend-protection line next to the spend total:
  - on:  `pricing  strict: models with no known price are refused`
  - off: `pricing  lax: unpriced models are charged $0 (set AGEZT_PRICING_STRICT=on to refuse)`
  `agt budget --json` carries `strict_pricing` for CI/jq.

## Tests
- `kernel/governor/snapshot_strict_test.go` — `Snapshot().StrictPricing` reflects the
  `Config.StrictPricing` value (both true and false).
- `kernel/controlplane` config-inventory test now passes with `AGEZT_PRICING_STRICT`
  registered.

### Live proof
Ran the offline daemon (`AGEZT_PROVIDER=mock`) and verified end-to-end:
- default: `agt budget` shows `pricing  lax: unpriced models are charged $0 (set AGEZT_PRICING_STRICT=on to refuse)`.
- `AGEZT_PRICING_STRICT=on`: `agt budget` shows `pricing  strict: models with no known price are refused`, and `agt budget --json` carries `"strict_pricing": true` — confirming the env → Config → Snapshot → control plane → CLI path.

## Verification
- `go test ./...` — 1607 passing, 0 failing.
- `go vet` clean on touched packages.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `cmd/agezt/main.go` — `AGEZT_PRICING_STRICT` → `Config.StrictPricing`.
- `kernel/governor/governor.go` — `BudgetSnapshot.StrictPricing` + `Snapshot()` wiring.
- `kernel/controlplane/budget.go` — `strict_pricing` in the budget response.
- `kernel/controlplane/config.go` — env var registered in `configEnvVars`.
- `cmd/agt/budget.go` — spend-protection render line.
- `kernel/governor/snapshot_strict_test.go` — new.
