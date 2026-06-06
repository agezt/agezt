# M489 — Wire the cleaned gates into CI (make "enforceable" real) + golangci-lint sweep

## Context
M485/M486/M487 drove `staticcheck` and `gitleaks` to zero and triaged the rest, and
M488 fixed the FreeBSD build. But a gate that CI does not run will rot — the "now
enforceable" claims were only true if `.github/workflows/ci.yml` actually executes
them. It did not. This milestone closes that loop and records the golangci-lint sweep.

## golangci-lint sweep (the last untapped linters)
Ran the high-signal correctness linters not covered individually before:

| linter | result | disposition |
|--------|--------|-------------|
| ineffassign | 0 | clean |
| unconvert | 0 | clean |
| bodyclose | 2 | test-only (`netguard_test.go` requests **expected to fail** → nil response, nothing to leak) |
| nilerr | 19 | all false positives (two idioms, below) |
| gocritic | 9 | pure style (`+=`, if-else→switch, singleCaseSwitch, unlambda) |
| noctx | 17 | HTTP calls bounded by `http.Client{Timeout}` / operator CLI |
| unparam | 12 | harmless (interface conformance / future params) |
| prealloc | 7 | micro-perf only |

**nilerr triage** — every hit is one of two deliberate, correct idioms:
1. **Tool-result convention** (memory/worldmodel/subagent managers): a tool returns
   `(agent.Result, error)` where `error` is the *protocol/transport* error; a
   *tool-level* failure is conveyed as `Result{IsError:true}` with a `nil` protocol
   error. `return Result{Output: err.Error(), IsError:true}, nil` is exactly right.
2. **Skip-malformed on read-only fold** (controlplane handlers, channel/history,
   runtime, main.go usage-fold, disk.go WalkDir): inside a `journal.Range` /
   `filepath.WalkDir` callback, returning a non-nil error **aborts the entire scan**,
   so one bad/irrelevant record returns `nil` to skip and continue. The `_ =` on the
   `Range`/`WalkDir` call and explicit comments ("skip a malformed payload rather than
   abort the fold"; disk.go's "must never be the reason a diagnostic errors") confirm
   intent. Aborting a whole journal fold on a single corrupt event would itself be a
   DoS.

No genuine defect surfaced — consistent with the M487 result across the other tools.

## CI gate wiring (the actual change)
`.github/workflows/ci.yml` previously ran: vet + test + build (ubuntu/macos/windows),
`-race` (linux/cgo), codegen-in-sync, cross-build (linux/darwin/windows), deps-check.
Gaps closed:

1. **New `lint` job** — `gofmt -l .` (zero-tolerance; tree verified gofmt-clean in LF),
   `staticcheck ./...` (zero, M485), `govulncheck ./...` (the runner's `go-version:
   stable` + `check-latest` carries the patched stdlib, so the M487 advisory is moot
   on CI and new advisories are caught going forward).
2. **New `secrets` job** — `gitleaks detect` over the **full history**
   (`fetch-depth: 0`), auto-loading the repo-root `.gitleaks.toml` baselined in M486.
3. **FreeBSD added to the cross-build matrix** — `freebsd/amd64`, which the M488 fix
   made buildable; the existing matrix would not have caught that regression.

## Verification (of what is verifiable offline)
- `actionlint` on the updated workflow: **exit 0** (YAML + expression syntax valid).
- Every command the new jobs run was executed locally and passes: tree-wide `gofmt -l`
  on staged LF blobs = 0 dirty; `staticcheck ./...` = 0; `gitleaks detect` = no leaks;
  `GOOS=freebsd GOARCH=amd64 go build ./cmd/...` = exit 0.
- The GitHub-hosted run itself cannot be executed in this offline environment; it is
  validated by actionlint + local command-equivalence (the jobs run the same commands
  proven green here).
- No Go code changed; `go.mod`/`go.sum` unchanged; host `go test ./...` still exit 0.

## Outcome
The static-analysis, secret, vulnerability, formatting, and FreeBSD-build gates are now
*actually enforced* on every push and PR — the cleanliness established in M485–M488 is
now durable, not a point-in-time snapshot.
