# M174 — Strict runtime[<digits>] validation for removable policy rules

## Why
A follow-up to the M173 Edict security review (MEDIUM-1). `IsRuntimeRule` is the
load-bearing invariant behind runtime policy management: `RemoveHardDeny` (and
`agt edict deny rm`) only removes a hard-deny rule when `IsRuntimeRule(name)` is
true, so the boot-time floor (built-ins + `AGEZT_EDICT_DENY` operator rules) can be
*tightened* at runtime but never *loosened*. It was implemented as a bare
`strings.HasPrefix(name, "runtime[")`, so any name starting with `runtime[` —
`runtime[`, `runtime[evil`, `runtime[]` — was classified as removable. The exact-name
match in `RemoveHardDeny`'s loop meant no *current* floor rule (named `rm-rf-root`,
`operator[N]`, …) was actually removable, so this was defense-in-depth rather than a
live exploit — but a security invariant this important should validate the full
shape, not just the opening bracket, so a future refactor or a forged durable event
can't smuggle a removable-looking name onto a floor rule.

## What
`IsRuntimeRule` now requires the exact canonical shape `AddHardDeny` mints —
`runtime[` + one-or-more ASCII digits + `]` — via `CutPrefix`/`CutSuffix` + a digit
check. Anything else returns false.

## Tests (+1)
`TestIsRuntimeRule_StrictShape` — `runtime[0]`/`runtime[7]`/`runtime[123]` are
runtime rules; `runtime[`, `runtime[evil`, `runtime[]`, `runtime[5x]`, `runtime[ 5]`,
`runtime[5] ` (trailing space), `operator[1]`, `rm-rf-root`, `mkfs`, and the empty
string are NOT; and `RemoveHardDeny("runtime[evil")` is refused. All legit runtime
rules (which are always `runtime[<N>]`) still pass, so runtime add/remove is
unaffected — verified by the existing `TestRemoveHardDeny_OnlyRuntimeRules` /
`TestAddHardDeny_AssignsRuntimeNameAndFires`.

## Verification
- `go.mod` / `go.sum` unchanged; no new protocol command, env var, or event kind.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok; `gofmt -l` clean on both files.
- `go test ./... -count=1` — FAIL 0, 1555 tests (was 1554; +1), 61 packages.

## Result
The "you can tighten but never loosen the floor" invariant is now enforced on the
full rule-name shape, not a spoofable prefix.
