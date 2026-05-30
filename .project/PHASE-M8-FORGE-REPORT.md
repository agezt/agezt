# Phase Report — Milestone M8 (Forge v1 — skill lifecycle)

> Status: **shipped** · Date: 2026-05-30
> Phase 2 (ROADMAP product-M2 "Memory & self-improvement"), slice 2 of 3.
> The "Curator-killer": the agent learns reusable, named **skills** from
> what it does, and those skills are governed through a **journaled,
> reversible state machine** (SPEC-05 §4–5) instead of straight-to-active
> markdown. This is the half of Phase 2 that makes the system *improve*,
> not just *remember*.

## Scope

World Model (M7) gave the system a graph to resolve references. Forge
closes the learning loop: after a complex task the agent **proposes** a
skill; the operator **promotes** it through `draft → shadow → active`;
once active it's **injected** into matching future tasks; every
transition is journaled and **`agt skill revert`** appends a reversal.

**Promotion model (chosen): operator-gated.** Forge proposes a `draft`
only; advancing it is an explicit `agt skill promote`. This is the SPEC
trust-ladder rule "bad skills never reach production silently" (§5.3).
**True shadow-execution comparison (SPEC open-Q3 — compare a shadow
skill's hypothetical actions to reality) is deferred**: `shadow` is a
real lifecycle state and a manual gate, but auto-promotion-by-fidelity
is not built in v1.

Both seams already existed (proven by memory/world) and were reused:
- **post-run trigger** = `maybeDistill`/`foldRunTools`/`buildTranscript`
  → `maybeForge` (propose a skill after a multi-tool run);
- **context injection** = `injectMemory`/`injectWorld` → `injectSkills`
  (prepend active skills' bodies, journal `skill.activated`).

## What shipped

### New `kernel/skill`
Mirrors the memory/world two-layer split:
- **Store** (`skill.go`) — pure, file-backed. `Skill` (content-addressed
  `BLAKE3("skill"\0name\0body)`, description/triggers/body/tools_required,
  semver `Version`, `Lineage`, `Status`, `Metrics`). The **lifecycle
  state machine**: a `legalTransitions` table + `CanTransition` +
  `PromoteTarget` (draft→shadow→active, quarantined→active). Single
  `skills.json`, atomic write + deterministic sort. `Count()` =
  active-only.
- **Retrieve** (`retrieve.go`) — pure ranking over **active-only** skills
  by name+description+triggers keyword overlap × recency (mirrors
  `memory.Search`/`worldmodel.Resolve`; the body is deliberately not
  matched — description/triggers are the curated activation surface).
- **Forge** (`forge.go`) — the journaling boundary. `Create` (content-
  address dedupe; a new body for an existing name is a new **version**
  with auto-`Lineage`), `Promote`/`Quarantine`/`Revert` (guarded
  transitions, each journaled; `Revert` archives + re-activates the
  lineage parent — non-destructive), `Activate` (rank + journal
  `skill.activated` + bump use metrics), `RecordOutcome`, and `Propose`
  (best-effort LLM extraction → **draft**, mirroring `memory.Distill`).

### Event kinds (append-only)
`skill.created`, `skill.promoted`, `skill.quarantined`, `skill.reverted`,
`skill.activated` — added to both the const block and `knownKinds`.

### Runtime wiring (`kernel/runtime/runtime.go`)
The skill store is a **first-class kernel resident**, opened at
`BaseDir/skills` like memory/world. Config knobs `SkillInject`/
`SkillTopK`/`SkillForge`/`SkillForgeMinTools` (all default OFF);
`Forge()` accessor; `injectSkills` prepends active matching skills next
to `injectWorld` (journaling `skill.activated`); `maybeForge` proposes a
draft after a ≥N-tool run next to `maybeDistill` (best-effort, never
fails the task).

### Control plane + CLI
`kernel/controlplane/skill.go` handlers (`CmdSkill{List,Get,History,
Promote,Quarantine,Revert}`); `History` folds the journal by skill id
(like `runs`/`inbox`). `cmd/agt/skill.go` — `agt skill list|show|history|
promote|quarantine|revert [--json]`. `agt status` reports
`active_skills`.

### Daemon wiring (`cmd/agezt/main.go`)
`skillOn`/`forgeOn` env toggles, `cfg.Skill*` set alongside the world
knobs, the `knowledge` banner line extended with `skills/forge (N
active)`.

| Env var | Meaning |
|---|---|
| `AGEZT_SKILLS` | `off` disables per-run skill injection (store/CLI stay live) |
| `AGEZT_FORGE` | `off` disables post-run skill proposal |

## Design rules followed

- **No new external dependency** — stdlib + existing BLAKE3; `go.mod`
  unchanged (POLICY).
- **Third copy of a proven pattern** — Store/Forge split, atomic write,
  content-addressing, journaling boundary, config-knob gating, the
  post-run trigger and the injection seam are all lifts from
  memory/world. The lifecycle state machine is the only genuinely new
  code.
- **Operator-gated promotion** — Forge can only author drafts; nothing
  reaches the active pool without an explicit `agt skill promote`.
- **Reversible & auditable** — content-addressed versions, lineage,
  journaled transitions; `revert` appends a reversal and restores the
  prior version rather than editing history; `agt skill history` and
  `agt why` explain every change.

## Test coverage

~30 new tests; `go test ./...` green on host (windows) + `GOOS=linux`
cross-compile; `go vet` clean. Package count 44 → 45 (added
`kernel/skill`).

- `kernel/skill`: content-address version-on-body-change; legal vs
  illegal transitions (`draft→active`, `archived→active` rejected);
  promote-chain; quarantine→reactivate; **revert archives + restores the
  lineage parent**; retrieve ranks active-only; propose parses a mock
  JSON skill into a draft + journals; propose-decline yields nothing;
  metrics; nil-bus store-only.
- `kernel/runtime`: active skill body injected into the system prompt +
  `skill.activated` provenance; draft **not** injected; Forge proposes a
  draft after a multi-tool run; both off by default.
- `kernel/controlplane` (real TCP pair): full create→promote→promote→
  history lifecycle over the wire; illegal-promote errors; empty list.
- `cmd/agt`: skill help / arg-validation exit codes.

### Manual end-to-end (mock provider)
The daemon banner reports `skills on/forge on (N active)`; `agt skill
list` and `agt status` (`active_skills`) reflect the live store. Note
the **offline demo mock** can't exercise auto-proposal live (it does one
tool call — below the 4-tool Forge threshold — and returns non-JSON, so
`Propose` declines); the proposal→draft path is instead proven by
`TestForgeProposesAfterMultiToolRun` (scripted mock) and the full
governed lifecycle (create → promote×2 → active → inject → history →
revert → restore-parent) by the `kernel/skill` and control-plane
integration tests over a real TCP pair — the identical code path an
operator drives with a real provider.

## Deferred (named for later)

- **True shadow-execution comparison** (SPEC open-Q3) — `shadow` is a
  state + manual gate in v1; auto-promotion by hypothetical-action
  fidelity is future work.
- **Auto-quarantine on regression** — the metrics + `Quarantine` hook
  exist; the automatic trigger (Pulse-surfaced) is deferred.
- **Periodic consolidation/merge pass** (§5.4), **sub-DAG skill bodies**
  & rich trigger conditions, **semver auto-bump** policy, marketplace/
  import (`agt migrate openclaw|hermes`).

## Closes / next

Forge gives Agezt a governed, observable, reversible self-improvement
pipeline — the Curator-killer SPEC-05 §5 promised, standing on the world
model M7 added beneath skill retrieval. The five in-process residents
(agent loop, memory, world model, pulse, channel) plus Forge and the
operator CLI are the base for the final Phase-2 slice: **Reflection v1**
(judgment recalibration + world-model decay), which closes the learning
loop by improving *judgment*, not just *skills* (§6).
