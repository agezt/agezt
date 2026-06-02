# M129 — `--tenant` on the observability CLIs

## Why
M128 fixed the **daemon** half of tenant self-observability: it granted a tenant
token authorization for the read-only commands that fold its own journal (memory /
world / approvals / plan / provider / schedule / warden log+stats). But the **agt
CLI** half was missing — those subcommands had no `--tenant` flag, so there was no
client path to actually exercise the grant:

```
$ agt memory log --tenant acme
agt memory log: unexpected arg "--tenant"
```

This completes the feature: a single coherent capability — observe a tenant's own
isolated subsystems — needs both the daemon grant (M128) and the CLI flag (M129).

## What
Added `--tenant <id>` to the 13 observability subcommands, using the same shared
helpers (`extractTenantFlag` + `withTenant`) already used by `agt runs`, `edict`,
`tool`, `webhook`, etc.:

- `memory log`, `world log`
- `approvals log`, `approvals stats`
- `plan history`, `plan stats`
- `provider log`, `provider stats`, `provider rejections`
- `schedule fires`, `schedule stats`
- `warden log`, `warden stats`

Each change is the same two lines (`tenant, args := extractTenantFlag(args)` at the
top; `withTenant(tenant, callArgs)` at the call) plus a `[--tenant <id>]` mention in
the usage string. An empty tenant still routes to the primary kernel, so existing
no-flag behavior is unchanged.

## Files
- `cmd/agt/memory_log.go`, `world_log.go`, `approvals_log.go`, `provider_log.go`,
  `warden.go`, `plan_history.go`, `schedule.go` — `--tenant` wiring + help text.
- `cmd/agt/tenant_flag_test.go` (new) — `TestObservabilityCmds_AcceptTenantFlag`
  (all 13 accept `--tenant`, never an "unexpected arg" / exit-2) and
  `TestObservabilityCmds_TenantFlagInHelp` (all 13 document it). Daemon-free: dial
  fails fast against an empty `AGEZT_HOME`, so the test asserts flag acceptance, not
  the round-trip.

## Live proof (multitenant daemon, offline mock)
```
OPERATOR (primary token):
  agt memory log     --tenant acme  → "no memory operations journaled yet."
  agt provider stats --tenant acme  → "provider routing (over 2 routed call(s)):"
  agt warden stats   --tenant acme  → "no sandboxed executions."

TENANT (own token):
  agt provider stats --tenant acme  → "provider routing (over 2 routed call(s)):"
  agt schedule stats --tenant acme  → "no scheduled firings."

BOUNDARY:
  <acme token> agt provider stats --tenant other → "controlplane: unauthorized"
```
Operator inspects any tenant; a tenant reads its own; cross-tenant access is
refused — the full M128+M129 capability works end-to-end from the CLI.

## Verification
- 55 packages `ok`, **FAIL 0**; **1419 tests**.
- `go vet` clean, `GOOS=linux go build ./...` ok, `go.mod` / `go.sum` unchanged.
- gofmt: all 7 touched CLI files and the new test clean under LF (HEAD-vs-work
  parity 0/0).
