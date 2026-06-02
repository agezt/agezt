# M159 — `agt run --dry-run`: resolve the run plan without executing

## Why
M148–M158 built a per-run override family (`--model`, `--system`, `--timeout`,
`--tools`/`--no-tools`). Each composes with the daemon defaults and a couple of
gates (the M91 vision gate, the M158 tool filter), and the *resolved* result —
which model actually runs, which tools survive the filter, what the effective
timeout is — was only observable by running and paying for it. An operator
stacking several overrides (`--model X --tools a,b --timeout 30s`) had no way to
confirm the resolution before committing tokens, nor to catch a model that isn't
in the catalog or a `--tools` name that isn't a registered tool.

`--dry-run` closes that loop: it resolves the full plan and returns it, executing
nothing and spending nothing.

## What
A dry-run reuses the existing `CmdRun` path (no new protocol command) with a
`dry_run` arg. `handleRun` parses every override exactly as a real run would —
so the plan reflects `--model`/`--system`/`--timeout`/`--tools` and passes the
same vision gate — then, instead of subscribing and starting the run, assembles a
plan and returns it.

### kernel/controlplane/dryrun.go (new)
- `runPlanInput` — the already-resolved primitives (effective model, override
  flags, catalog caps, system/timeout state, full toolset, requested allow-list),
  so the builder stays pure and table-testable (no kernel/catalog handles).
- `buildRunPlan(in) map[string]any` — resolves `model_source`
  (per-run vs daemon default), `system_source` (per-run / daemon default / none),
  `timeout` (per-run / daemon default / none), and the effective tool set: absent
  allow-list = the full toolset; a present allow-list intersects with the
  registered names, sorting the survivors into `tools` and any unknown requested
  name into `tools_dropped` (it would surface as `tool "X" is not available` at run
  time — the dry-run flags it up front). Capability keys (`supports_vision`,
  `supports_tools`) are included only when the model is known to the catalog.

### kernel/controlplane/server.go (`handleRun`)
- After every override is parsed (and after the vision gate), a `dry_run` arg
  short-circuits: build `runPlanInput` from the request + kernel accessors
  (`k.Model()`, `k.System()`, `k.MaxDuration()`, `k.Catalog().FindModel`,
  `k.Tools()`), call `buildRunPlan`, return it as a single `RespResult`. No
  correlation ID, no subscription, no `RunWith`.

### kernel/runtime/runtime.go
- New `MaxDuration() time.Duration` accessor so the control plane can report the
  daemon-wide timeout without reaching into `k.cfg`.

### cmd/agt/main.go
- `--dry-run` flag on `run`; sets `runArgs["dry_run"]=true`.
- `runDryRunMode` — a single non-streaming `c.Call`; renders a compact human
  summary (intent/tenant/model+caps/system/timeout/tools, plus a dropped-tools
  line) or, with `--json`, the raw plan object. `toStringSlice` helper coerces the
  decoded JSON arrays.
- Usage text extended with the `--tools`/`--no-tools`/`--dry-run` line.

## Tests (+7, all passing)
`kernel/controlplane/dryrun_test.go`:
- `TestBuildRunPlan_ToolsModes` (5 subtests) — unrestricted=all, `--no-tools`=none,
  subset, unknown-name-dropped, dedup; asserts `tools_mode`, sorted `tools`, and
  `tools_dropped`.
- `TestBuildRunPlan_Sources` — default sources (daemon/none), all-overrides
  (per-run model/system/timeout, caps surfaced for a known model), and the
  daemon-default-timeout / daemon-default-system path; also asserts capability keys
  are *omitted* (not just false) for an unknown model.

## Live proof (offline mock, ONE daemon — dry-runs never touch the provider)
- `--dry-run "..."` → `tools: all (7): browser.read, delegate, file, http, memory,
  shell, world`.
- `--dry-run --no-tools` → `tools: none (--no-tools)`.
- `--dry-run --tools shell,ghost` → `tools: restricted (1): shell` +
  `tools dropped: ghost (requested but not registered)`.
- `--dry-run --model foo-model --timeout 30s` → `model: foo-model (per-run
  (--model)) [not in catalog]`, `timeout: 30s (per-run)`.
- `--dry-run --json --tools file` → raw plan `{... "tools":["file"],
  "tools_mode":"restricted" ...}`.
- After **five** dry-runs, a real `agt run` still executed normally — proving the
  dry-runs consumed none of the mock's finite scripted responses (no provider call,
  no token spend).

## Verification
- `go.mod` / `go.sum` unchanged; no new protocol command or env var (M127 drift
  guard unaffected).
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./... -count=1` — **FAIL 0**, **1505 tests** (was 1498; +7), 61 packages.

## Result
The per-run override family is now inspectable: an operator can resolve exactly
what a run would do — model, capabilities, system source, timeout, and the precise
post-filter tool set — and catch an unknown model or an unregistered tool, all
before a single token is spent.
