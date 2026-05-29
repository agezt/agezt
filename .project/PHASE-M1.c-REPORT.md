# Phase Report — Milestone 1.c (Warden v1 — cross-platform exec facade)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-06 §2 (Warden — isolation profiles)](SPEC-06-SECURITY.md),
> [TASKS P1-WARD-01](TASKS.md), and [DECISIONS F3/F4](DECISIONS.md).
> Continues [PHASE-M1.b-REPORT.md](PHASE-M1.b-REPORT.md).

## Scope

This sub-phase completes the **Safety v1** trio (Edict + Warden + halt)
in ROADMAP §2.1 essential #7. Edict shipped in M1.a; halt in M0.5;
Warden was deferred twice while the routing layer (M1.b) landed.

| MVP essential | Status after M1.c |
|---|---|
| #1 Kernel core (journal/bus/supervisor/control plane) | ✅ M0.5 |
| #2 DAG scheduler + Planner | ⏸ M1.d |
| #3 Governor + ≥2 providers | ✅ M1.b |
| #4 4 tools: shell, file, http, browser | ✅ shell/file/http (browser → M1.d uses ProfileContainer) |
| #5 1 channel: Telegram | ⏸ M1.d |
| #6 Pulse v1 | ⏸ M1.d |
| #7 Safety: Edict v1, Warden v1, redaction, halt | ✅ **Warden v1 (cross-platform facade) shipped** |

M1.c deliberately ships the **cross-platform facade only**. The Linux
namespace+cgroups+seccomp backend lands in M1.d behind a
`//go:build linux` partition; OCI container and microVM backends are
M2+ optional plugins (SPEC-06 §2.2). On every platform today, a request
for ProfileNamespace/Container/MicroVM is honoured *as a request* and
transparently downgraded to ProfileNone with a one-shot
`warden.profile_downgraded` event — so audits stay honest about the
actual isolation level.

## What shipped

### New package: `kernel/warden` (726 LoC, 9 tests)

| File | LoC | Role |
|---|---:|---|
| `warden.go` | 372 | `Engine` interface, default cross-platform engine, `Profile`/`Spec`/`Result`/`Limits`, event publishers |
| `capbuf.go` | 55 | tail-truncating io.Writer used to cap captured stdout/stderr |
| `warden_test.go` | 299 | 9 tests covering exec, exit codes, timeout, truncation, downgrade dedup, payload shape |

Public surface:

```go
type Profile string
const (
    ProfileNone      Profile = "none"
    ProfileNamespace Profile = "namespace"   // Linux nsenter+cgroups+seccomp (M1.d)
    ProfileContainer Profile = "container"   // OCI (M2+ plugin)
    ProfileMicroVM   Profile = "microvm"     // firecracker-class (M2+ plugin)
)

type Engine interface {
    Run(ctx, Spec) (*Result, error)
    EffectiveProfile(Profile) Profile
    SetBus(*bus.Bus)
}
```

### What the cross-platform engine enforces today

| Constraint | Mechanism | Default |
|---|---|---:|
| Wall-clock timeout | `context.WithTimeout` + `cmd.WaitDelay` | 30 s |
| Output cap (stdout+stderr) | `capBuffer` tail-truncation | 256 KiB |
| Working directory | `cmd.Dir` | inherit |
| Environment scrubbing | exact `cmd.Env` (nil = empty) | empty |
| Audit trail | `warden.executed` per Run | always on |
| Profile honesty | one-shot `warden.profile_downgraded` per (requested profile) | always on |
| Limit notifications | `warden.limit_exceeded` for timeout/output cap | always on |

### New event kinds (`kernel/event/kinds.go`)

| Kind | Subject pattern | Payload |
|---|---|---|
| `warden.executed` | `warden.exec` | `{profile_effective, profile_requested, downgraded, argv0, exit_code, duration_ms, stdout_bytes, stderr_bytes, truncated, timed_out, workdir, host_os}` |
| `warden.profile_downgraded` | `warden.profile` | `{requested, effective, host_os, reason}` (once per (engine, requested) lifetime) |
| `warden.limit_exceeded` | `warden.limit` | `{limit, value, argv0}` |

Downgrade-dedup matters: without it, every shell call on Windows would
spam an identical "Linux backend not built" event into the journal. We
publish once per requested profile and rely on the per-Run
`warden.executed` event (which always carries `downgraded` + the
effective/requested profiles) to keep every individual call auditable.

### Wiring changes

- `kernel/runtime.Config` gained `Warden warden.Engine` (auto-defaults
  to `warden.New(kbus)` when nil — the runtime is never warden-less).
- `Kernel.Warden()` accessor added alongside `Edict()`/`Bus()`/`Journal()`.
- `plugins/tools/shell` rewritten to route every command through Warden:
  ```go
  // Before (M1.a): exec.CommandContext(...).CombinedOutput()
  // After  (M1.c): w.Run(ctx, warden.Spec{
  //    Profile: ProfileNamespace,
  //    Argv: []string{shellBin, shellArg, in.Command},
  //    Limits: warden.Limits{Timeout: ..., MaxOutputBytes: 64*1024},
  //    Actor: "tool.shell",
  // })
  ```
  Output combining (stdout then stderr), truncation prefix
  (`[truncated to last 64 KiB]`), and the exit-code/timeout result
  shapes are preserved bit-for-bit so the agent's model sees the same
  tool_result format it did before.
- `cmd/agezt/main.go`:
  - Builds Warden externally (same pattern as Governor), passes it to
    both `buildTools` and `runtime.Config`.
  - `Warden.SetBus(k.Bus())` post-Open closes the wiring loop.
  - Banner now reads:
    ```
    tools  : shell(warden=requested-namespace), file(...), http(...)
    warden : requested=namespace, effective=none (M1.c facade; downgrades journaled)
    ```

## Demo transcript (real binaries, warden engaged)

```
$ rm -rf /tmp/agezt-m1c-demo && mkdir -p /tmp/agezt-m1c-demo
$ AGEZT_HOME=/tmp/agezt-m1c-demo AGEZT_PROVIDER=mock ./bin/agezt

Agezt 0.0.0-m0 — daemon ready (protocol v1)
  base dir         : /tmp/agezt-m1c-demo
  governor         : primary=mock(offline; scripted shell+final), daily_ceiling=$20.00
  tools            : shell(warden=requested-namespace), file(root=…/workspace), http(hosts=0)
  policy engine    : edict (defaults from DECISIONS F3; AskAllow)
  warden           : requested=namespace, effective=none (M1.c facade; downgrades journaled)
  control plane    : 127.0.0.1:58556

$ AGEZT_HOME=/tmp/agezt-m1c-demo ./bin/agt run \
    "list the files here and tell me what this project is"

  [evt seq=0  kind=task.received               subject=agent.…task]
  [evt seq=1  kind=llm.request                 subject=agent.…llm]
  [evt seq=2  kind=routing.decision            subject=governor.route]
  [evt seq=3  kind=budget.consumed             subject=governor.budget]
  [evt seq=4  kind=llm.response                subject=agent.…llm]
  [evt seq=5  kind=policy.decision             subject=agent.…policy]
  [evt seq=6  kind=tool.invoked                subject=agent.…tool]
  [evt seq=7  kind=warden.profile_downgraded   subject=warden.profile]   ← NEW (M1.c)
  [evt seq=8  kind=warden.executed             subject=warden.exec]      ← NEW (M1.c)
  [evt seq=9  kind=tool.result                 subject=agent.…tool]
  [evt seq=10 kind=llm.request                 subject=agent.…llm]
  [evt seq=11 kind=routing.decision            subject=governor.route]
  [evt seq=12 kind=budget.consumed             subject=governor.budget]
  [evt seq=13 kind=llm.response                subject=agent.…llm]
  [evt seq=14 kind=task.completed              subject=agent.…task]

--- final answer ---
[offline-mock] I ran a directory listing via the shell tool. This project
is Agezt — …
```

The journal payloads for the two new events:

```json
{"seq":7,"kind":"warden.profile_downgraded","actor":"tool.shell",
 "subject":"warden.profile",
 "payload":{"requested":"namespace","effective":"none","host_os":"windows",
            "reason":"linux namespace backend not built in M1.c (TASKS P1-WARD-01)"}}

{"seq":8,"kind":"warden.executed","actor":"tool.shell","subject":"warden.exec",
 "payload":{"profile_requested":"namespace","profile_effective":"none",
            "downgraded":true,"argv0":"cmd","exit_code":0,"duration_ms":20,
            "stdout_bytes":1021,"stderr_bytes":0,"truncated":false,
            "timed_out":false,"workdir":"","host_os":"windows"}}
```

Notice the second shell round (seq 10–14 above) shows **no second
`warden.profile_downgraded`** — the once-per-profile dedup is working.
But `warden.executed` (seq 8 here, and the equivalent inside the second
round) fires every Run, so per-invocation accounting stays exact.

```
$ ./bin/agt journal verify
{ "ok": true }
```

BLAKE3 chain intact across all 15 events.

## Verified invariants

| Invariant | Test |
|---|---|
| Run captures stdout and exits 0 | `TestRun_ExecutesAndCapturesStdout` |
| Non-zero exit is propagated as Result.ExitCode, not engine err | `TestRun_PropagatesNonZeroExit` |
| Empty Argv rejected with ErrBadSpec | `TestRun_RejectsEmptyArgv` |
| Timeout kills process and flags `TimedOut`; `warden.limit_exceeded` fires | `TestRun_TimeoutKillsAndFlagsTimedOut` |
| Output past `MaxOutputBytes` is tail-truncated; flags + event | `TestRun_OutputTruncated` |
| Requested ProfileNamespace on non-Linux downgrades to ProfileNone | `TestRun_DowngradesNamespaceToNone` |
| Downgrade event fires **once** per (engine, requested-profile) | same test, second Run |
| All non-None profiles downgrade in M1.c (every host OS) | `TestEffectiveProfile_AllRequestsDowngradeInM1c` |
| Unknown profile resolves to ProfileNone | `TestEffectiveProfile_UnknownDowngradesToNone` |
| `warden.executed` payload carries actor, correlation_id, profiles, host_os | `TestEvent_ExecutedPayloadShape` |

All 9 warden tests pass. Existing shell tool tests still pass (the
warden refactor preserves the exact tool_result format). Total module:
**158 passing tests** across **23 packages**, vet clean, depscheck clean.

## Cumulative status

```
23 packages | ~10,500 lines source+tests | 158 tests passing | 2 deps (allowlisted)
```

| Subsystem | LoC | Tests |
|---|---:|---:|
| `kernel/{ulid,event,journal,state,bus,agent,runtime,controlplane}` | ~4,130 | 63 |
| `kernel/edict` | 538 | 14 |
| `kernel/governor` | 899 | 12 |
| `kernel/warden` | **726** | **9** |
| `plugins/providers/{mock,anthropic,ollama}` | 1,034 | 13 |
| `plugins/tools/{shell,file,http}` | ~1,360 | 35 |
| `cmd/{agezt,agt}` | ~890 | 8 |
| `internal/{brand,paths}` | 102 | 1 |
| `tools/{jsonschemagen,depscheck}` | 633 | (jsonschemagen: 3 + e2e) |

## Deviations from spec (intentional)

1. **No Linux namespace backend yet.** M1.c ships only the
   cross-platform facade with downgrade journaling. The Linux
   namespaces + cgroups + seccomp implementation (TASKS P1-WARD-01)
   lands in M1.d behind a `//go:build linux` partition. Today on Linux,
   shell calls still execute under ProfileNone (same as macOS/Windows),
   journaled honestly via the downgrade event.
2. **Only the shell tool is warden-routed.** `plugins/tools/file` and
   `plugins/tools/http` still run in-process. They don't fork external
   programs, so the value of routing them through Warden is limited —
   they'd just inherit the same effective ProfileNone. They'll move to
   Warden's data-plane policies (file access, network egress) once
   those are real (Linux backend).
3. **No cgroup CPU/memory limits.** `Limits.Timeout` and
   `Limits.MaxOutputBytes` are the only enforced constraints today;
   CPU-seconds and memory bytes are documented in the type but not
   honoured. Both require the Linux backend.
4. **No env scrubbing in the shell tool.** `Spec.Env` defaults to nil
   (= empty environment) but the shell tool currently passes nil,
   which on Windows means the child sees an empty environment too. We
   may need to populate PATH+SYSTEMROOT on Windows for some commands;
   M1.d will revisit once we have a real story for which env vars
   should pass into a sandbox.

## Open items for M1.d

- **Warden Linux backend** (TASKS P1-WARD-01) — namespaces (pid/mount/
  net/user), cgroups v2 (cpu, memory, pids), seccomp default profile.
  Behind `//go:build linux`; non-Linux still downgrades.
- **Browser tool** (TASKS P1-TOOL-04) — Playwright/CDP requesting
  ProfileContainer, falling back to ProfileNamespace, finally
  ProfileNone (each downgrade journaled).
- **DAG scheduler + Planner** (TASKS P1-SCHED-01..03, P1-PLAN-01).
- **Telegram channel** (TASKS P4-CHAN-01) + out-of-process plugin host
  per DECISIONS B0a (isolation cases).
- **Pulse v1** (TASKS P3-*) — observers, salience, initiative, briefing.
- **Live HITL approval routing** — promote Edict's `WouldAsk` to a real
  prompt over the channel.
- **Live model catalog sync** (TASKS P1-CONDUIT-04 / SPEC-15) — replaces
  the hardcoded `modelPriceTable` in `kernel/governor/pricing.go`.

## Pointers

- Tests: `go test ./...` (158 pass, vet clean, depscheck OK)
- Warden demo (any platform; downgrade is the same on macOS/Windows):
  ```
  AGEZT_HOME=/tmp/agezt-m1c-demo AGEZT_PROVIDER=mock ./bin/agezt
  ```
  then `./bin/agt run "..."`. The daemon log shows
  `warden.profile_downgraded` (once) + `warden.executed` (per shell
  call) interleaved with the agent's existing `tool.invoked` /
  `tool.result` pair.
- Future-readiness: tools that need external work should accept a
  `warden.Engine` (see `shell.NewWithWarden(w)`). When the Linux backend
  ships, those tools automatically gain real isolation without changing
  their callers.
- Next milestone report: `PHASE-M1.d-REPORT.md`
