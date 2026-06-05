# M437 — cgroup-aware GOMAXPROCS (SPEC-11 §4 resource limits)

## Context
Audit of the two not-yet-reviewed specs, **SPEC-11 (Deployment)** and **SPEC-12
(Widgets)**, against the codebase (both subordinate to DECISIONS.md — gRPC/
protobuf/all-out-of-process language superseded).

- **SPEC-12 (Widgets): no offline-actionable gaps — deliberate deferral.** The
  whole widget layer (descriptor `{widget,data}`, SDK widget submodule, sandboxed
  React render, first-party widgets, conversation-surface block list) is assigned
  to Phases 5/7/8 and marked APPEND-ONLY-not-yet-built in `agezt-contract.jsonc`.
  It rides on the React UI (SPEC-07) the project deliberately did not build (the
  shipped UI is the stdlib vanilla-JS operator dashboard). The lone observation —
  the `ui_widgets` contribution field is declared in the contract but unwired in
  the SDK — would be dead plumbing with no renderer to consume it (gold-plating),
  so it is correctly left as spec-ahead-of-code. **No change.**
- **SPEC-11 (Deployment): largely satisfied.** Health/readiness (`/healthz`,
  `/readyz`, `/metrics`), graceful drain + restart=recovery, loopback-default
  exposure (0.0.0.0-never-implicit), 0600 secret files, default-deny egress are
  all already implemented. Of the two candidate gaps the audit raised: the
  `config.yaml`/flag config layer is a **deliberate** env-only decision (recorded
  in the SPEC-16 audit — "config = env-based per B0c/stdlib-first, no YAML file"),
  so not a gap. That left one genuine offline-actionable gap.

## The gap
SPEC-11 §4: *"Resource limits: per-deployment CPU/mem caps … don't hot-loop a
Pi."* The Go runtime is **not cgroup-aware**: inside a CPU-quota cgroup (a
container started with `--cpus=N`, a constrained VPS) it still defaults
`GOMAXPROCS` to the number of **host** cores. So a 1-CPU deployment spins up
`NumCPU()` Ps and over-schedules against a fraction of a core — exactly the
"hot-loop a Pi" symptom. The native `GOMAXPROCS`/`GOMEMLIMIT` env vars (which the
runtime already honors) only help if the operator remembers to set them; nothing
adapts automatically. `uber-go/automaxprocs` solves this but is a dependency the
project's lean-deps policy forbids.

## The fix (`cmd/agezt/maxprocs.go`)
A stdlib-only equivalent. Two pure, injectable helpers:
- `cgroupCPUQuota(readFile)` reads the fractional CPU quota from cgroup **v2**
  (`/sys/fs/cgroup/cpu.max`, "quota period" / "max period") then **v1**
  (`cpu.cfs_quota_us` / `cpu.cfs_period_us`); returns false for no finite quota.
- `cgroupMaxProcs(readFile, numCPU, gomaxprocsEnv)` computes the target — `ceil`
  of the quota, clamped to `[1, numCPU]`, **0 = no change**. It defers entirely to
  an explicit `GOMAXPROCS` env, and returns 0 when the quota ≥ host cores (the
  default is already correct). Only ever lowers toward the quota.

`applyAutoMaxProcs()` wires `os.ReadFile` / `runtime.NumCPU()` /
`os.Getenv("GOMAXPROCS")`, is a no-op off Linux, calls `runtime.GOMAXPROCS(n)`,
and returns a one-line banner note. `runDaemon` calls it first and prints the
note (e.g. `GOMAXPROCS 8 → 2 (cgroup CPU quota ≈ 2 cores)`).

## Verification
- **`cmd/agezt/maxprocs_test.go`**: `TestCgroupMaxProcs` (11 sub-cases: v2/v1
  quotas, fractional→ceil, `max`=no-limit, negative/unset, explicit-env-wins,
  quota≥cores→no-change, over-host clamp, no-files); `TestCgroupCPUQuota_V2PrecedesV1`
  (hybrid host → v2 authoritative). All pure, no real cgroup needed.
  - **Negative controls:** (1) drop the `GOMAXPROCS`-env guard → "explicit wins"
    FAILs (returns 1, want 0); (2) drop the `>= numCPU` clamp guard → the
    quota-≥-host and over-host cases FAIL (return 8/16, want 0). Both restored.
- **Gate:** staged (LF) blobs gofmt-clean, `go vet` clean, `GOOS=linux go build
  ./...` ok, `go.mod`/`go.sum` unchanged. Full suite **2312** passing (was 2299;
  +13), `go test ./...` exit 0. CHANGELOG Reliability entry.

## Notes
- `GOMEMLIMIT` needs no code: the Go runtime honors the env var natively (Go
  1.19+); operators set it directly. Adding an `AGEZT_*` alias would be redundant.
- Auto-detection only **lowers** GOMAXPROCS and never overrides an explicit
  setting, so it is safe-by-construction on every host class.

## Review status
SPEC-11 and SPEC-12 are now audited (the last two unaudited specs). All 16 SPECs
audited. SPEC-11's offline-actionable surface is satisfied; remaining SPEC-11
items are Docker/CI/cloud/network infra not implementable or verifiable offline.
