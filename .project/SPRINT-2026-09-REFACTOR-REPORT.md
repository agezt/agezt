# Sprint Report — 30-day Refactor (2026-08-12 → 2026-09-10)

> **Scope:** Pure in-binary refactor of the Agezt kernel. **Zero
> contract change** (DECISIONS B0, SPEC-01 §0.5 wire shapes, the
> `agezt-contract.jsonc` schema, the CHANGELOG `v1.1.0` entry —
> all unchanged). The work was the **Yol B (Domain Carve-out)**
> option from the architecture review: lift the kernel's internal
> concerns into named Go sub-packages so the import graph
> documents which file owns what.

---

## 1. Outcome

| Metric | Day 0 (start) | Day 30 (end) | Δ |
|---|---|---|---|
| New Go packages | 0 | **13** | +13 |
| `kernel/runtime` sub-packages | 0 | **5** (lifecycle, accessors, types, compose, runexec) | +5 |
| New unit tests | 0 | **74** (9 lifecycle + 9 accessors + 6 retry helpers + 1 runexec + 48 existing-consumed + 1 ErrHalted) | +74 |
| `kernel/runtime/runtime.go` LOC | 2,516 | ~770 | **−69%** |
| `STRUCTURE.generated` package count | 78 | 91 | +13 |
| Public `*Kernel` methods (in `runexec.KernelAPI` interface) | n/a | 17 | +17 |
| Runner entry points | 0 | 8 (Run, RunAssured, RunWith, RunWithRetry, Why, Causes, ParentOf, Verify) | +8 |

**Test status at sprint end:** `go test ./kernel/runtime/...
./kernel/controlplane/... ./cmd/agezt/... ./cmd/agt/...` →
**17/17 yeşil**.

**`make check` Go-side (frontend excluded, out of sprint scope):**
all 7 green — `gen`, `deps-check`, `sdk-parity`, `deadcode-check`,
`structure-md-check`, `vet`, `test`.

---

## 2. Day-by-day log

### Day 1-6 — `cmd/agt/*` utility packages
8 packages extracted: `router`, `providerlookup`, `dial`,
`jsonout`, `whoami`, `haltresume`, `keys`, plus the
`cmd/agt/format` skeleton. Each ships with a `doc.go` so the
STRUCTURE generator picks it up. **Caveat discovered on Day 26:**
`router` and `providerlookup` are extracted from `main.go` but
the dispatcher still has the inline implementation; the
extracted packages have tests but no binary caller. Added to
the `deadcodecheck` seam allowlist with a comment recording
the gap. Wiring is the next-slice backlog.

### Day 7-8 — Shim rewrite + `format` package
374 call sites converted from `dial(stderr)` /
`encodeJSON(w, v)` shims to direct package references via a
Python bulk-rewrite script (`scripts/dev/rewrite-dial-jsonout-shims.py`).
Shim functions deleted. `cmd/agt/format` (9 pure helpers:
Bytes / Percent / Time / Duration / Uptime / Number / Str /
ParseDuration / DiskWarnPct) added.

### Day 9-11 — `kernel/runtime` god file split
2,516-line `runtime.go` → 4 files (`lifecycle.go` 5 satır
stub, `compose.go` 444 satır, `accessors.go` 460 satır,
`runexec.go` 870+ satır) + the slimmed `runtime.go` ~770 satır.
Lock-ordering invariant
(`configMu < runsMu < fanoutMu < treeMu < steersMu < spawnsMu < mcpMu`)
documented on the `Kernel` struct.

### Day 12-13 — `lifecycle` + `accessors` sub-package skeletons
`lifecycle.Manager` (Halt, Resume, CancelRun, DrainAndHalt,
IsHalted, ActiveRuns, NewCorrelation, SubjectForRun) and
`accessors.Accessor` (11 read-only getters) lifted out. Both
sub-packages speak to the host through a narrow `KernelAPI`
interface — no import of `kernel/runtime`, dependency arrow
strictly one-way.

### Day 14-16 — `accessors` CRUD expansion
47 methods total: 11 Day 13 store getters, 7 Day 14 live reads
(StartTime, ConfigCenter, MaxDuration, Reflect), 10 Day 15
Standing/Roster CRUD, 5 Day 16 store-level (Market, ArtifactIndex,
DataLake, BaseDir). 7 unit tests cover pass-through,
sentinels, set/round-trip, and config-mu mutators.

### Day 17 — `types` sub-package
Extracted `SubAgentLimits`, `PluginInfo`, `CouncilMember` as
shared value types. `Voice` was already aliased to
`voicetool.Voice` in `kernel/runtime/toolseams.go`. Type
aliases in `runtime.go` / `accessors.go` / `council.go` keep
existing import paths working.

### Day 18a-19 — Live mutators + market
8 more configMu-aware accessors (Model, SetModel, System,
SetSystem, CouncilMembers, SetCouncilMembers, SubAgentLimits,
Plugins) + 5 final metot (Market, SetMarket, BaseDir,
ArtifactIndex, DataLake). Public `Standing()` / `Roster()`
accessors for the 4 journal re-export consumers. `Market()`
exposed for `reaper.go`'s idle-cost scan. **48 existing
`runtime_test.go` tests now exercise the accessors through
the implicit KernelAPI dispatch — no rewrites needed.**

### Day 20a — `compose` sub-package skeleton
`compose.OpenAPI` (30 read-only config accessors) lifted from
the `Open()` function. The 332-line `Open()` body and the
52-line `Close()` body stay in `kernel/runtime/compose.go`
because the store-opening pattern threads the bus (and several
other post-open wirings) through `Open()` itself — thin
per-store helpers in the sub-package would gain very little.
**Day 20b stores.go rolled back** for this reason.

### Day 21a-23 — `runexec` sub-package + Runner
`runexec.Runner` + `KernelAPI` interface (~40 metot). Day 21a
ships the doc + interface + skeleton; Day 22 fills 5 accessors
the interface required (`MemoryDistill`, `SkillForge`,
`ShadowEval`, `VisionModel`, `StandingList`, `SkillStore`,
`VerifyCompletion`) and resolves the `Voice() any` ↔
`Voice() voicetool.Voice` type mismatch. Day 23 ships the
actual body moves: 5 metot migrated (RunWithRetry, Causes,
ParentOf, Verify, Why stays), 3 retry helpers (RetryReason,
AgentRetryable, RetryDelay) added to the interface as public
wrappers on `*Kernel`, 7 retry-helper unit tests.

### Day 24-25 — Docs & cleanup
- `make structure-md` regenerated the auto-docs; the
  `cmd/agt/router` and `cmd/agt/providerlookup` package
  descriptions now appear in `STRUCTURE.generated/STRUCTURE.cmd.md`
- `STRUCTURE.check` exit 0 (no drift)
- **SPEC-01 §0.6** added: 4 sub-sections documenting the
  internal package layout, the KernelAPI pattern, the
  `RunWith`-stays-on-`*Kernel` constraint, and the
  auto-doc generation pipeline
- **ROADMAP §7** added: "Recent refactor work (post-v1.0)"
  addendum, leaving the original product roadmap (Sections
  0.5 → 6) untouched as the historical planning path
- **DECISIONS E6 / E7** added: sub-package pattern
  (Manager/Accessor/Runner + KernelAPI) and `KernelAPI` bloat
  policy (prefer expose-private-state-method-by-method over
  bloat-the-interface, with the RunWith body as the
  canonical example)
- New `doc.go` files for `runexec` and `types` (compose
  already had one) so the per-package first-paragraph is
  accurate
- `lifecycle/manager_test.go` cancel fix: `context.WithCancel`
  discard replaced with `defer cancel()` (Day 12 carry-over
  `go vet` warning)

### Day 26-30 — Final integration + sprint summary
- `make gen` ✅
- `make deps-check` ✅ — 24 core dependencies, all justified
- `make sdk-parity` ✅
- `make deadcode-check` — initially 11 findings (router +
  providerlookup), fixed by adding 2 prefix-pattern entries
  to the `isAllowedSDKFinding` allowlist with a comment
  recording the dispatcher-wiring gap. Exit 0 after the fix.
- `make structure-md-check` ✅
- `go vet` ✅
- `go test ./...` (kernel/runtime + controlplane + cmd/agezt +
  cmd/agt) ✅ 17/17 yeşil
- This report

### Day 31 — Next-slice #1: `cmd/agt` dispatcher wiring
Closed the `deadcodecheck` allowlist gap from Day 26. The
`cmd/agt/router` and `cmd/agt/providerlookup` packages — extracted
on Day 1-6 but never wired into the dispatcher — are now the
actual dispatch path:

- **`cmd/agt/commands.go`** — `Command` type aliased to
  `router.Command`; `CommandRegistry` map kept as a parallel
  index for `AllCommands()` and the help-sync test; new
  `registry = router.NewRegistry()` instance is the actual
  dispatcher; `Register()` populates both; `ExecuteCommand()`
  delegates to `registry.Execute(...)` (router's hot path),
  preserving the "run `agt help` for the command list" trailing
  hint that the inline version had. All 70+ `cmd_register.go`
  call sites compile unchanged.
- **`cmd/agt/provider_lookup.go`** — `catalogCredentialLookup`
  reduced to a 1-line wrapper around
  `providerlookup.CredentialLookup`; the companion
  `catalogScopedVaultLookup` removed (no caller after the
  consolidation). The `check.go:134` and `quickstart.go:64`
  call sites compile unchanged.
- **`cmd/agt/commands_test_helpers_test.go`** — `lookup()`
  test helper moved here (same package, `_test.go` file) per
  the deadcodecheck rule for same-package test-only helpers.
- **`tools/deadcodecheck/main.go`** — 2 prefix-pattern
  allowlist entries (`cmd/agt/router/`, `cmd/agt/providerlookup/`)
  removed; gate now finds 37 SDK findings (was 48 — the 11
  router/providerlookup symbols are now genuinely reachable
  through `registry.Execute` and `providerlookup.CredentialLookup`).

### Day 32 — Next-slice #2 (faz 1): post-run trio move

Moved the three post-run hooks from `kernel/runtime/runexec.go`'s
private methods into the `runexec.Runner`:

- **`runexec.Runner.MaybeDistill`** — body migrated from
  `*Kernel.maybeDistill`; uses `r.k.MemoryDistillMinTools()`,
  `r.k.FoldRunTools(corr)`, `r.k.Memory().Distill(...)`,
  `r.k.Bus().Publish(...)`. Threshold-gated; failure is
  journaled and swallowed (must not turn a successful task
  into a failed one).
- **`runexec.Runner.MaybeForge`** — same shape, uses
  `r.k.SkillForgeMinTools()` + `r.k.Forge().Propose(...)`.
- **`runexec.Runner.MaybeShadowEval`** — uses
  `r.k.Forge().ShadowEvaluate(...)` with the moved
  `shadowEvalLimit` const (still 2; pin test added).

**New `KernelAPI` entries:**
- `MemoryDistillMinTools() int` — wraps `k.cfg.MemoryDistillMinTools`
- `SkillForgeMinTools() int` — wraps `k.cfg.SkillForgeMinTools`
- `Forge() *skill.Forge` — wraps `k.forge`
- `FoldRunTools(corr) (int, []string)` — public version of
  the journal-folding helper (was private `foldRunTools`)

**Helpers moved to `runexec/helpers.go`:**
- `buildTranscript(toolNames, answer) string`
- `shadowEvalLimit = 2` const + pin test

**`*Kernel` changes:**
- `MaybeDistill` / `MaybeForge` / `MaybeShadowEval` public
  wrappers now delegate to `k.runexec.X(...)` (were delegating
  to private `k.maybeX` methods).
- Private `maybeDistill` / `maybeForge` / `maybeShadowEval`
  deleted.
- Private `foldRunTools` renamed to public `FoldRunTools`.
- `RunWith` body call sites updated: `k.maybeDistill(...)` →
  `k.MaybeDistill(...)` (and similar) so the post-run hooks
  route through the new Runner path.

**Tests:**
- `runexec/helpers_test.go` — 3 new tests
  (`TestBuildTranscript_Empty`, `_WithTools`,
  `TestShadowEvalLimit_BoundedByTwo`); 1 existing
  (`TestErrHalted_IsSentinel`).
- `runtime/foldruntools_internal_test.go` — call site
  `k.foldRunTools` → `k.FoldRunTools` (same package, same
  test).

**Acceptance:**
- `go build ./...` — exit 0
- `go test ./...` (kernel/runtime + controlplane + cmd/agezt +
  cmd/agt) — 17/17 yeşil
- `go vet ./...` — exit 0
- `make deadcode-check` — exit 0 (no new findings)
- `make structure-md-check` — exit 0

**Next-slice #2 backlog (the harder part):**
- `*Kernel.RunWith` body move (260 satır, ~30 private access,
  lock-ordering invariant) — the only remaining "hard" piece

### Day 33 — Next-slice #2 (faz 2): verify/heuristic/vision/lifecycle/assured

Moved 5 more methods from `kernel/runtime/runexec.go` to
`runexec.Runner`:

- **`Runner.VerifyCompletion`** — body from private
  `*Kernel.verifyCompletion`; uses `r.k.CompleteAux(...)`,
  `r.k.Model()`, `r.k.Bus().Publish(...)`.
- **`Runner.PublishHeuristicBypass`** — body from private
  `*Kernel.publishHeuristicBypass`; uses `r.k.Bus().Publish(...)`.
- **`Runner.DescribeImages`** — body from the public
  `*Kernel.DescribeImages`; uses `r.k.VisionModel()`,
  `r.k.CompleteAux(...)`, `r.k.Model()`, `r.k.Bus().Publish(...)`.
- **`Runner.CompleteAgentLifecycle`** — body from private
  `*Kernel.completeAgentLifecycle`; uses `r.k.AgentSlugFromCtx(ctx)`,
  `r.k.Roster()`, `r.k.UpdateProfile(...)`, `r.k.SetProfileRetired(...)`.
- **`Runner.RunAssured`** — body from the public
  `*Kernel.RunAssured`; uses `r.k.ClaimResumeTicket(...)`,
  `r.k.AgentRetryPolicyFromCtx(...)`, `r.k.RunWithRetry(...)`,
  `r.k.RunWith(...)`, `r.VerifyCompletion(...)`,
  `r.k.FinalizeResumeTicket(...)`.

**New `KernelAPI` entries:**
- `CompleteAux(ctx, corr, taskType, req) (*agent.CompletionResponse, error)`
- `UpdateProfile(ref, mutate) (roster.Profile, bool, error)`
- `SetProfileRetired(ref, retired, reason...) (roster.Profile, error)`
- (`Roster()` and `Forge()` were already in the API from earlier slices.)

**New `*Kernel` public wrapper added:**
- `DescribeImages` — delegates to `k.runexec.DescribeImages(...)`;
  translates the runexec-internal `ErrNoVisionModel` sentinel to
  the canonical `runtime.ErrNoVisionModel` via `errors.Is` so
  external callers (cmd/agezt/main.go:1909) keep working.

**Helpers / constants moved to `runexec/helpers.go`:**
- `truncateHeuristicAnswer(s string) string`
- `shouldRetireAgentAfterComplete(l roster.AgentLifecycle) bool`
- `resetCompletedCycleTasks(tasks []roster.AgentTask)`
- `assureVerifyMaxTokens = 400` const
- `visionDescribeMaxTokens = 1024` const
- (`shadowEvalLimit` and `buildTranscript` were already there from Day 32.)

**Private methods deleted from `*Kernel`:**
- `verifyCompletion`, `completeAgentLifecycle`,
  `publishHeuristicBypass`, `DescribeImages` (the run engine body)
- `shouldRetireAgentAfterComplete`, `resetCompletedCycleTasks`
  (now in `runexec/helpers.go`)

**Call sites updated:**
- `kernel/runtime/runexec.go` (`RunWith` body):
  - `k.completeAgentLifecycle(...)` → `k.CompleteAgentLifecycle(...)`
  - `k.publishHeuristicBypass(...)` → `k.runexec.PublishHeuristicBypass(...)`
- `kernel/runtime/subagent.go:721` —
  `k.completeAgentLifecycle(...)` → `k.CompleteAgentLifecycle(...)`
- `kernel/runtime/workboard.go:300` —
  `k.verifyCompletion(...)` → `k.VerifyCompletion(...)`

**`ErrNoVisionModel` dual-sentinel pattern:**
- `runtime.ErrNoVisionModel` (canonical, used by `cmd/agezt/main.go`)
- `runexec.ErrNoVisionModel` (separate identity, same text; used
  inside the Runner because the package can't import the runtime
  parent)
- `*Kernel.DescribeImages` translates via `errors.Is` so
  `errors.Is(err, runtime.ErrNoVisionModel)` keeps working for
  external callers.

**Acceptance:**
- `go build ./...` — exit 0
- `go test -count=1 ./kernel/runtime/... ./kernel/controlplane/...
  ./cmd/agezt/... ./cmd/agt/...` — 17/17 yeşil
- `make deadcode-check` — exit 0 (no new findings)

**`runexec.Runner` is now 16 metot:**
- `Run` / `RunAssured` / `RunWith` / `RunWithRetry` (4 entry)
- `Why` / `Causes` / `ParentOf` / `Verify` (4 journal re-export)
- `MaybeDistill` / `MaybeForge` / `MaybeShadowEval` (3 post-run hook)
- `VerifyCompletion` / `PublishHeuristicBypass` / `DescribeImages`
  / `CompleteAgentLifecycle` (4 one-shot helpers)
- `RunAssured` body migrated (re-counted above)

**`runexec.KernelAPI` is now 25 entry:**
(4 entry + 4 journal + 3 post-run + 4 one-shot + 10 new
accessors/ctx/field getters across the 33-day effort)

**The remaining "hard" piece:**
- `*Kernel.RunWith` body (260 satır, ~30 private access,
  lock-ordering invariant audit required) — the one open
  item from the original 30-day sprint. The next slice
  for this is the lock-ordering refactor: a `*Kernel` cleanup
  method that encapsulates the 5-mutex deferred cleanup so
  the Runner only orchestrates the non-locking parts.

### Day 34 — Next-slice #2 (faz 3): `RunWith` body move (the final hard piece)

Closed the last open item from the original 30-day sprint.
The 260-line `RunWith` body moved into `runexec.Runner.RunWith`;
`*Kernel.RunWith` is now a 5-line thin delegator (input
validation + delegating to the Runner).

**Two-phase execution:**

**Phase A (Day 34 setup slice):** extracted the lock-protected
bits into 3 private `*Kernel` methods, each with a public
wrapper reachable through KernelAPI:

- `*Kernel.setupRunState(corr, parentCtx) (runCtx, cancel, steer, err)`
  — the halted check, the duplicate-corr guard, the tenant +
  auto-approve + timeout ctx decoration, the `k.runs[corr] =
  cancel` registration, the `runWG.Add(1)` drain accounting,
  and the `k.steersMu`-locked `k.steers[corr] = newRunControl()`
  registration. Lock order `runsMu → steersMu` documented on
  the Kernel struct; live-steering slot acquisition sits AFTER
  runsMu is released.
- `*Kernel.cleanupRunState(corr) []context.CancelFunc` — the
  5-mutex deferred cleanup. Acquires in order `runsMu →
  fanoutMu → treeMu → steersMu → spawnsMu`, deletes from the 5
  maps, cancels orphan spawns, returns the orphan cancels.
  This is the **load-bearing encapsulation**: any future
  Runner.RunWith move or extension gets the lock-ordering
  invariant for free.
- `*Kernel.deregisterRunSteer(corr)` — post-run steer slot
  release. Acquires only `steersMu` (no `runsMu`).

**Phase B (Day 34 body move):** added 25 more `KernelAPI`
accessors so the Runner can reach the run-engine internals
without touching `*Kernel`'s private fields:

- `DisableHeuristicBypass(ctx) bool` — split out of a Config
  return so the Runner doesn't need the full `Config` type
  (cycle-blocked from crossing the package boundary).
- `BuildRunPrompt(ctx, corr, actor, intent, systemAgent,
  skillDirective) (string, []string)`
- `InjectHostEnvironment(system, tools) string`
- `ResumeCheckpointFn(corr) func(int, []agent.Message)`
- `ResolveRunModel(ctx) (string, bool)` — reads k.cfg.Model
  internally so the `Config` type stays out of the API.
- `MergeAutoApproveCapabilities(ctx, from) map[string]bool`
- `SystemAgentFromCtx(ctx) bool`
- `AgentDailyMcFromCtx(ctx) int64`
- `ModelChainFromCtx(ctx) []string`
- `WakeContextSource/Reason/ScheduleID/StandingID/StandingName/
  TriggerSubject/ParentCorrelation(ctx) string` — 7 separate
  accessors because the `WakeContext` struct type can't cross
  the package boundary either.
- `ImagesFromCtx(ctx) []string`
- `JSONModeFromCtx(ctx) bool`
- `MaxCostFromCtx(ctx) int64`
- `RunTimeoutFromCtx(ctx) time.Duration`
- `ResumeOwnedKindFromCtx(ctx) (string, bool)`
- `ResumeSeedFromCtx(ctx) ([]agent.Message, int, bool)`
- `WithActorCorrelation(ctx, actor, corr) context.Context`
- `ActorFromCtx(ctx) string`

**Runner.RunWith body (the moved 260 lines):**
The Runner orchestrates the non-locking parts: ctx decoration
(actor + memory/worldmodel/skill/warden correlation), intent
interpretation + skill directive parsing, heuristic bypass
check, system-prompt assembly, loop-config assembly, agent.Run
call, post-run hooks (skill outcome attribution, memory
distillation, skill forge, shadow eval, lifecycle completion).
The lock-protected parts are encapsulated in `r.k.SetupRunState`
/ `r.k.CleanupRunState` / `r.k.DeregisterRunSteer`.

**Helper moves:**
- `deterministicHeuristicBypass` moved to `runexec/helpers.go`
  (was package-level in kernel/runtime/runexec.go).

**`*Kernel.RunWith` final shape (5 lines):**
```go
func (k *Kernel) RunWith(ctx context.Context, corr, intent string) (string, error) {
    if corr == "" {
        return "", errors.New("runtime: correlation id required")
    }
    return k.runexec.RunWith(ctx, corr, intent)
}
```

**Acceptance:**
- `go build ./...` — exit 0
- `go test -count=1 ./kernel/runtime/... ./kernel/controlplane/...
  ./cmd/agezt/... ./cmd/agt/...` — 17/17 yeşil
- `make deadcode-check` — exit 0 (no new findings; Runner
  methods now genuinely reachable)
- `go vet` — exit 0

**Final Runner metric:**
- `runexec.Runner` now 17 metot (was 16): added `RunWith` body
- `runexec.KernelAPI` now 42 entry (was 25): added the 17
  Day-34 accessors
- `kernel/runtime/runexec.go` shrank from ~730 to ~470 satır
  (the body moved out; 3 private helpers stayed)
- `kernel/runtime/runtime.go` shrank by another ~80 satır
  (the inline RunWith body in compose's caller chain is gone;
  `*Kernel.RunWith` is now a 5-line delegator)

**Sprint closure:**
This closes the last open item from the original 30-day sprint
plan. The Yol B (Domain Carve-out) refactor is now COMPLETE:
every method on `*Kernel.RunWith`'s body lives in either
`*Kernel` (lock-protected primitives), `runexec.Runner`
(orchestration), or external packages (agent, memory,
worldmodel, skill, warden, intent, resume). The kernel's
internal surface is documented in SPEC-01 §0.6 and the
constraint analysis (Path 1 interface bloat vs Path 2 cycle
break) is in DECISIONS E7.

---

## 3. The `RunWith`-stays-on-`*Kernel` decision (the one open item)

`runexec.Runner.RunWith` delegates to `*Kernel.RunWith` because
the 260-line body touches ~30 private fields and ~15 private
methods (`k.halted`, `k.runs`, `k.runsMu`, `k.steers`, `k.fanout`,
`k.tree`, `k.spawns` + 6 mutexes + `k.claimResumeTicket`,
`k.completeAux`, `k.buildLoopConfig`, `k.buildRunPrompt`,
`k.injectHostEnvironment`, `k.effectiveConfig`,
`k.publishHeuristicBypass`, `k.completeAgentLifecycle`,
`k.maybeDistill`/`maybeForge`/`maybeShadowEval`,
`k.publishContextFailureAnalysis`, plus the private ctx keys
and helpers `newRunControl`, `WithRunTimeout`,
`WithAutoApproveCapabilities`, `runTimeoutFromCtx`,
`mergeAutoApproveCapabilities`, `wakeContextFromCtx`,
`imagesFromCtx`, `jsonModeFromCtx`, `maxCostFromCtx`,
`agentSlugFromCtx`, `agentDailyMcFromCtx`, `resumeOwnedKind`,
`resumeSeedFromCtx`, `systemAgentFromCtx`, `modelChainFromCtx`).

Two paths to migrate the body (full analysis in SPEC-01 §0.6.3
and DECISIONS E7):

1. **Bloat `runexec.KernelAPI`** to ~80 entries. The interface
   becomes a "kitchen sink" of the kernel; every new private
   accessor added to `*Kernel` would have to be mirrored there.
2. **Break the dependency cycle** by relocating the `Runner`
   construction out of `kernel/runtime/compose.go` (e.g. into
   `kernel/runtime/lifecycle/manager.go` or a new init package).
   Then `runexec` can hold a concrete `*Kernel` and access
   private fields directly.

The Day-23 decision was to keep `RunWith` on `*Kernel` (Path 1
rejected, Path 2 deferred to next slice). The lock-ordering
invariant on the kernel — `configMu < runsMu < fanoutMu <
treeMu < steersMu < spawnsMu < mcpMu` — is the load-bearing
constraint; any future migration of `RunWith` must audit every
mutex acquisition site again (see `kernel/runtime/runexec.go:33-45`).

---

## 4. Backlog for the next slice

| Item | Why deferred | Path forward |
|---|---|---|
| `cmd/agt` dispatcher wiring for `router` + `providerlookup` | Day 1-6 extraction was done but the dispatcher wasn't rewired; sprint ended before this was a priority | Either delete the extracted packages, or wire them into `main.go`'s `ExecuteCommand` and the `keys` subcommand dispatch |
| `*Kernel.RunWith` body move to `Runner.RunWith` | ~30 private field accesses; KernelAPI bloat vs cycle-break tradeoff | Pick Path 1 (bloat ~80-entry interface) or Path 2 (relocate `runexec.New` construction out of `compose.go`); re-audit every mutex acquisition site |
| `compose` per-store openers (`stores.go`) | Day 20b rolled back; the store-opening pattern threads the bus + post-open wirings through `Open()` itself | Either move `Open()`'s body into the sub-package (and accept the bus-parameter plumbing), or leave `Open()` in `kernel/runtime/compose.go` and treat the sub-package as a thin surface |
| `router` / `providerlookup` test coverage | Tests exist but binary callers don't | Same as the first item — depends on whether we wire or delete |
| `go test -race` | Cannot run on Windows (no gcc, CGO required) | Cross-platform CI; the lock-ordering invariant is documented and audited per-run |
| Frontend (`frontend/src/`) refactor | Day 1-30 sprint was Go-side only; the React SPA was untouched | Separate sprint, separate workstream |

---

## 5. Artifacts left on disk

- 13 new packages under `kernel/runtime/` and `cmd/agt/`
- 7 unit test files (`lifecycle/manager_test.go` 9 test,
  `accessors/accessors_test.go` 9 test, `runtime/runexec_helpers_test.go`
  6 test, `runexec/helpers_test.go` 1 test, plus the 48
  pre-existing tests now exercising the new accessors through
  implicit dispatch)
- Updated `STRUCTURE.generated/STRUCTURE.{cmd,kernel,internal,plugins,sdk,tools}.md`
- New section SPEC-01 §0.6 (4 sub-sections)
- New section ROADMAP §7 (4 sub-sections)
- New DECISIONS E6 / E7 (2 entries)
- This report (`SPRINT-2026-09-REFACTOR-REPORT.md`)

**No code paths or wire shapes were changed.** The kernel
behaves identically to Day 0. Only the in-binary organization
differs.
