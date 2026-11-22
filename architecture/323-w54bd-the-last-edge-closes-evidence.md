# W5.4bd — the last edge closes: the archcheck allowlist is zero

**Status:** done (local)
**Date:** 2026-11-21
**Roadmap row:** W5.4 — *Final: archcheck allowlist = 0, forbidden-call allowlist = 0, codemap docs regenerated, `docs/` stale architecture docs archived.*

## The claim

The tree has **no allowlisted architecture edge left**. The import ratchet is
at zero, the forbidden-call ratchet is at zero, and every module boundary in
the repository is now a rule the gate enforces — not a line in a debt ledger.

This slice pays the **last** edge, and it is the only one that was still
excused:

```
cross-module kernel/modules/agents/selfrepair -> kernel/runtime
```

One line, in `tools/archcheck/allowlist.txt`, for years.

## What the edge was

The **self-repair module** — the agents module's doctor — held the whole
kernel handle and asked it for eleven verbs. Measured on the commit before
this slice (`f25bbb27`, in the eleven files that used the runtime):

| Measurement | Before | After |
|---|---|---|
| Kernel verbs the module asked for | **11** | **0** |
| Call sites | **22** | **0** |
| Daemon row types the module named | **7** (30 mentions) | **0** |
| Files in the package | 15 | 16 (`ports.go` is new) |
| Lines in the package | 2 199 | 2 384 |

The eleven verbs, and who owns each one:

| The module asked the daemon for | Its owner | How the module reaches it now |
|---|---|---|
| `k.Bus()` (7 sites) | `kernel/bus` — L2 platform | the port's `Bus` |
| `k.Journal()` (4) | the host's durable journal | the port's `Journal` (a walk, not the store) |
| `k.Provider()` (1) | `kernel/contract/llm` — L1 | the port's `LiveProvider` |
| `k.ReaperScan()` (1) | the agents module's **own** reaper | the port's `Scan` |
| `k.RosterGet()` / `k.RosterList()` (2) | the roster's rows (`agentapi`) | the port's `Profile` / `Profiles` |
| `k.RosterService()` (2) | the agents module's **own** service | the port's `Roster` |
| `k.RunnerService()` (2) | `kernel/modules/runs/api` — another module's **api** package | the port's `Runner` |
| `k.NewCorrelation()` (1) | the host's identity door | the port's `Correlation` |
| `k.WithAgentProfile()` (1) | the **daemon's** profile door | the port's `BindProfile` |
| `k.WithMaxCost()` (1) | the **daemon's** cost door | the port's `CapCost` |

Two of the eleven are the module's own business (the reaper, the roster
service), one is a legal cross-module reach (the runs module's *api* package —
archcheck's principle P5), and the rest are capabilities the host hands over.

## The two doors that stay

`k.WithAgentProfile` and `k.WithMaxCost` are **forwarding copies** of the run
context's rules, and this programme has spent a long time deleting forwarding
copies. These two stay, and the reason is written down in the daemon's own
ratchet (`kernel/runtime/runctx_ratchet_test.go`):

> the daemon kept only the doors a MODULE needs without naming the runs module
> — the profile and the ceiling — **the self-repair module binds a run through
> them**.

The ratchet **requires** those two doors to exist (`TestTheDaemonKeepsTheDoorsAndNothingMore`
pins their signatures; `TestTheProfileDoorBindsTheRun` pins that they bind).
Deleting them would have been a "cleanup" that removed the only sanctioned way
for a module to bind a run.

So the module does not name the run context's vocabulary; it asks the host to
bind the profile, through a port. The name stays the daemon's, the decision
stays the module's, and there is no second spelling of `WithAgentProfile`.

## The shape of the change

**`kernel/modules/agents/selfrepair/ports.go`** (new) — the module's ports.
Eleven fields, one per capability, each with a nil-tolerant reader so a host
that supplies nothing is *answered for* rather than called into:

| Port | What it carries |
|---|---|
| `Bus *bus.Bus` | the host's event bus |
| `Journal func(func(*event.Event) error) error` | the journal walk |
| `LiveProvider func() llm.Provider` | the live provider (a function: the provider is hot-swappable) |
| `Scan func(int64, int64) api.ReaperReport` | the module's own reaper |
| `Profile` / `Profiles` | the roster's rows |
| `Roster api.RosterService` | the roster's lifecycle |
| `Runner runsapi.RunnerService` | the runs module's run engine |
| `Correlation func() string` | the host's identity door |
| `BindProfile func(context.Context, roster.Profile) context.Context` | the daemon's profile door |
| `CapCost func(context.Context, int64) context.Context` | the daemon's cost door |

Every port is **optional**, and every reader answers for the host that
supplied nothing: no roster means "no such agent", no scanner means "an empty
fleet", no journal means "no repair history", no provider means "no live model
chain".

The **one** exception is the wake: a wake without the run engine, the
correlation or the profile binding **refuses**, because a wake that ran
without a profile would run somebody else.

**`cmd/agezt/selfrepair_port.go`** (new) — the composition root's half. The
daemon is the one place that knows both the kernel and the module, so the
daemon fills the ports (ten of the eleven are method values: `k.RosterGet`,
`k.RosterList`, `k.Provider`, `k.ReaperScan`, `k.NewCorrelation`,
`k.WithAgentProfile`, `k.WithMaxCost`, `k.RunnerService()`).

**The eleven production files** now speak the ports. The row types moved with
them: `kernelruntime.DegradedAgent` and its six siblings are the module's own
`api.*` rows now (they were always aliases — `api.DegradedAgent =
agentapi.DegradedAgent` — so the spelling moved to where the row lives).

## The ratchets

**Five tests in the module** (`kernel/modules/agents/selfrepair/ports_test.go`):

1. `TestTheSelfRepairModuleHoldsNoKernelHandle` — the module's **production**
   files must not import `kernel/runtime` or `kernel/modules/runs/runctx`.
   The check is over the whole package, not one file, because the edge could
   hide in any of them.
2. `TestTheModulePortsAnswerForAHostThatSuppliedNothing` — every port answers
   for the host that supplied nothing.
3. `TestTheWakeRefusesWithoutItsDoors` — the wake refuses rather than running
   an unprofiled agent.
4. `TestTheScanPortIsTheModulesOwnReaper` — the scan arrives through the
   module's own reaper vocabulary, with the caller's two cutoffs.
5. `TestTheProfileBindingIsTheHostsDoor` — the module **asks** the host to
   bind the profile and the cost ceiling; it does not bind them itself.

**Three tests in the daemon** (`cmd/agezt/selfrepair_port_test.go`):

6. `TestTheDaemonFillsEverySelfRepairPort` — every capability the module
   needs is handed over, and none of them is a nil door.
7. `TestTheSelfRepairProfileDoorBindsTheRun` — the door the daemon hands over
   is the daemon's door: the run really carries the profile and the ceiling.
8. `TestTheSelfRepairPortsAnswerForNoKernel` — a host with no kernel holds no
   healer.

The daemon's side matters as much as the module's: every port is optional by
design, so a daemon that forgets one does not fail — the healer goes **quiet**
instead of saying why. That is the failure the sixth test exists for.

## The mutations — three, three caught, all reverted

| # | The mutation | The gate that caught it |
|---|---|---|
| **M01** | The module names the daemon again (`api.DegradedAgent` → `kernelruntime.DegradedAgent`, runtime imported). | **Two** gates: `archcheck` reports `cross-module kernel/modules/agents/selfrepair -> kernel/runtime` as a **new** violation, and `TestTheSelfRepairModuleHoldsNoKernelHandle` fails with the import path in the message. |
| **M02** | The daemon forgets the profile door (`BindProfile: k.WithAgentProfile` removed from the wiring). | `TestTheDaemonFillsEverySelfRepairPort` fails ("the daemon must hand the module its profile door") and `TestTheSelfRepairProfileDoorBindsTheRun` panics on the nil door. |
| **M03** | The port stops asking the host: `bindProfile` returns the context unchanged. | `TestTheProfileBindingIsTheHostsDoor` fails ("bound profile = {…}, want the profile the module asked for"). |

M01 is the one that matters most: it is the whole slice put back, and it is
caught by the *tool* as well as by a test — the allowlist is empty, so the
edge has nowhere to hide.

All three were reverted; the tree after the revert measures `violations: 0`
and passes both test packages.

## The gates

| Gate | Result |
|---|---|
| `gofmt -l` (kernel, cmd) | clean |
| `go build ./...` / `go vet ./...` | clean |
| `go test ./...` | no failures |
| `go run ./tools/archcheck -summary` | **340 packages, 0 violations**; `call:exec` 0, `call:http-client` 0, `call:raw-write` 0; **0 allowlisted import violations and 0 allowlisted call sites** |
| `go run ./tools/deadcodecheck` | OK (33 public SDK findings + 1 cross-package test seam allowlisted) |
| `go run ./tools/depscheck` | OK (24 core dependencies) |
| `go run ./tools/structure-md -check` | clean — the codemap is current |
| `go run ./tools/docsindex` | OK |
| `go run ./tools/docclaimscheck` | OK |
| `go run ./tools/changelog-lint` | OK |
| `staticcheck` (GOTOOLCHAIN=go1.27.1) | clean |
| `go run ./tools/sdkgen -check` | clean — the generated catalogs are current |
| Rust SDK (`cargo fmt --check`, `cargo test`) | clean (3 tests) |
| frontend | **untouched** — this slice changes no frontend file, so the vitest baseline stands (the suite is already red on this machine: its Node build has no `localStorage`); `npm run typecheck` and `npx knip` pass |

## What W5.4's four parts now stand at

| Part | Where it stands |
|---|---|
| archcheck allowlist = 0 | **this slice** — the last import edge is gone; both allowlists hold comments only |
| forbidden-call allowlist = 0 | already true (`calls-allowlist.txt` holds no entries); the gate sees `exec` 0, `http-client` 0, `raw-write` 0 |
| codemap docs regenerated | `structure-md -check` is clean (`.project/STRUCTURE.generated/**` is current) |
| `docs/` stale architecture docs archived | W5.4f |

## What this slice does not do

- **The daemon's handle is still the daemon's.** `kernel/runtime` is 180 files
  and roughly 17.9k lines, most of it tests. The module stopped *reaching* for
  it; the daemon still *is* it. That is the W3 programme's business.
- **`kernel/modules/runs/api` did not grow a door.** The run context's two
  doors stayed the daemon's, so no second spelling of `WithAgentProfile` was
  created. The module asks the host instead.
- **The module's own rows moved only in spelling.** `api.DegradedAgent` and
  `agentapi.DegradedAgent` are the same type; the module now writes the
  module's name, and the daemon keeps its alias.
- **The ports are duplicated in the test host.** The module's test file builds
  the same eleven capabilities from a kernel handle, because the module may
  not hold one. The daemon's half is pinned by the daemon's own tests.
