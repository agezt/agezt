# W3.8 — the agents module: a home, and a guardian's lowered ceiling survives the boot

**Date:** 2026-10-22
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 step 8 — `modules/agents`
(roster, guardians, selfrepair, reaper, overseer ops; remove the
`kernel/controlplane`/`selfrepair` → `plugins/tools/overseertool` edge; fix
guardian reconcile resetting operator-lowered caps).

## What moved

| Before | After | What it is |
|---|---|---|
| `kernel/roster` (10 files) | `kernel/modules/agents/roster` | the durable agent identity: `Profile`, the store, the normalization, the validation, the autonomy helpers |
| `kernel/selfrepair` (19 files) | `kernel/modules/agents/selfrepair` | the auto-repair coordinator: the claim path, the dispatch, the routing rollback, the wake chain |

Both packages were already one module — the layer map named `kernel/roster` and
`kernel/selfrepair` as layer 3 in the module `agents` — but they sat at the top
of `kernel/`, outside the `kernel/modules/` tree W3 establishes as the target
home of the bounded contexts. The package names do not change, so the move is
an import-path change and nothing else: **161 importer files** repointed.

The placement is now *derived* (`kernel/modules/*/...` → module `agents`) and
the two hand-written layer rules are gone.

## The defect the roadmap named: guardian reconcile reset the operator's ceiling

**The bug.** A guardian is a system agent with a built-in spend ceiling
(`$5`/run, `$10`/day). The built-in fleet's reconcile runs on every boot and
assigned those built-in values **outright**:

```go
dst.MaxCostMc = maxCostMc
dst.MaxDailyMc = maxDailyMc
```

So an operator who tightened a guardian's budget got it silently raised back to
`$5`/`$10` on the next restart — the daemon undid the operator's own policy,
and the guardian-budget guardian's own contract ("pause or lower
`max_daily_mc`; never raise ceilings", `architecture/10-tools.md` §guardians)
was broken by the daemon itself. The bug was already on the books
(`architecture/07-autonomy-and-extensibility.md` §"Correctness / UX" item 7 and
the known-issues table in `architecture/00-README.md`).

**The fix.** The built-in caps are **ceilings**, and the reconcile only ever
tightens — the same rule the trust ceiling right below it already followed:

```go
if dst.MaxCostMc <= 0 || dst.MaxCostMc > maxCostMc {
    dst.MaxCostMc = maxCostMc
}
if dst.MaxDailyMc <= 0 || dst.MaxDailyMc > maxDailyMc {
    dst.MaxDailyMc = maxDailyMc
}
```

A cap above the built-in maximum is clamped down, an unset one is filled, and
a ceiling the operator lowered is **left alone**.

The package's own test already stated this contract — *"the reconcile only
tightens (clamps high / fills zero), it never raises an operator's tighter
limit"* — but only exercised the clamping half (a seeded cap of `100 * usd` is
clamped to `5 * usd`). The implementation did not match the comment, and the
other half was untested.

## The edge the roadmap named: the overseer tool was imported upward

`kernel/controlplane` and `kernel/selfrepair` both imported
`plugins/tools/overseertool` — a kernel package (L3/L5) reaching **up** into
a plugin (L6). The roadmap's ask was to remove that edge and let the overseer
tool be an L6 caller of the module's vocabulary.

The row they all spoke — the outcome of one agent repair — now lives in the
module (`roster.RepairResult`), and the overseer tool keeps its own name as an
**alias** of it, so every importer compiles unchanged:

```go
// plugins/tools/overseertool/overseer.go
type RepairResult = roster.RepairResult
```

The two kernel-side callers no longer import the tool at all:

- `kernel/controlplane` asks for a repair through the module's
  `roster.RepairSource`, injected by the daemon (`Server.SetRepairSource`).
  The paths that used it report "agent repair is not available on this daemon"
  when it is unwired, instead of dereferencing nil.
- `kernel/modules/agents/selfrepair` receives its repair source the same way
  (`WireAutoRepair(..., src, ...)`), so the module never names the tool that
  provides it.

The daemon (`cmd/agezt`) owns the wiring — it builds the source once and hands
it to both callers, exactly as it already does for the reasoning module.

**The ratchet paid.** Both `upward` edges are gone, so the archcheck ratchet
**drops from 128 to 126** import violations (13 call sites unchanged). The
allowlist was tightened accordingly, which is the only way the ratchet moves:
entries may only be deleted.

## The proof

Permanent tests:

- `plugins/builtinguardians/guardian_cap_test.go`
  - `TestSeedAll_KeepsAnOperatorsLoweredGuardianCap` — a guardian whose caps
    the operator lowered (`1 * usd` / `2 * usd`) keeps them across the boot.
  - `TestSeedAll_FillsAnUnsetGuardianCap` — a guardian with no cap at all
    still gets the built-in ceiling (the reconcile fills, it does not leave a
    guardian without a limit).
- `kernel/controlplane/roster_test.go` — `TestAgentRepair_AcceptsAndJournalsLifecycle`
  was **strengthened**: it now asserts that the repair reported the agent's
  own answer, not merely that it was accepted and journaled.

## Mutations — 8 mutations, 8 caught

| # | Mutation | Caught by |
|---|---|---|
| M01 | the reconcile raises the operator's lowered run cap again | `TestSeedAll_KeepsAnOperatorsLoweredGuardianCap` |
| M02 | the reconcile raises the operator's lowered daily cap again | `TestSeedAll_KeepsAnOperatorsLoweredGuardianCap` |
| M03 | an unset guardian cap is not filled | `TestSeedAll_FillsAnUnsetGuardianCap` |
| M04 | the roster stops filling an unset system guardian cap | the roster's own suite |
| M05 | the coordinator stops repairing the agent | the self-repair suite |
| M06 | the control plane stops forcing the routing chain | `TestAgentResolve*` |
| M07 | the operator repair reports an empty result | `TestAgentRepair_AcceptsAndJournalsLifecycle` (strengthened) |
| M08 | the repair source stops repairing | `TestAgentRepair_AcceptsAndJournalsLifecycle` (strengthened) |

**The run found a test gap and it is fixed and named.** M07 and M08 escaped
the first pass: the operator's repair test asserted only that the repair was
*accepted* and *journaled*, so a repair that did nothing at all — an empty
result, a source that never ran — still satisfied it. The test now asserts that
the repair reported the agent's answer (the mock provider's text), which is
what a repair that really ran produces. The mutations are caught on the repeat.

Two further notes on the run, recorded honestly: M06's first anchor did not
compile (`declared and not used: src`) and M07's first anchor was a build
failure as well — both were re-run with compiling anchors. A build failure is
not an escape and is not counted as one.

## The gates

All green: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (**273 packages, 126 import violations, 13 call sites** — the
ratchet *dropped*, one hand-written layer rule per moved package is gone, the
allowlist entries moved with their edges and two were deleted with the
edges), `tools/structure-md` (+ its `-check`), `tools/changelog-lint`,
`tools/docclaimscheck`, `staticcheck`, and the full `go test ./...`.

## What the move broke, and what it cost

- **The parity ledger.** `cmd/agt/compare_data.go` lists the files that back
  each OpenClaw/Hermes parity claim and **asserts that they exist**; one row
  pointed at `kernel/roster/roster.go` and the ledger's own test
  (`TestCmdCompareAudit_JSONHermesEvidence`) failed on the move. Fixed.
- **The repair source became a dependency to wire.** The control plane and the
  self-repair module previously *constructed* the repair source themselves
  (`overseertool.NewKernelSource(k, baseDir)`); now they receive it. The
  daemon builds it once. Three test call sites had to inject it — and one of
  them (the operator's repair test) had to be strengthened, which is how the
  gap above surfaced.
- **`SearchFilter` and `BulkResult` stayed in the plugin.** They are the
  overseer tool's own vocabulary (nothing outside the plugin names them), and
  moving them would have bought nothing.

## What is deliberately left

- **The reaper.** `kernel/runtime/reaper*.go` (the scan, the health report, the
  probation and the routing pressure) is the runtime's health loop: it reads
  `k.Journal()`, `k.Roster()`, `k.ArtifactIndex()`, `k.Schedules()`,
  `k.Standing()` and the agent-binding checks. Moving it into the module is
  the same ports rewrite W3.7 did for the workflow engine, and the repair
  brief is built from its report rows (`kernelruntime.ReaperReport`). It is the
  next slice, with the reaper's own vocabulary.
- **The module's `api` package.** The module is two packages (roster,
  selfrepair) and the host reaches the roster directly, which the ratchet
  already carries as debt. The package split — a `modules/agents/api`
  vocabulary package and a composition-root-injected service — belongs to the
  slice that stops the host from importing the module's internals at all.
- `modules/extensions` (mcp, plugin host, toolforge, toolbox, acpcatalog) is
  the next W3 step.
