# M517 — Mutation testing planner: fix FormatUSD dropping the sign on sub-dollar negatives

## Context
Twenty-seventh package in the mutation pass: `kernel/planner` (NL-intent → DAG generation,
plan refinement, cost estimation). Run with `GOMAXPROCS=3` (CPU-capped). go-mutesting
score 0.731 — the highest assessed; the DAG validation (cycle / duplicate-id / unknown-dep
/ self-dep / unknown-kind / empty-intent rejections) is thoroughly tested. Tree restored
clean after the run.

## The genuine defect (FIXED — not just a test gap)
`FormatUSD(microcents int64)` renders integer microcents as a dollar string. Its negative
handling was **incorrect**:

```
whole := microcents / 1_000_000_000
frac  := microcents % 1_000_000_000
if frac < 0 { frac = -frac }      // abs the fraction, but the sign is now lost
dec := frac / 100_000
return fmt.Sprintf("$%d.%04d", whole, dec)
```

For a sub-dollar negative (`|amount| < $1`), integer division gives `whole == 0`, so the
sign lives *only* in `frac`. Abs-ing `frac` without recording the sign drops the leading
`-`: `FormatUSD(-500_000_000)` returned **`"$0.5000"`** — a negative half-dollar printed as
positive. (`-1_234_500_000` was fine because its sign survived in `whole == -1`.)

`go-mutesting` flagged the abs guard as a surviving mutant; investigating *why* it was
unkillable revealed the guard itself was buggy — `TestFormatUSD` only ever passed
non-negative values, so the sign-loss had no coverage. Per the standing goal ("ne yanlış"
— what's wrong), the buggy behaviour was fixed rather than pinned.

Reachability: all current callers (`agt plan cost` / `plan dryrun`) pass non-negative cost
sums, so the bug is latent today — but `FormatUSD` is exported and its own `frac < 0` guard
documents clear intent to support negatives, so the contract was made correct.

## Fix
Compute the sign once up front, abs the whole value, and format with a sign prefix:

```
sign := ""
if microcents < 0 { sign = "-"; microcents = -microcents }
whole := microcents / 1_000_000_000
dec := (microcents % 1_000_000_000) / 100_000
return fmt.Sprintf("$%s%d.%04d", sign, whole, dec)
```

`FormatUSD(-500_000_000)` → `"$-0.5000"`, `-1_234_500_000` → `"$-1.2345"`. All five
existing (non-negative) cases are byte-identical.

## Test + negative control
Added negative cases to `TestFormatUSD` (`-500_000_000 → "$-0.5000"`, `-1_234_500_000 →
"$-1.2345"`, `-100_000 → "$-0.0001"`). Restoring the OLD buggy body makes the test FAIL
(`$0.5000` for the sub-dollar case) — proving it catches the regression; the fix restores
green.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on both staged LF blobs;
  `go build ./...` clean (production code changed).
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — twenty-seven packages (M490–M517)
…plugin, webhook, channel, anomaly, restapi, acp, state, planner — plus the controlplane
primary-token auth gate verified solid. planner is the first package in this arc where a
surviving mutant exposed a real production defect (a money-formatting sign bug) rather than
a pure test gap or an equivalent mutant.
