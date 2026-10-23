# W3.9 — the reaper has a home, and the module has a door

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 step 8 — `modules/agents`
(roster, guardians, selfrepair, reaper, overseer ops). W3.8 delivered the
roster and the self-repair coordinator and named the reaper as the next slice;
this is that slice, together with the module's **api** package — the door the
W3.8 evidence said the package split belonged to.

## What moved

| Before | After | What it is |
|---|---|---|
| `kernel/runtime/reaper.go` + `reaper_scan.go` + `reaper_health.go` + `reaper_routing.go` + `reaper_probation.go` + `reaper_helpers.go` | `kernel/modules/agents/reaper` | the scan: the dead, degraded, misconfigured, retry-pressure, routing-pressure, forced-probation and unstable findings, plus the stale-artifact totals |
| the finding rows, declared in `kernel/runtime/reaper.go` | `kernel/modules/agents/api` | the module's vocabulary: the finding rows, the shared rows (`ConfigIssue`, `Binding`), the ports (`ReaperPorts`) and the surface (`ReaperScanner`) |
| `kernel/runtime/reaper.go` (the scan itself) | `kernel/runtime/reaper.go` (the host's face) | the aliases of the module's rows, `SetReaperScanner`, `ReaperPorts` and the delegating `ReaperScan` |

The reaper was the runtime's **health loop**: it reached into the kernel's own
fields for the journal, the roster, the artifact index, the schedules, the
standing orders and the provider. A module may import another module's api
package and nothing else (archcheck's principle P5), so the move could not be
an import-path change — it had to be the same **ports rewrite** W3.7 did for
the workflow engine.

## The module's door

`kernel/modules/agents/api` is the only place between the module and the host
where a row is declared. It holds:

- the finding rows (`ReaperAgent`, `DegradedAgent`, `MisconfiguredAgent`,
  `RetryPressureAgent`, `RoutingPressureAgent`, `RoutingForcedAgent` with the
  three phase names kept as aliases, `RoutingUnstableAgent`, `ReaperReport` +
  `Empty()`),
- the rows the host and the module share (`ConfigIssue` — one malformed
  runtime override; `Binding` — one trigger that names an agent),
- the module's **ports** (`ReaperPorts`): `Journal`, `Profiles`,
  `RuntimeConfigIssues`, `StaleArtifacts`, `TaskModelChain`, `Schedules`,
  `Standing`,
- the module's **surface** (`ReaperScanner`).

Every port is optional: a host that cannot supply one gets a nil port and the
module skips that finding — a fleet with no artifact index has no stale
artifacts to report.

**The doctor's validation is the host's, and the module asks.** The reaper
judges a profile's runtime overrides through the `RuntimeConfigIssues` port,
which the runtime answers with its own `agentRuntimeConfigIssues` — the same
function the doctor uses. So the doctor and the reaper cannot disagree about
what a valid override is. (`agentConfigIssue` is now an **alias** of the
module's `api.ConfigIssue`: one row, one home.)

## The wiring

The runtime keeps its public names as aliases of the module's rows and holds
the module as `api.ReaperScanner`:

```go
// kernel/runtime/reaper.go
type ReaperReport = api.ReaperReport

func (k *Kernel) SetReaperScanner(s api.ReaperScanner) { ... }
func (k *Kernel) ReaperPorts() api.ReaperPorts { ... }
func (k *Kernel) ReaperScan(agentIdleCutoffMs, artifactStaleCutoffMs int64) ReaperReport {
	svc := k.reaperService()
	if svc == nil {
		return ReaperReport{}
	}
	return svc.Scan(agentIdleCutoffMs, artifactStaleCutoffMs)
}
```

The composition root owns the concrete scanner, exactly as it owns the
reasoning module's service:

```go
// cmd/agezt/main_toolinvoker.go: openAppKernel
k.SetReasoning(reasoning.New(k.ReasoningDependencies()))
k.SetReaperScanner(reaper.New(k.ReaperPorts()))
```

The primary kernel and every tenant kernel go through `openAppKernel`, so both
are wired.

## The ratchet did not move

**275 packages, 126 import violations, 13 call sites** — the same 126/13 as
after W3.8, with two packages added and **no new edge**:

- the runtime imports `kernel/modules/agents/api`, and an **api package is
  exempt** from the cross-module rule (P5) — the same exemption the reasoning
  module's api package already uses;
- the module's implementation (`kernel/modules/agents/reaper`) is imported by
  exactly one place: the composition root (`cmd/agezt`, L7), which may reach
  anything;
- no adapter and no plugin grew an edge: the control plane, the overseer tool
  and the introspect tool keep naming the rows through the runtime's aliases,
  and the self-repair module names them through its own module's api package.

Had the engine gone anywhere else, the runtime would have had to import it —
a **new** cross-module edge (module `runs` → module `agents`), and the ratchet
only tightens.

## The proof

Permanent tests:

- `kernel/modules/agents/reaper/reaper_test.go` — the module's own suite, run
  over the **host's real ports** (the same ports the daemon wires), so a port
  that lies fails here rather than in production: the grace window, the shipped
  guardians, the paused and retired agents, the health threshold, the runtime
  overrides, the hierarchy references, the trigger bindings, the routing
  pressure, the retry pressure, the forced probation, the forced failure and
  exhaustion, the unstable chains.
  - `TestReaperScan_JudgesTheFleetTheHostGave` — the module judged a fleet
    **handed over through the ports**: an agent that ran a task after the
    cutoff is alive, one that did not is a candidate, and the report carries
    when it was last seen. The stale-artifact totals travel too.
  - `TestReaperScan_WithoutPortsFindsNothing` — a host with no ports gets an
    empty report, not a crash.
- `kernel/modules/agents/api/api_test.go` — the vocabulary: every finding row
  keeps every field, the three forced phases are one row, and `Empty()` is
  empty only when there is nothing to reap.
- `kernel/runtime/reaper_wiring_test.go` — the host's face: the kernel's scan
  **is** the module's scan (the cutoffs travel with it and its report comes
  back), an unwired kernel answers with an empty report instead of a panic,
  and the ports carry the host's own journal and fleet.
- `cmd/agezt/main_toolinvoker_test.go` — `TestOpenAppKernelWiresTheReaper`: the
  daemon's kernel really answers a scan. The wiring is a permanent test, not a
  convention.
- `kernel/controlplane/reaper_app_test.go` and
  `kernel/controlplane/controlplane_test.go` — the control plane's own
  fixtures now wire the scanner the way the daemon does.

## Mutations — 21 mutations, 21 caught

| # | Mutation | Caught by |
|---|---|---|
| M01 | the grace window is dropped (a fresh agent is judged) | `TestReaperScan_FlagsIdleFiltersRetiredPausedAndNew` |
| M02 | the shipped guardians are reaped too | the same |
| M03 | the paused and retired agents are reaped too | the same |
| M04 | an agent that ran a task recently is reaped too | `TestReaperScan_JudgesTheFleetTheHostGave` |
| M05 | the dead agents are not sorted | the same |
| M06 | the stale-artifact totals are dropped | the same |
| M07 | the failure threshold is off by one | `TestReaperScan_FlagsDegradedAgentsFromHealthPolicy` |
| M08 | the runtime overrides are not judged | `TestReaperScan_FlagsMisconfiguredAgentsFromRuntimeOverrides` |
| M09 | the hierarchy references are not judged | `TestReaperScan_FlagsMisconfiguredAgentHierarchy` |
| M10 | the trigger bindings are not judged | `TestReaperScan_FlagsMisconfiguredAutomationBindings` (strengthened) |
| M11 | a retired agent is a valid **schedule** target | the same (strengthened) |
| M12 | a managed sub-agent may be called directly | the same |
| M13 | the model-chain fallbacks are not counted | `TestReaperScan_FlagsRoutingPressureAgentsFromModelFallbacks` |
| M14 | a fallback outside the model chain is counted | the same |
| M15 | the retries are not counted | `TestReaperScan_FlagsRetryPressureAgents` |
| M16 | a forced chain is not read as a forced one | `TestReaperScan_SuppressesRoutingPressureDuringForcedChainProbation` |
| M17 | the exhausted chains are never exhausted | `TestReaperScan_FlagsForcedChainExhaustionAfterRepeatedForcedGenerations` |
| M18 | the rollback threshold is off by one | `TestReaperScan_FlagsRoutingUnstableAgentsAfterRollbackAndRenewedPressure` |
| M19 | a report with stale artifacts claims to be empty | `TestEmptyReportsNothingToReap` |
| M20 | an unwired kernel is asked anyway | `TestReaperScan_WithoutTheModuleAnswersEmpty` |
| M21 | the cutoffs do not travel to the module | `TestReaperScan_DelegatesToTheInjectedModule` |

**The run found three gaps in the tests and they are fixed and named.** They
are the same class of gap as W3.8's: a test that asserted the *finding* but
not the *whole* finding.

1. **M10 escaped.** The binding findings were only asserted on their own, so
   an agent with a broken override **and** a broken binding lost one of the
   two — the binding issue was still published (by the second loop, which adds
   the binding-only agents), so the finding was never lost; what was lost was
   the *merge* of the two into one row. The fixture now gives the managed
   sub-agent both a broken override and a broken binding and asserts that both
   are in its row.
2. **M11 escaped.** The retired agent's **standing order** was exercised but
   its **schedule** was not, so the schedule branch's retired check was
   uncovered. The fixture now binds the retired agent to both.
3. **M04 and M06 would have escaped.** Neither "an agent that ran a task
   recently is not a candidate" nor the stale-artifact totals was covered at
   all — the old suite only ever judged agents that had never run. The new
   port-driven test judges a fleet handed over by the host and asserts both.

Two further notes on the run, recorded honestly: M05's first anchor did not
compile (removing the sort left `sort` unused) and M15's first anchor did not
match the file (the retry function's block is indented one level less). Both
were re-run with compiling, matching anchors. A build failure is not an
escape and is not counted as one; a skipped anchor is not a catch either.

## The gates

All green: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (**275 packages, 126 import violations, 13 call sites** — the
ratchet unchanged), `tools/structure-md` (+ its `-check`),
`tools/changelog-lint`, `tools/docclaimscheck`, `staticcheck`, and the full
`go test ./...`.

## What the move cost

- **The scan became a dependency to wire.** The kernel cannot import a module's
  implementation, so the scanner is injected by the composition root. Three
  test call sites had to wire it (the control plane's reaper fixture, the
  control plane's shared fixture, and the daemon's own test) — and the daemon's
  wiring is now a permanent test, because an unwired kernel answers with an
  **empty** report: the fleet would look healthy.
- **The runtime's `agentConfigIssue` became an alias.** The module and the
  host now name the malformed-override row once, in the module's api package.
- **The rows are unchanged.** The runtime's aliases are the module's rows, so
  the control plane, the overseer tool, the introspect tool, the application
  layer and the self-repair module all compile unchanged and publish the same
  wire.

## What is deliberately left

- **The overseer ops.** Step 8's third item — the overseer tool as an L6
  caller of `app` ops — is still open. It was the part that needed the api
  door, and the door now exists.
- **The `agents` module's other vocabulary.** `agents/api` holds the reaper's
  rows today. The module's public operations (`Create/Edit/Remove/Wake`,
  `Repair`, `Overseer.*`) will land there as their callers are rewired.
- **`modules/extensions`** (mcp, plugin host, toolforge, toolbox, acpcatalog)
  is the next W3 step, then `modules/providers` and `modules/runs` last.
