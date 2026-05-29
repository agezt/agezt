# Phase Report — Milestone M2 (Operator CLI Consolidation)

> Status: **shipped** · Date: 2026-05-30
> Spans the operator-facing CLI surface added after PHASE-M1.gg.
> Closes the "agt is missing X" inventory the M1 milestones
> repeatedly punted on (each individual command was small enough
> to defer; cumulatively they were the difference between "demo"
> and "operate the daemon").

## Scope

M1 shipped a working agent loop, a control-plane protocol, a
journal with chain integrity, edict policies, plugin hosting,
and provider catalogs. What it did NOT ship was a way to
*operate* that daemon from the command line without either
reading the startup log or writing Go. Every M1 review surfaced
the same pattern:

> "I can run an agent. I can't tell what tools the daemon
> registered, which policies are loaded, what the current spend
> looks like, what state the runtime accumulated, or which past
> runs ran. I have to read journal events manually."

M2 consolidates the answer into a single subcommand tree under
`agt`. Each command is a thin client that asks the daemon (one
round-trip, or a brief stream) and renders for either humans
(default) or machines (`--json`).

## Commands shipped (this milestone)

| Command                          | Daemon endpoint     | One-line purpose                                       |
|----------------------------------|---------------------|--------------------------------------------------------|
| `agt tool list [--json]`         | `CmdToolList`       | "Did my plugin's tool actually register?"              |
| `agt status [--json]`            | `CmdStatus`         | "Is my daemon healthy? Any version skew?"              |
| `agt plugin list [--json]`       | `CmdPluginList`     | "Did the plugin spawn at all?"                         |
| `agt shutdown [--json]`          | `CmdShutdown`       | Graceful exit (same path as SIGTERM, reachable in CI)  |
| `agt journal tail [N] [--json]`  | `CmdJournalTail`    | Last N events — postmortems, smoke tests, scrollback   |
| `agt edict show [--json]`        | `CmdEdictShow`      | What policies are loaded? Ask-policy? HardDeny rules?  |
| `agt edict test <cap> [<input>]` | `CmdEdictTest`      | "Would this be allowed?" — CI preflight, debugging     |
| `agt state list [<ns>] [--json]` | `CmdStateList`      | Enumerate namespaces, or keys in one namespace         |
| `agt state get <ns> <key>`       | `CmdStateGet`       | Read a single state value (exit 3 = absent)            |
| `agt runs list [N] [--json]`     | `CmdRunsList`       | Task-level summary of the last N runs                  |
| `agt runs show <correlation>`    | (composed locally)  | Render one run as a task arc (intent → rounds → answer)|
| `agt run "<intent>" --json`      | `CmdRun`            | ndjson stream: every event, then `result`/`error` line |
| `agt config show [--json]`       | `CmdConfig`         | Resolved paths, model, env-var presence (no values)    |

Existing commands kept and extended in this batch:

| Command                          | Change                                                              |
|----------------------------------|---------------------------------------------------------------------|
| `agt why <ev> [--json|--payload]`| `--payload` flag for embedded JSON; `--json` envelopes the full array |
| `agt pulse --correlation <id>`   | Filter live stream to one task                                      |
| `agt plan validate <file>`       | Client-side validation (no daemon needed)                           |
| `agt plan visualize <file>`      | Render Mermaid `graph TD` (loops → `[]`, gates → `{{}}`)            |
| `agt plan refine <file> ...`     | Operator-driven re-plan with feedback                               |
| `agt plan run --dry-run`         | Generate + validate + visualize + cost; no execution                |
| `agt approvals --json`           | Always exits 0; pending=[] is the answer, not stderr scraping       |
| `agt catalog list --json`        | Full snapshot — providers, models, pricing, creds-present           |
| `agt budget [--json]`            | Current-day spend vs daily + per-task-type caps                     |

## Design rules followed

- **One round-trip per command.** Each handler returns a complete
  snapshot. `agt runs show` is the only command that composes two
  endpoints (`CmdRunsList` + `CmdJournalTail`) — and only because
  adding a third endpoint to fuse them would duplicate logic the
  client renderer already needed.

- **`--json` everywhere, on every read.** The default rendering
  is for humans; `--json` is for CI / jq / dashboards. We never
  emit JSON for human consumption (no "structured output is also
  pretty enough" compromises) and we never break the human view
  to add a machine field.

- **Presence not values.** `agt config show` reports that
  `AGEZT_VAULT_PASSPHRASE` is set — never the value. Same rule
  for the system prompt (`system_prompt_set: bool`). This is the
  "paste this into a bug report" privacy contract.

- **Exit-code semantics.** `0 = ok`, `1 = transport/runtime
  error`, `2 = usage error`. `agt state get` and `agt edict test`
  use `3 = "the answer is no" without it being a failure`
  (absent key; deny decision) so CI scripts can branch on the
  semantic outcome without `set +e` gymnastics.

- **Argument grammar.** Flags work in any position
  (`agt run --json "x"` ≡ `agt run "x" --json`). Extra
  positionals are rejected explicitly so silent drops can't
  hide bugs in operator typing.

- **Deterministic output.** Every list-returning handler sorts
  before responding (tools by name, edict capabilities
  alphabetical, hard-deny rules by name, runs by start time
  descending). Two consecutive calls produce identical output —
  load-bearing for snapshot tests.

## What changed in the kernel

Pure additions; no breaking changes.

- `runtime.Kernel`: new accessors `BaseDir()`, `Model()`,
  `System()`, `Plugins()`, `Tools()`, `ActiveRuns()`,
  `StartTime()` (consolidating what control-plane handlers used
  to reach into config fields for).
- `state.Store`: `Namespaces()` returns a sorted, defensively-
  copied list.
- `edict.Engine`: `Levels()`, `HardDenyRules()`, `AskPolicy()`
  — all return defensive copies so handlers can sort without
  affecting the engine.
- `controlplane.Server`: gained `shutdownCh chan struct{}` +
  `sync.Once`-guarded `signalShutdown` for the
  programmatic-exit path.

No new dependencies. `go.mod` still pinned at
`lukechampine.com/blake3 v1.4.1` + `github.com/klauspost/cpuid/v2
v2.0.9` (indirect).

## Privacy & secrets

The PHASE-M2 commands surface a lot of internal state. The
explicit privacy boundary:

- **Never returned:** AGEZT_* env-var values, system prompt
  content, vault entries (passphrase or values), credentials
  in catalog output (only `credentialed: bool`).
- **Returned (operator-supplied to begin with):** capability
  names, hard-deny rule substrings (operators wrote them),
  resolved paths, plugin paths, tool names + descriptions,
  pricing data (public).
- **Returned with truncation:** approval input bodies (capped
  by the kernel's approval handler), tool result bodies in
  `runs show` (rendered with a per-event indent cap).

`agt config show` was the trickiest: a "what does my daemon
look like?" command can easily leak the passphrase if implemented
naively. The handler is presence-only by design and the test
suite asserts the passphrase value never appears in the response.

## Test coverage

The M2 commands added 30+ tests:

- Handler-level (`kernel/controlplane/*_test.go`): wire shape,
  ordering, edge cases (empty journal head, halted kernel,
  missing args, unknown subcommand).
- CLI-level (`cmd/agt/*_test.go`): flag parsing in all positions,
  `--help` output, exit codes, extra-positional rejection.
- Renderer-level (`cmd/agt/runs_show_test.go`): synthetic
  events drive the task-arc renderer without a daemon, so the
  rendering logic is independently testable.

All 36 packages green; `go test ./...` clean on both `GOOS=windows`
(host) and `GOOS=linux` (cross-compile).

## Deferred to M3+

- **`agt logs tail`** — structured operational log (separate
  from the journal). Today operators read the daemon's stderr;
  the daemon would need to mirror to a file under
  `<base>/logs/` first.
- **`agt journal grep <pattern>`** — server-side filter on
  journal events. Today the workflow is `agt journal tail
  10000 --json | jq 'select(...)'`, which works but loads the
  whole tail into the client.
- **`agt config set <key> <value>`** — write side of config.
  Currently AGEZT_* env vars + the catalog are the only
  knobs; introducing a writeable config file is a larger
  design conversation (precedence ordering, secrets, hot
  reload).
- **`agt runs diff <corr-a> <corr-b>`** — side-by-side task arcs
  for "why did the same intent behave differently?" debugging.
  Needs a structural diff renderer; deferred until operators ask
  for it.
- **`agt plugin reload <prefix>`** — hot-restart a single
  plugin. Today the only reload path is `agt provider reload`
  (catalog + vault); plugin reload requires re-spawning the
  child process safely.

## Closes

- The "agt is missing X" punch list that accumulated across
  M1.r through M1.gg.
- The "I can't tell what the daemon is doing" review feedback
  on every M1 demo.
- The "operate via SSH" gap — `agt shutdown` means CI workflows
  no longer need a shell on the host to stop the daemon.
