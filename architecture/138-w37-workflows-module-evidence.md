# W3.7 — the workflows module: a home, and a trigger that says why it fired

**Date:** 2026-10-22
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 step 7 — `modules/workflows`
(move `runtime/workflowrun*` + draft + test-node; fix trigger `source` labels).

## What moved

The workflow engine was in two places that both sat outside the module tree W3
establishes as the target home of the bounded contexts:

| Before | After | What it is |
|---|---|---|
| `kernel/workflow` (11 files) | `kernel/modules/workflows/workflow` | schema, `Validate`, `Store`, templating, the template gallery, the cron/event trigger runner |
| `kernel/runtime/workflowrun.go`, `workflowrun_graph.go`, `workflowrun_helpers.go`, `workflowrun_node.go` | `kernel/modules/workflows/workflow/engine*.go` | the graph walk, the per-node dispatchers, the reliability wrapper, the journal snippets, the provenance map |
| `kernel/runtime/workflowdraft.go` | `kernel/modules/workflows/workflow/engine_draft.go` | the copilot (draft/refine), the JSON extraction, the auto-layout |
| `kernel/runtime/workflowtestnode.go` | `kernel/modules/workflows/workflow/engine_testnode.go` | the single-node test |

The package name does not change (`workflow`), so the move of the store and
the types is an import-path change and nothing else: **41 importer files**
repointed. The engine files are the second half of the move — they were
`runtime.Kernel` methods, so they could not simply be repointed.

**The module is one package.** The target architecture lists
`workflow`, `workflowexec`, `runtime/workflowrun*` and `workflowdraft` as four
homes; this slice realises them as one package (`kernel/modules/workflows/workflow`,
18 files). The reason is the archcheck ratchet: the runtime may not import a
module's internals, only its `api` package, so every package the runtime
reaches is one cross-module edge. Splitting the engine into `workflowexec`
and `workflowdraft` would have added two more edges (128 → 130) that the
ratchet cannot absorb — the ratchet only ever tightens. The module's own
package split (a `workflows/api` vocabulary package, `workflowexec` and
`workflowdraft` as separate packages) stays open for the module's rewrite
slice, when the runtime stops importing the module directly at all.

## The engine, rewritten against ports

The engine cannot live in the runtime and cannot reach into it: the module
must not import another module (archcheck's principle P5). So the engine now
holds **no provider, no tool registry, no bus and no sandbox of its own**.
Every capability it needs arrives through `workflow.Ports`:

| Port | The host's path |
|---|---|
| `Publish` | the host's bus |
| `Complete` | `Kernel.completeAux` — the Governor-routed, spend-attributed completion |
| `InvokeTool` | `Kernel.RunTool` — the governed tool call with the Edict gate |
| `RunCode` + `ScriptRunnerReady` | `Kernel.runCode` — the sandboxed script |
| `Approve` | the approval registry — the operator gate |
| `Model` / `DefaultModel` | `resolveRunModel` / the live default model |
| `HasProvider`, `Halted` | the host's state |

The host keeps its public names (`Kernel.RunWorkflow`, `Kernel.DraftWorkflow`,
`Kernel.TestWorkflowNode`, `runtime.RunWorkflowResult`,
`runtime.NodeTestResult`) as thin delegations in
`kernel/runtime/workflow_host.go`; every importer compiles unchanged.

The one thing the module could not keep is the **provenance**: the run's
"why" used to be read from the runtime's wake context, which the module
cannot see. It now travels as an explicit value — `workflow.Provenance` —
that the host fills from its own wake context (`workflowProvenance`) and the
module stamps on every `workflow.*` event it publishes. The wake context
itself stays where it was; only the reading of it moved.

## The defect the roadmap named: trigger `source` labels

**The bug.** A workflow fired by the cron or event trigger runner was
journalled as `source:"manual"`. The trigger runner knew exactly what fired
the workflow — it is the only place that does — but it told the host only a
human label (`"cron interval"`, `"event task.failed"`), and the daemon's fire
closure called `Kernel.RunWorkflow` without a wake context. The provenance
therefore read an empty source and fell back to `"manual"`: every automatic
run in the journal looked like a human had pressed the button, and the
"what fired this?" question had no answer in the audit trail. The manual
path (`source:"manual"`), the schedule path (`source:"schedule"`) and the
webhook path (`source:"webhook"`) were all correct — only the trigger path
was silent.

**The fix.** The trigger runner now hands the host *what fired the
workflow*:

```go
type Trigger struct {
    Source  string // "cron" or "event"
    Subject string // the matched event subject (event triggers only)
    Reason  string // the human label
}
```

`FireFunc` carries it, and the host's `Kernel.FireWorkflow` maps it into the
wake context the run already used:

```go
rctx = WithWakeContext(rctx, WakeContext{
    Source:         t.Source,
    Reason:         t.Reason,
    TriggerSubject: t.Subject,
})
```

A cron fire is now journalled as `source:"cron"` (runner `"cron"`), an event
fire as `source:"event"` with `trigger_subject` naming the matched subject.
The daemon's fire closure is now the kernel's method, so the wiring cannot
drift apart again:

```go
wfFire := k.FireWorkflow
```

A run that nobody triggered is still `source:"manual"` — the fix labels the
triggered runs, it does not relabel the hand-pressed ones.

## The proof

Two permanent test files, five cases:

- `kernel/modules/workflows/workflow/trigger_source_test.go`
  - `TestTriggerRunner_CronFireCarriesItsSource` — the cron fire carries
    `Source: "cron"`.
  - `TestTriggerRunner_EventFireCarriesItsSourceAndSubject` — the event fire
    carries `Source: "event"` and the matched subject.
- `kernel/runtime/workflow_trigger_source_test.go`
  - `TestFireWorkflow_CronTriggerIsJournalledAsCron` — the run's
    `workflow.started` event says `source:"cron"`, not `"manual"`.
  - `TestFireWorkflow_EventTriggerIsJournalledAsEvent` — the run's
    `workflow.started` event says `source:"event"` and names the subject.
  - `TestRunWorkflow_WithoutATriggerStaysManual` — the untriggered run still
    says `"manual"`.

**How the bug was established.** The defect was already on the books —
`architecture/07-autonomy-and-extensibility.md` §"Correctness / UX" item 9
and the known-issues table in `architecture/00-README.md` both named it. The
old code's own provenance function defaulted an empty wake source to
`"manual"`, and the daemon's fire closure set no wake context at all, so
every triggered run took that default. The last test above reproduces the
old outcome on purpose: a run with no trigger is journalled as `"manual"` —
the same value the cron and event fires used to get.

## Mutations — 10 mutations, 10 caught

| # | Mutation | Caught by |
|---|---|---|
| M01 | the event trigger loses its `Source` | `TestTriggerRunner_EventFireCarriesItsSourceAndSubject` |
| M02 | the cron trigger is labelled `"manual"` | `TestTriggerRunner_CronFireCarriesItsSource` |
| M03 | the event trigger loses the matched subject | `TestTriggerRunner_EventFireCarriesItsSourceAndSubject` |
| M04 | the host stops carrying the trigger source into the run | `TestFireWorkflow_*` |
| M05 | the provenance mapping drops the wake source | `TestFireWorkflow_*` |
| M06 | the module ignores the source it was given | `TestFireWorkflow_*` |
| M07 | a tool node drops the interpolated arguments | `TestRunWorkflow_*` |
| M08 | the per-node retries are ignored | the reliability suite |
| M09 | the event cooldown is ignored | `TestEventTrigger_FiresWithPayloadAndCooldown` |
| M10 | a saved workflow is not enabled by default | the store suite |

M01–M06 are the fix: they prove the source really travels from the trigger
runner through the host into the journal, and that no step of that chain can
be quietly dropped. M07–M10 are the moved packages: they prove the suites
still test the module after the move. The first run of M09 was a **build
failure** (the anchor `if false {` left two variables unused), not an
escape; it was re-run with a compiling anchor (`if now.Sub(last) < 0 {`)
and caught. No mutation escaped.

## The gates

All green: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (**273 packages, 128 import violations, 13 call sites** —
the ratchet is unchanged: one hand-written layer rule is gone and three
allowlist entries moved with their edges), `tools/structure-md` (+ its
`-check`), `tools/changelog-lint`, `tools/docclaimscheck`, `staticcheck`, and
the full `go test ./...`.

## What the move broke, and what it cost

Two live references to the old paths were found and fixed:

- `cmd/agt/compare_data.go` — the OpenClaw/Hermes parity ledger lists the
  files that back each claim and **asserts that they exist**. Three rows
  pointed at `kernel/workflow/workflow.go`; the ledger's own test
  (`TestCmdCompareAudit_JSONHermesEvidence`) failed on the move and the paths
  were repointed. This is the class of breakage the protocol warns about: a
  move that leaves a live reference behind breaks something that only a test
  notices.
- `kernel/agent/schema.go` — `agent.ValidateToolInput` was a forwarder to
  `kernel/platform/schema.ValidateToolInput` with exactly one caller: the
  workflow engine's pipeline schema check. A module may not import
  `kernel/agent` (another module), so the engine calls the platform package
  directly — and the forwarder was left with no caller at all. It was
  removed; `kernel/platform/schema` is the one home of that contract, and the
  schema test's side-by-side comparison was reduced accordingly.

The cost, stated plainly:

- **The engine moved, but the host's surface did not.** `Kernel.RunWorkflow`
  and friends are delegations now, not implementations. The public API, the
  wire, the events, the store file (`<home>/workflows/workflows.json`), the
  node types, the reliability semantics, the step cap, the snippet bound and
  the subflow depth cap are all unchanged.
- **The provenance is now an argument, not a context read.** A caller that
  builds a run's provenance by hand must fill `workflow.Provenance`; the
  host's `workflowProvenance` does it from the wake context, so every
  existing path (manual, schedule, standing, webhook, workboard) keeps its
  labels.
- **A run started through a tool is still `source:"manual"`.** The workflow
  tool (`plugins/tools/workflowtool`) calls `Kernel.RunWorkflow` without a
  wake context; the runner shows `"agent"` (the agent slug is in the
  context) but the source stays `"manual"`. That is the pre-existing
  behaviour and it was not changed here — labelling a tool-driven run
  "manual" is a separate question from labelling a *triggered* run.
- **The frontend comments that name `kernel/workflow`** (two prose
  references in the workflows UI) were left alone: the committed Web UI
  bundle must stay byte-identical to its sources for the
  `frontend-dist-in-sync` gate, and a stale prose comment is not worth a
  bundle rebuild.

## What is deliberately left

- The module's own package split (`workflows/api`, `workflowexec`,
  `workflowdraft`) — it belongs to the slice that stops the runtime from
  importing the module directly (the composition root injects the service,
  as `modules/reasoning` already does).
- The `modules/triggers` unification (one `Trigger{Source, Target, Policy}`
  for schedules, standing orders, workflow triggers and system tasks) — that
  is W4.1, and this slice's `Trigger` type is deliberately shaped like the
  one W4.1 will define.
- `modules/agents` (roster, guardians, selfrepair, reaper, overseer ops) is
  the next W3 step.
