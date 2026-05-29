# Phase Report — Milestone 1.d (HITL Approval Routing v1)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-06 §3.4 (Approvals)](SPEC-06-SECURITY.md),
> [DECISIONS F3 (trust ladder)](DECISIONS.md), and the long-deferred
> "Live HITL approval routing" open item from M1.a/b/c.
> Continues [PHASE-M1.c-REPORT.md](PHASE-M1.c-REPORT.md).

## Scope

M1.a shipped Edict's trust ladder with two folding modes (AskAllow,
AskDeny) and journaled every would-have-been-prompt as a `WouldAsk`
flag — honest about the gating posture but operationally a no-op.
M1.d **closes the loop**: a third folding mode (AskPrompt) actually
pauses the tool-loop and routes a real request to the operator via
the control plane.

What this unlocks now (M1.d), and what it sets up for later phases:

| Need | M1.d status |
|---|---|
| Operator says no to a dangerous command before it runs | ✅ `agt deny <id>` |
| Operator says yes to an L1/L2/L3 capability after inspecting it | ✅ `agt approve <id>` |
| Timeout auto-denial when no operator answers | ✅ default 5 min, configurable |
| Audit trail of every requested→resolved approval | ✅ 4 new event kinds, BLAKE3 chained |
| Channel-routed prompts (Telegram, IDE, web) | ⏸ M1.e — reuses the same Resolve API |
| Plan-approval gate-nodes (DECISIONS C3, SPEC-06 §3.4) | ⏸ M1.e DAG — reuses the same Submit API |

## What shipped

### New package: `kernel/approval` (578 LoC, 8 tests)

| File | LoC | Role |
|---|---:|---|
| `approval.go` | 320 | `Registry`, `Request`, `Outcome`, `Submit`/`Resolve`/`Pending`, timeout/cancel paths, event publishers |
| `approval_test.go` | 258 | 8 tests covering grant, deny, timeout, ctx-cancel, unknown-id, sort order, payload shape |

Public surface:

```go
type Decision string // "grant" | "deny" | "timeout" | "cancel"

type Registry struct{ /* ... */ }
func New(Config) *Registry

// Blocks until Resolve, timeout, or ctx cancel. Always returns an
// Outcome; never errors. All four paths journaled.
func (*Registry) Submit(ctx, SubmitSpec) Outcome

// Out-of-band: any channel (agt, Telegram, IDE) calls this.
// Accepts only Grant or Deny — Timeout/Cancel are system-internal.
func (*Registry) Resolve(id string, d Decision, reason, by string) error

func (*Registry) Pending() []Request
func (*Registry) PendingCount() int
```

The Submit→Resolve handoff uses a per-entry buffered channel of size 1
so Resolve never blocks even if the waiter has already exited via
timeout or ctx-cancel. Every Submit cleanly detaches its entry on exit,
so a Resolve race against a timeout returns `ErrUnknownApproval`
rather than leaking a goroutine.

### Edict adds `AskPrompt` mode (kernel/edict)

```go
type AskPolicy int
const (
    AskAllow  AskPolicy = iota   // M1.a default
    AskDeny                      // M1.a strict mode
    AskPrompt                    // M1.d live HITL — NEW
)

type Outcome struct {
    /* ... */
    WouldAsk         bool
    RequiresApproval bool   // NEW in M1.d
}
```

Under AskPrompt, Ask-class capabilities (L1..L3) return
`Decision=Deny + RequiresApproval=true + WouldAsk=true`. The Deny is a
**fail-closed default** for any caller that ignores RequiresApproval —
only the runtime's policyHook knows to actually pause and submit.
Hard-deny rules still fire first (verified by
`TestDecide_AskPromptDoesNotBypassHardDeny`).

### Runtime policyHook integration

```go
// kernel/runtime/runtime.go
func (k *Kernel) policyHook(ctx context.Context, tc agent.ToolCall) agent.PolicyVerdict {
    cap := edict.CapabilityForToolCall(tc.Name, tc.Input)
    out := k.edict.Decide(cap, string(tc.Input))

    verdict := /* ... build from out ... */
    if !out.RequiresApproval {
        return verdict
    }
    // Live HITL: pause, route, block.
    res := k.approvals.Submit(ctx, approval.SubmitSpec{
        Capability:    string(out.Capability),
        ToolName:      tc.Name,
        Input:         string(tc.Input),
        Reason:        out.Reason,
        Actor:         actorFromCtx(ctx),
        CorrelationID: correlationFromCtx(ctx),
    })
    switch res.Decision {
    case approval.DecisionGrant:
        verdict.Allow = true
        verdict.Reason = "approval granted by " + res.ResolvedBy
    default:
        verdict.Allow = false
        verdict.Reason = fmt.Sprintf("approval %s: %s", res.Decision, res.Reason)
    }
    return verdict
}
```

The actor + correlation ID flow into the approval via ctx values
(stashed by `RunWith` before calling `agent.Run`) so every approval
event carries the same correlation as the originating task —
`agt why <event_id>` continues to walk the full chain.

When the operator denies, the existing agent-loop deny path takes
over: no `tool.invoked`, a synthetic `tool.result` with the deny
reason gets fed back to the model, and the run continues so the model
can react gracefully (rather than crashing with an unhandled error).

### New event kinds (`kernel/event/kinds.go`)

| Kind | Subject | Payload |
|---|---|---|
| `approval.requested` | `approval.request` | `{approval_id, capability, tool_name, input, reason, timeout_unix, created_unix}` |
| `approval.granted` | `approval.resolve` | `{approval_id, decision, reason, resolved_by}` |
| `approval.denied` | `approval.resolve` | `{approval_id, decision, reason, resolved_by}` |
| `approval.timeout` | `approval.resolve` | `{approval_id, decision, reason, resolved_by}` |

Timeout and the synthetic ctx-cancel reuse `approval.denied`'s kind
with `decision="cancel"` so consumers only have to know about three
terminal kinds (granted/denied/timeout). A grant-vs-deny on the same
ID would never race: Resolve atomically removes the entry, so a second
attempt cleanly returns `ErrUnknownApproval`.

### Control plane + `agt` CLI

Two new commands on the wire:

```
CmdApprovals = "approvals"   // → {pending: [...], count: N}
CmdDecide    = "decide"      // {id, decision="grant|deny", reason}
```

Three new `agt` subcommands:

```
agt approvals              # list pending; prints id, capability, tool, reason, input, timeout
agt approve <id> [reason]  # grant
agt deny    <id> [reason]  # deny
```

The pending list is sorted by `CreatedAt` ascending so the operator
sees the oldest waiter first.

### Daemon env wiring (`cmd/agezt`)

```
AGEZT_APPROVAL_MODE = allow | deny | prompt
                       (default allow; M1.a/b/c behaviour unchanged)
```

Banner now reflects the active mode:

```
policy engine : edict (defaults from DECISIONS F3; AskPrompt (live HITL via `agt approve|deny`))
```

The default is unchanged (`AskAllow`) so existing demos keep working.
`prompt` mode opts into the new blocking behaviour.

## Demo transcript (real binaries, live deny)

```
$ AGEZT_HOME=/tmp/agezt-m1d-demo AGEZT_PROVIDER=mock \
  AGEZT_APPROVAL_MODE=prompt ./bin/agezt

Agezt 0.0.0-m0 — daemon ready (protocol v1)
  base dir         : /tmp/agezt-m1d-demo
  governor         : primary=mock(offline; scripted shell+final), daily_ceiling=$20.00
  tools            : shell(warden=requested-namespace), file(...), http(hosts=0)
  policy engine    : edict (defaults from DECISIONS F3; AskPrompt (live HITL via `agt approve|deny`))
  warden           : requested=namespace, effective=none (M1.c facade; downgrades journaled)
  control plane    : 127.0.0.1:55514

# In shell A — the agent run blocks waiting for approval:
$ ./bin/agt run "list the files here and tell me what this project is"
  [evt seq=0 kind=task.received]
  [evt seq=1 kind=llm.request]
  [evt seq=4 kind=llm.response]
  ▶ (blocking — agent.shell is L2; waiting for operator)

# In shell B — operator inspects:
$ ./bin/agt approvals
1 pending approval(s):

  id         : appr-01KSS6AMM37JVJ516WS5XYX8JB
  capability : shell
  tool       : shell
  reason     : level L2; AskPolicy=AskPrompt → operator approval required
  actor      : agent-run-01KSS6AMM0769WTW1CVK1SXX4E
  input      : {"command":"dir"}
  timeout    : unix 1780036003

Resolve with: agt approve <id> [reason]  |  agt deny <id> [reason]

# Operator denies:
$ ./bin/agt deny appr-01KSS6AMM37JVJ516WS5XYX8JB "looks suspicious"
{ "decision": "deny", "id": "appr-01KSS6AMM37JVJ516WS5XYX8JB", "ok": true }
```

Daemon log for that run (all events on the bus):

```
[evt seq=0  kind=task.received          subject=agent.…task]
[evt seq=1  kind=llm.request            subject=agent.…llm]
[evt seq=2  kind=routing.decision       subject=governor.route]
[evt seq=3  kind=budget.consumed        subject=governor.budget]
[evt seq=4  kind=llm.response           subject=agent.…llm]
[evt seq=5  kind=approval.requested     subject=approval.request]    ← NEW (M1.d)
[evt seq=6  kind=approval.denied        subject=approval.resolve]    ← NEW (M1.d)
[evt seq=7  kind=policy.decision        subject=agent.…policy]
[evt seq=8  kind=tool.result            subject=agent.…tool]         ← no tool.invoked between!
[evt seq=9  kind=llm.request            subject=agent.…llm]
[evt seq=10 kind=routing.decision       subject=governor.route]
[evt seq=11 kind=budget.consumed        subject=governor.budget]
[evt seq=12 kind=llm.response           subject=agent.…llm]
[evt seq=13 kind=task.completed         subject=agent.…task]
```

**Grant** path is symmetric — same event sequence but with
`approval.granted` instead of `approval.denied` at seq 6, then a
normal `tool.invoked` → `warden.profile_downgraded` →
`warden.executed` → `tool.result` chain follows seq 7:

```
[evt seq=5  kind=approval.requested]
[evt seq=6  kind=approval.granted]                                   ← grant
[evt seq=7  kind=policy.decision]
[evt seq=8  kind=tool.invoked]                                       ← runs!
[evt seq=9  kind=warden.profile_downgraded]
[evt seq=10 kind=warden.executed]
[evt seq=11 kind=tool.result]
[evt seq=12 kind=llm.request]
…
[evt seq=16 kind=task.completed]
```

```
$ ./bin/agt journal verify
{ "ok": true }
```

BLAKE3 chain intact across both flows (deny: 14 events, grant: 17 events).

## Verified invariants

| Invariant | Test |
|---|---|
| Grant unblocks Submit with `DecisionGrant`; emits requested+granted | `TestSubmit_GrantedUnblocksWithDecision` |
| Deny unblocks Submit with `DecisionDeny`; emits requested+denied | `TestSubmit_DeniedUnblocksWithDecision` |
| Timeout auto-denies after the configured duration; emits requested+timeout | `TestSubmit_TimeoutAutoDenies` |
| ctx-cancel exits Submit with `DecisionCancel`; queue entry removed | `TestSubmit_CtxCancelExits` |
| Resolve(unknown id) returns `ErrUnknownApproval` | `TestResolve_UnknownReturnsError` |
| Resolve rejects non-terminal decisions (timeout/cancel/garbage) | `TestResolve_RejectsNonTerminalDecisions` |
| `Pending()` sorted by CreatedAt ascending | `TestPending_SortedByCreatedAt` |
| `approval.requested` payload carries actor + correlation + ID | `TestEvent_RequestedPayloadShape` |
| AskPrompt marks Ask-class capabilities with `RequiresApproval` and a fail-closed Deny | `TestDecide_AskPromptMarksRequiresApproval` |
| Hard-deny still fires under AskPrompt; no approval requested | `TestDecide_AskPromptDoesNotBypassHardDeny` |

10 new tests pass (8 approval + 2 edict). Existing 158 tests
unaffected by the policyHook change. Total module:
**168 passing tests** across **24 packages**, vet clean, depscheck clean.

## Cumulative status

```
24 packages | ~11,200 lines source+tests | 168 tests passing | 2 deps (allowlisted)
```

| Subsystem | LoC | Tests |
|---|---:|---:|
| `kernel/{ulid,event,journal,state,bus,agent,runtime,controlplane}` | ~4,250 | 65 |
| `kernel/edict` | ~600 | 16 |
| `kernel/governor` | 899 | 12 |
| `kernel/warden` | 726 | 9 |
| `kernel/approval` | **578** | **8** |
| `plugins/providers/{mock,anthropic,ollama}` | 1,034 | 13 |
| `plugins/tools/{shell,file,http}` | ~1,360 | 35 |
| `cmd/{agezt,agt}` | ~990 | 8 |
| `internal/{brand,paths}` | 102 | 1 |
| `tools/{jsonschemagen,depscheck}` | 633 | (jsonschemagen: 3 + e2e) |

## Deviations from spec (intentional)

1. **One channel only.** The control plane (TCP localhost + JSON-line)
   is the only approval surface in M1.d. Telegram (SPEC-06 §3.4 lists
   it as the canonical channel), web UI, and in-IDE prompts all
   implement the same `Registry.Resolve` API and land later.
2. **No timeout-policy distinction.** SPEC-06 §3.4 mentions
   "Time-outs default to deny." We synthesise `DecisionTimeout` (not
   `DecisionDeny`) so the journal distinguishes "operator said no"
   from "operator never answered" — both end the run, but the audit
   trail tells you which happened. Effectively the same outcome from
   the agent's perspective; richer for forensics.
3. **No approval scoping** (`allow once / for this task / raise trust`
   from SPEC-06 §3.4). Every approval is single-shot. Scoped grants
   need persistent state (Edict-level changes + a per-task / per-task-
   tree TTL store) that I'd rather build atop the DAG (M1.e) where
   "task" has a concrete identity.
4. **No reverse-channel for the operator's reasoning.** The denying
   operator can include a free-text reason (`agt deny <id> "why"`),
   and it lands in both the event payload and the synthetic
   `tool.result` fed back to the model. The model's own re-plan after
   a denial isn't guided beyond that — the next LLM round just sees
   the deny reason as a tool error.
5. **No bulk decide.** `agt approve --all` / `agt deny --all` are
   trivial to add (loop `Pending()` + call Resolve) but I'd rather
   wait until we have a real flood-of-approvals problem to size them.
6. **No persistence across daemon restarts.** Pending approvals live
   in memory; a `agezt` crash or restart loses them (the originating
   run is also lost — see `kernel/runtime` lifecycle). Persisting
   in-flight approvals is coupled to persisting in-flight runs and
   would land with M2-class crash recovery.

## Open items for M1.e

- **DAG scheduler + Planner** (TASKS P1-SCHED-01..03, P1-PLAN-01) —
  the loop becomes a node type; the approval queue gains a
  `plan-approval-gate` node type that pauses a DAG before high-cost
  branches run.
- **Telegram channel** (TASKS P4-CHAN-01) + out-of-process plugin
  host pattern (DECISIONS B0a) — Telegram becomes a second Resolve
  caller alongside `agt`.
- **Browser tool** (TASKS P1-TOOL-04) — requests ProfileContainer
  through Warden; sensitive-domain interactions automatically request
  approval through the M1.d queue.
- **Warden Linux backend** (TASKS P1-WARD-01).
- **Pulse v1** (TASKS P3-*) — observers, salience, initiative.
- **Live model catalog sync** (TASKS P1-CONDUIT-04 / SPEC-15).
- **Scoped grants** ("allow once / for this task / raise trust") —
  built atop the DAG once "task" has a concrete identity.

## Pointers

- Tests: `go test ./...` (168 pass, vet clean, depscheck OK)
- HITL demo:
  ```
  # shell A — daemon in prompt mode:
  AGEZT_HOME=/tmp/d AGEZT_PROVIDER=mock AGEZT_APPROVAL_MODE=prompt ./bin/agezt

  # shell B — fire a run that needs L2 (shell):
  ./bin/agt run "list files"     # blocks at L2

  # shell C — operator decides:
  ./bin/agt approvals            # see pending; copy the id
  ./bin/agt approve <id>         # → run continues
  # or:
  ./bin/agt deny <id> "nope"     # → run finishes with denial in context
  ```
- Default behaviour: drop `AGEZT_APPROVAL_MODE` and the daemon
  behaves exactly like M1.c (AskAllow, no operator interaction).
- Next milestone report: `PHASE-M1.e-REPORT.md`
