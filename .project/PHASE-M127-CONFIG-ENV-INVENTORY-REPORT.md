# M127 — Complete the `agt config show` env inventory (+ drift guard)

## Why
`agt config show` exists to answer "what is this daemon actually running with?" —
explicitly designed to be pasted into a bug report. Its `env` section reports the
PRESENCE (never the value) of each `AGEZT_*` var the daemon reads. The list,
`configEnvVars`, carries a documented invariant:

> Source-of-truth: every `Getenv("AGEZT_...")` in cmd/agezt/. New vars MUST be
> added here when introduced.

That invariant had silently rotted. An audit of actual env reads vs. the list:

- **~78** distinct `AGEZT_*` vars read by the daemon (cmd/agezt + kernel + plugins).
- **26** in `configEnvVars`.
- **55 missing** — including `AGEZT_WEBHOOKS`, `AGEZT_MULTITENANT`, `AGEZT_SCHEDULE`,
  `AGEZT_SUBAGENT*`, `AGEZT_PEERS`, `AGEZT_PULSE*`, `AGEZT_TELEGRAM_*`,
  `AGEZT_REDACT*`, `AGEZT_RUN_TIMEOUT`, `AGEZT_TOOL_TIMEOUT`, `AGEZT_REST_ADDR`,
  `AGEZT_WEB_ADDR`, `AGEZT_API_ADDR`, the `AGEZT_DEMO_*` hooks, and more.

So an operator debugging "is multitenancy on? are webhooks configured?" got a
silent NO from `config show` even when they were set — a misleading observability
surface.

## What
1. **Restored the full inventory** — `configEnvVars` now lists all 81 operator-facing
   vars (the union of the prior list and every non-test daemon read), sorted.
   Presence-only privacy is unchanged: even secret-bearing names like
   `AGEZT_TELEGRAM_TOKEN` / `AGEZT_TOKEN` report `true`/absent, never a value.
2. **Made the invariant self-enforcing** — new internal test
   `TestConfigEnvVars_CoversCmdAgeztReads` scans `cmd/agezt/*.go` for the two
   canonical read forms (`brand.EnvPrefix + "NAME"` and
   `os.Getenv/LookupEnv("AGEZT_NAME")`) and fails if any is absent from
   `configEnvVars`. The regexes match only real reads, not banner/help strings that
   merely mention a var. Verified it catches an omission (removing `AGEZT_WEBHOOKS`
   → the test fails naming it) and is green when complete. The inventory can no
   longer silently rot.

## Files
- `kernel/controlplane/config.go` — `configEnvVars` rewritten to the complete
  sorted set; comment updated to point at the guard test.
- `kernel/controlplane/config_inventory_test.go` (new, `package controlplane` so it
  can read the unexported list) — the drift guard.

## Live proof (offline mock)
Daemon started with `AGEZT_WEBHOOKS`, `AGEZT_MULTITENANT`, `AGEZT_SUBAGENT` set:
```
config show → env now includes:
  AGEZT_APPROVAL_MODE
  AGEZT_MODEL
  AGEZT_MULTITENANT     ← previously invisible
  AGEZT_PROVIDER
  AGEZT_SUBAGENT        ← previously invisible
  AGEZT_WEBHOOKS        ← previously invisible
```

## Verification
- 55 packages `ok`, **FAIL 0**; **1416 tests**. The guard catches a deliberately
  removed var and passes when complete.
- `go vet` clean, `GOOS=linux go build ./...` ok, `go.mod` / `go.sum` unchanged.
- gofmt: both touched files clean under LF (0 complaints).
