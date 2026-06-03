# M211 — Operator-tunable mesh hop limit

## Why
M209 bounds cross-node delegation at a fixed `MaxHops = 8`. That default is right for
almost everyone, but a fixed constant is wrong for two real cases: a deployment with a
legitimately deep, designed delegation topology (a chain of specialized nodes) needs a
higher ceiling, and a security-sensitive deployment may want a tighter one (e.g. allow
at most 2 hops). An operator should be able to set the bound without recompiling — the
same way budget, rate-limit, pricing-strict, and model-strict are all env-tunable.

## What
- **`kernel/meshctx`**:
  - `EnvMaxHops = "AGEZT_MESH_MAX_HOPS"` — the override env var (hardcoded literal to keep
    this leaf package dependency-free; matches the `AGEZT_` prefix).
  - `MaxHopsFromEnv() int` — returns the override when it parses to an integer in
    `[1, maxConfigurableHops]` (cap = 64), otherwise the `MaxHops` (8) default. Unset,
    zero, negative, over-cap, and unparseable all fall back to the default, so a typo
    like `100000` or `abc` can't silently defeat the guard.
- **`kernel/restapi` (`handleRunsRoot`)** — computes `maxHops := meshctx.MaxHopsFromEnv()`
  once and uses it for both the refusal comparison and the `mesh.loop_refused` event's
  `max_hops` payload (so the audit reflects the effective limit).
- **`plugins/tools/peer` (`remote_run`)** — the local pre-check uses `MaxHopsFromEnv()`
  too, so a node configured with a higher limit doesn't prematurely refuse its own
  delegations. The receiving node remains authoritative (each node enforces its own
  inbound limit).
- **`kernel/controlplane/config.go`** — `AGEZT_MESH_MAX_HOPS` added to `configEnvVars`
  so `agt config show` reports it (and the config-inventory invariant holds).

The default behaviour is unchanged: with the env unset, the effective limit is 8,
exactly as M209.

## Tests (+11)
- `kernel/meshctx/meshctx_test.go` — `TestMaxHopsFromEnv` table: unset → default; valid
  override (3, 1, whitespace-trimmed `" 5 "`); zero / negative / over-cap / garbage all
  fall back to default; at-cap (64) honored. (9 sub-cases.)
- `kernel/restapi/mesh_hop_test.go` — `TestMeshHop_EnvOverrideTightens`: with
  `AGEZT_MESH_MAX_HOPS=2`, a hop of 3 (under the default 8 but over the configured 2) is
  refused with `508`, while a hop of 2 (at the limit) still runs.

The M209/M210 tests (default-limit refusal, at-limit run, no-header run, audit event)
remain and pass unchanged.

## Verification
- `go test ./...` — 1674 passing (1663 + 11 new sub-tests/tests), 0 failing.
- `go vet ./kernel/meshctx/ ./kernel/restapi/ ./plugins/tools/peer/ ./kernel/controlplane/` — clean.
- `gofmt -l` (CRLF-normalized) clean on all touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only: `os`, `strconv`, `strings`).
- Local commit only (no push); standard trailer.

## Files
- `kernel/meshctx/meshctx.go` (+ test) — `EnvMaxHops`, `MaxHopsFromEnv`, cap.
- `kernel/restapi/restapi.go` — use `MaxHopsFromEnv()` for compare + audit payload.
- `plugins/tools/peer/peer.go` — local guard uses `MaxHopsFromEnv()`.
- `kernel/controlplane/config.go` — register the env var.
- `kernel/restapi/mesh_hop_test.go` — env-override test.

## Mesh thread (M8) so far
- **M200** bounded peer health read · **M201** `agt peers models` · **M202**
  `remote_run {model}` · **M203** auto-route · **M204** `agt peers route` · **M205**
  discovery cache · **M206** failover · **M207** `agt doctor` mesh check · **M208**
  `agt status` mesh config · **M209** loop guard · **M210** loop-refusal audit · **M211**
  tunable hop limit (this milestone).

The loop guard is now enforced (M209), observable (M210), and configurable (M211). A
refused-loop count surfaced in `agt status`/`doctor`, and load/cost-aware routing, remain
natural follow-ons left deferred to keep this milestone single-purpose.
