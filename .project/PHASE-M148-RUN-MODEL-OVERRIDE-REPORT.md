# M148 — `agt run --model <id>` (per-run model override)

## Why
`agt run` always used the daemon's configured model (`AGEZT_MODEL`). To run one
intent against a different model — a bigger model for a hard task, a cheaper one
for a throwaway query — an operator had to restart the daemon with a different
`AGEZT_MODEL`, or drive it through the OpenAI-compatible API (which DOES route
per-request). The native CLI couldn't. Yet the whole mechanism already exists: the
agent loop reads a per-run model from the context (`modelFromCtx`), and the OpenAI
API sets it via `runtime.WithModel`. `agt run --model` just wires the CLI into that
same, tested path.

## What
- **Handler** (`kernel/controlplane/server.go`, `handleRun`) — reads an optional
  `model` arg. It computes the **effective model** (`effModel`) — the override or
  the kernel default — up front, and when an override is given sets
  `ctx = runtime.WithModel(ctx, override)` so the loop routes this run to it (the
  same context key the OpenAI API uses). Empty override = unchanged default path.
- **Correctness — vision gate uses the effective model**: the M91 vision capability
  gate previously checked `k.Model()` (the daemon default). With a per-run override
  that was wrong — an image attached to a vision-capable *override* model would be
  rejected because the *default* isn't vision-capable (or vice-versa). The gate now
  judges `effModel`, and the rejection event/message name it.
- **CLI** (`cmd/agt/main.go`, `cmdRun`) — `--model <id>` / `--model=<id>`, passed as
  the `model` run arg; documented in help.

## Files
- `kernel/controlplane/server.go` — `effModel`, per-run `WithModel`, vision gate on
  the effective model.
- `cmd/agt/main.go` — `--model` flag + help.
- `kernel/controlplane/controlplane_test.go` — `TestRun_ModelOverride`.

## Tests (+1, all passing)
- `TestRun_ModelOverride` — a run submitted with `model: "custom-model-xyz"` reaches
  the provider with `req.Model == "custom-model-xyz"` (captured via the mock's
  `OnRequest`), proving the override routes through the loop.

## Live proof (offline mock, real booted daemon)
```
$ agt run "hi"                              # default
  usage: mock · 2 iteration(s)

$ agt run --model my-custom-model "hello"   # override
  --- final answer ---
  [offline-mock] …                          # run SUCCEEDS (exit 0)
  usage: my-custom-model · 2 iteration(s)   # routed to the requested model
```

The run completes and the usage line (M146) reflects the overriding model — the
requested model is what the run routed to (the always-on mock fallback supplied the
actual completion offline, as designed).

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./...` — **FAIL 0**, **1473 tests** (was 1472; +1), 61 packages.

## Result
`agt run --model <id>` routes a single run to any model without touching the daemon
config — composing with the M146 cost line ("which model, how many iterations, what
$") for quick, per-run model/cost experiments — and the vision gate now correctly
judges the model the run will actually use.
