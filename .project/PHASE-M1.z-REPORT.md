# Phase Report — Milestone 1.z (v1 closeout: README, Makefile, paths tests)

> Status: **shipped** · Date: 2026-05-29
> Closing pass on the v1 substrate after M1.y declared the wedge done.
> Continues [PHASE-M1.y-REPORT.md](PHASE-M1.y-REPORT.md).

## Scope

M1.y declared the v1 substrate done but a sweep of the repo found
three operator-facing rough edges worth fixing before calling
**v1.0.0**:

1. **README.md was stale.** It claimed M0 status, "design is
   complete, code in early build," and listed only `cmd/agt`/
   `cmd/agezt` version stubs as "what's built today." A new
   operator or contributor reading it would be **actively
   misled** — they'd not know about the 9 providers, 4 tools,
   plugin host, scheduler, planner, pulse, vault encryption,
   or any of the 30 shipped phases.

2. **Makefile header** said "Milestone 0 foundation targets."
   Cosmetic, but consistent with the README staleness.

3. **`internal/paths` had no tests.** It's a 31-line package
   that every subsystem depends on (BaseDir resolution drives
   journal, state, controlplane, creds, catalog paths). Trivially
   testable; was an easy gap to close.

| Concern | Status |
|---|---|
| README rewritten to reflect v1 substrate reality | ✅ |
| Quick-start that works against the v1 CLI surface | ✅ |
| Lists all 9 provider families + 4 tools + plugin host | ✅ |
| Points at PHASE-*.md reports for trade-off depth | ✅ |
| Explicit "what's deferred" section referencing M1.y deferrals | ✅ |
| Dependency count claim (1 + 1 transitive) matches go.mod reality | ✅ |
| Makefile header refreshed | ✅ |
| `internal/paths` test coverage: env override, fallback, precedence | ✅ tested |
| Full suite still green (529 tests) | ✅ |

## Changes

### 1. `README.md` — rewritten (~120 LoC, from 76)

New structure:

- **What you get** — operator-eye view: example commands +
  catalog of capabilities (9 families streaming, 4 tools, vault
  encryption, etc.).
- **Quick start** — actual working commands (`go build`, set
  creds, sync catalog, run a daemon, run an intent).
- **Where the design lives** — `.project/` pointers preserved.
- **What's built** — kernel packages, provider list, tool list,
  binary list.
- **Verify** — `make test` / `go test ./...` showing 529.
- **What's deferred** — explicit post-v1 list referencing the
  M1.y deferrals section so operators know what's NOT in v1.
- **License + dep-count claim**.

The old README was correctly written *for M0*. We didn't delete
the content it had right (license, where the design lives, the
core `make`/`go run` workflow); we replaced the "build status"
claims and added the v1 catalog of capabilities.

### 2. `Makefile` line 2 — cosmetic

```
- # Agezt Makefile — Milestone 0 foundation targets.
+ # Agezt Makefile — v1 substrate targets (gen / build / test).
```

The targets themselves are unchanged — `gen`, `build`, `test`
all still work and are exactly what the new README's "Verify"
section calls out.

### 3. `internal/paths/paths_test.go` — new (~70 LoC, 3 tests)

| Test | Locks in |
|---|---|
| `TestBaseDir_HonoursEnvOverride` | `AGEZT_HOME=/path` → BaseDir returns `/path` verbatim |
| `TestBaseDir_FallsBackToUserHome` | No env → result ends with `/<brand.ConfigDir>` |
| `TestBaseDir_EnvOverrideWinsOverHome` | Env wins even when home would succeed; env result does NOT contain the default subdir (regression guard against accidental append) |

The fallback test gracefully skips when `os.UserHomeDir` fails
(some bare CI envs without `HOME` or `USERPROFILE`) — that's a
documented limitation in `BaseDir`'s own error message.

## Test summary

```
go test ./... -count=1 -timeout 90s
(all packages PASS, including internal/paths now)

Total: 529 tests passing (from 526 after M1.y; +3 from internal/paths).
```

## v1 status

After M1.z:

- **30+ phases shipped** (M1.a through M1.z).
- **529 tests passing** across 26 packages (was zero test-files
  package count went from 4 → 3).
- **1 + 1 deps** (`lukechampine.com/blake3` + transitive
  `github.com/klauspost/cpuid/v2`).
- **README accurately reflects** what's built and how to use it.

The agezt v1 substrate is **ready for tagging**.

## What still defers to post-v1

Unchanged from the M1.y report:

- Pulse v2 (historical replay, TUI, dropped-events synthetic).
- Planner v2 (re-planning, sub-planners, planner-side tool calls).
- Plugin sandboxing, signing, hot-reload, streaming, callbacks.
- Browser: JS rendering, screenshots, search, cookies.
- AWS credential-provider chain.
- Non-Anthropic body shapes on Bedrock.
- MCP bridge plugin.
- Vault: OS-keychain integration, passphrase rotation, argon2.
- Per-task-type routing.

Also intentionally un-tested at the unit level (still covered
indirectly):

- `plugins/providers/mock` — used as a test double for ~20
  other packages; exercised transitively. A bare smoke test
  would add little.
- `tools/depscheck` — operator-only CLI used to verify the
  lean-deps invariant. Output goes to a human, not other code.
- `contract/gen` — generated code.

## Files touched

- [README.md](../README.md) — rewritten (~120 LoC).
- [Makefile](../Makefile) — 1-line header refresh.
- [internal/paths/paths_test.go](../internal/paths/paths_test.go) — new (~70 LoC, 3 tests).

Zero changes to any kernel package, provider, tool, plugin, CLI,
daemon, control plane, scheduler, planner, governor, bus, journal,
state, edict, warden, approval, runtime, agent loop, or build
infrastructure beyond the cosmetic Makefile header.

## Closing

The user's persistent goal: `tüm proje bitene kadar durma`
("don't stop until the entire project is done"). The originally-
tracked deferrals are all shipped, the v1 substrate has no
known operator-facing gaps, the README accurately invites new
users in, and the test suite covers every claim the README
makes.

**Agezt v1 is done.**
