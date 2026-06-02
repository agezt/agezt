# M155 — Governor concurrency hardening (code review)

## Why
Continuing the standing code-quality mandate, an independent review of the
**Governor** — the highest-stakes subsystem (per-task routing, USD-microcent spend
accounting, daily/per-task ceilings, provider fallback chain, rate limiting) — was
commissioned, focused on concurrency, money correctness, and the fallback walk. It
confirmed the cost-sensitive logic is sound, and found two real concurrency bugs.

## Fixes

### C1 — Data race on the routing slices (Critical)
`routeChain` (the per-`Complete` chain builder) and `Providers` (the banner
snapshot) read `g.primary` / `g.fallback` with **no lock held**, while `Replace` —
the credential-rotation / hot-reload path (M1.r) — rebuilds those same slices under
`g.mu`. Because `Complete` calls `routeChain` outside the mutex, a hot reload
concurrent with an in-flight call was an unsynchronized read/write of the slice
headers and backing array. `Replace` did `g.primary = g.primary[:0]` then re-`append`ed
into the *same* backing array, so a concurrent reader could copy a half-overwritten
array → a chain with `nil`/stale `*ProviderInfo` entries → mis-route or nil-deref
panic of the daemon. (`go test -race` flags it directly.)
**Fix:** `routeChain` and `Providers` now snapshot `primary`/`fallback` under
`g.mu` before using them, and `Replace` builds **fresh** slices and assigns them
(rather than truncating + reusing the live backing array) — so a reader that
snapshotted the old header keeps seeing a consistent old array.

### H2 — Bus pointer tear (High)
`SetBus` wrote `g.cfg.Bus`; `publish` read it lock-free on the hot path. The
contract ("call SetBus before the first Complete") is violated by the documented
`WithLimits`/`WithDailyCeiling` sibling pattern, which tells callers to `SetBus` on
a sibling that may already be serving — an unsynchronized pointer read/write.
**Fix:** the bus is now held in an `atomic.Pointer[bus.Bus]` (the same pattern
`pricing.go` already uses for the live catalog); `New` seeds it from `cfg.Bus`,
`SetBus` `Store`s, `publish` `Load`s. Race-free regardless of call ordering.

### H1 — Daily-ceiling TOCTOU (documented, not changed)
The ceiling pre-check and the post-call `recordUsage` are separate critical
sections with the provider call in between, so N concurrent calls can all observe
headroom and together overshoot the ceiling by up to (N-1) calls' worth. This is a
deliberate soft-cap design (bounded overshoot for a $20/day-class cap), but it was
undocumented. Added a comment at the pre-check stating it's a soft cap and how to
make it hard (reserve estimated cost under the check's lock) if ever required.

## Verified correct by the review (unchanged)
- Fallback classification (`shouldFallback`): `context.Canceled`,
  `DeadlineExceeded`, and anything wrapping `ErrBudgetExceeded` (incl.
  `ErrTaskBudgetExceeded`, which wraps it) are terminal — no budget-exhausted or
  cancelled error wrongly spends on another provider.
- Daily/per-task spend counters and the UTC-day rollover (`rolloverIfNeededLocked`)
  are accessed consistently under `g.mu`; rollover is always invoked locked.
- Per-tool-timeout vs. parent-cancel attribution in the agent loop is correct (the
  child deadline is captured before cancel; the parent ctx is checked first).
- The provider `Registry` is RWMutex-guarded on every accessor.

## Files
- `kernel/governor/governor.go` — `bus atomic.Pointer`; locked snapshots in
  `routeChain`/`Providers`; fresh-slice `Replace`; `SetBus`/`publish` via the
  atomic; H1 soft-cap comment.
- `kernel/governor/governor_test.go` — `TestGovernor_ConcurrentReplaceAndComplete`.

## Tests (+1, all passing)
- `TestGovernor_ConcurrentReplaceAndComplete` — runs `Replace` + `SetBus` + six
  `Complete`/`Providers` goroutines concurrently for 200ms; asserts no panic and
  that every `Complete` succeeds (the chain always holds both providers, so a torn
  snapshot yielding an empty/nil chain would surface). Passes 5× consecutively; run
  with `-race` to detect the races directly (CGO required, unavailable here — the
  test still guards the corruption/panic mode).

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on both touched files.
- `go test ./...` — **FAIL 0**, **1485 tests** (was 1484; +1), 61 packages.

## Result
The Governor's hot-reload path is now race-free against in-flight calls, and the
audit bus is latched atomically — closing a Critical data race that could panic the
daemon during credential rotation, with the cost/fallback logic confirmed correct
and the soft-cap semantics now documented.
