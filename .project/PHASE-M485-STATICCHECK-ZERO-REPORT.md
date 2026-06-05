# M485 — staticcheck ./... driven to zero (17 → 0)

## Context
The defect-hunt arc (M456–M484) used manual review + targeted pattern scans. To
convert "no more defects found under my methodology" into an *objective,
enforceable* terminal condition, I ran the offline static-analysis toolchain that
is installed on the machine (staticcheck, govulncheck, errcheck, gosec,
golangci-lint, shadow, gitleaks). This milestone closes the `staticcheck` gate.

## Findings (17, all confirmed real)
`staticcheck ./...` reported, by check:

| check  | n  | meaning |
|--------|----|---------|
| S1005  | 13 | unnecessary `_` on a map read (`x, _ := m[k]`); a map index yields one value |
| SA4006 | 1  | a value assigned then overwritten before any read (dead write) |
| U1000  | 1  | an unused struct field |
| S1021  | 1  | a `var x T` immediately followed by `x = …` that should merge |
| S1016  | 1  | an identical-shape struct literal that should be a type conversion |

Non-test sites were in `kernel/controlplane/{edict,server,state}.go` (S1005) and
`plugins/sdk/sdk.go` (S1016). The rest were in tests.

## Fixes
- **S1005 ×13** — `capRaw, _ := req.Args["capability"]` → `capRaw := req.Args["capability"]`
  and the same for every control-plane arg read (edict/server/state) plus the
  `halt_resume_test.go` payload reads. `req.Args` is a `map[string]any`; the index
  expression already returns a single value, so the comma-ok form discarded nothing.
- **S1016** — `sdk.go` `return invokeResult{Output: out.Output, IsError: out.IsError}`
  → `return invokeResult(out)`. `Result` and `invokeResult` have identical fields
  (`Output string`, `IsError bool`), differing only in struct tags, which Go permits
  for a direct conversion. Behaviour-identical.
- **SA4006** — `budget_check_test.go` "all uncapped" case assigned `h, unl = …` but
  only read `unl` before `h` was overwritten by the next case. Changed to
  `_, unl = …` and noted that headroom is meaningless when unlimited. The test still
  asserts exactly what it meant.
- **U1000** — removed the never-read `gotIter int` field from the `streamProv` test
  mock in `agent_test.go`.
- **S1021** — `netguard_test.go` `var srv *httptest.Server; srv = httptest.NewServer(…)`
  → `srv := httptest.NewServer(…)`. The handler closure does not reference `srv`, so
  the two-step form was unnecessary (it is required only when the closure captures
  `srv`, which this one does not).

## Test + negative control
These are semantics-preserving cleanups, so the "test" is the existing full suite
(green) plus the gate itself. Negative control on the representative S1016 fix:
re-introducing the struct literal in `sdk.go` made `staticcheck ./plugins/sdk/...`
report `S1016` again; restoring the conversion cleared it (exit 0). The before/after
of the whole gate (17 → 0) is itself the sensitivity check.

## Verification / gate
- `staticcheck ./...` exit 0 (was 17 findings).
- gofmt-clean on staged LF blobs (all 8 files), `go vet ./...` exit 0,
  `GOOS=linux GOARCH=amd64 go build ./...` exit 0.
- Full `go test ./...` exit 0.
- `go.mod` / `go.sum` unchanged.

## Provenance / arc
First milestone of the OBJECTIVE-GATE arc that follows the DEFECT-HUNT arc
(M456–M484). The remaining gates: gitleaks (M486 — clean baseline for the 16
test-fixture hits), and govulncheck (2 stdlib advisories fixed in go1.26.4,
documented as a toolchain-bump remediation since go1.26.4 is not fetchable offline).
shadow was reviewed: all ~40 hits are idiomatic `if err := f(); err != nil` blocks
with no error-masking — nothing to fix.
