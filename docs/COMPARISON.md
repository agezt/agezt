# AGEZT Compared With Generic Agent Frameworks

AGEZT is not positioned as another prompt runner, chat wrapper, or workflow-only automation tool. It is an auditable autonomous-agent runtime: agents are durable identities that can sleep, wake, act under policy, communicate, delegate, and leave an inspectable event trail.

This document intentionally avoids unverifiable claims about named competitors. Instead, it compares AGEZT with common patterns in generic agent frameworks and states what is implemented, what is still being hardened, and what evidence should be used to evaluate the claim.

## Positioning statement

AGEZT is for operators who want autonomous agents that remain under authority:

- durable agent identities, not disposable prompt sessions
- governed tool use, not unconstrained function calling
- event-sourced audit, not opaque chat history
- typed schedules and workflows, not hidden prompt storage
- proactive operation, not only request/response chat
- extensibility through governed plugins, tools, channels, and SDKs

The short version:

> AGEZT is the auditable autonomous-agent runtime for durable agents that can sleep, wake, act under policy, explain every action, and recover.

## Comparison matrix

| Axis | Generic agent framework pattern | AGEZT position |
|---|---|---|
| Agent model | A run, prompt, chain, or chat session is treated as the agent. | Agents are durable roster identities with lifecycle, memory, skills, authority, mailbox, workspace, model route, retry/repair policy, and ownership metadata. |
| Autonomy model | Mostly user-triggered runs; scheduled work is often a prompt with a timer. | Schedules are typed infrastructure: agent wake, workflow run, system task, or governed tool target. Standing orders, mailbox wakes, delegation, doctor wakes, and Pulse make autonomy explicit. |
| Audit trail | Logs or traces may exist, but often outside the core model. | Events are first-class. Runs, wake evidence, tool decisions, policy denials, costs, and outcomes are intended to be inspectable through journal/status/UI/CLI. |
| Policy and authority | Tool allowlists or approvals are often bolted on around function calls. | Edict/trust/tool policy is a core runtime concern. Tool denials are enforced and journaled; effective authority is being tightened end-to-end. |
| Tool execution | Tools execute in-process or through arbitrary shell/function calls. | In-process and out-of-process tools are governed. High-risk tools such as shell, file, HTTP, and code execution have explicit effect metadata, bounds, audit hooks, and containment controls. |
| Persistence | State is often app-specific or memory-provider-specific. | Roster, journal, state, memory, world model, skills, schedules, standing orders, artifacts, and config are separate durable concerns. |
| Multi-agent communication | Delegation may be ad hoc or hidden inside a run. | Board/mailbox, directed messages, wake linkage, delegated sub-agents, parent/child ownership, and agent detail surfaces are explicit system concepts. |
| Workflows | Chains are often the primary abstraction. | Workflows are reusable chains, but not identities. Agents may run workflows; workflows do not become agents. |
| Operator control | Mostly API/SDK control surfaces. | CLI, Web UI, REST, OpenAI-compatible API, ACP, SDKs, approvals, halt/resume, doctor, status, pulse, and journal inspection are part of the product surface. |
| Extensibility | Packages/plugins are often framework-level code. | AGEZT supports first-party tools/channels/providers plus out-of-process plugins with pinning, allowlists, progress, callbacks, and an MCP bridge. |
| Deployment shape | Often a library embedded in an app. | AGEZT is a daemon + CLI with embedded Web UI and API surfaces. It can be driven by clients or run as a resident local/server process. |

## What AGEZT already has strong evidence for

These claims are grounded in the current repository structure and documented behavior.

### 1. Durable agents instead of prompt sessions

AGEZT separates agent identity from chat sessions, schedules, and workflows. The architecture centers on durable profiles in `kernel/roster`, execution in `kernel/runtime`, communication in `kernel/board`, and operator/API surfaces in `kernel/controlplane` and the frontend.

Evidence to inspect:

- `ARCHITECTURE.md`
- `kernel/roster/`
- `kernel/runtime/`
- `kernel/controlplane/roster.go`
- `frontend/src/components/AgentDetail.tsx`
- `frontend/src/views/Roster.tsx`

### 2. Wake causality as a product concept

AGEZT tracks why an agent woke, not only what it said after waking. Current architecture notes describe autonomy runbook evidence flowing from event payloads into status, Agent Detail, and activity timelines across manual, schedule, standing, mailbox, delegated, and doctor-triggered wakes.

Evidence to inspect:

- `NEXT.md`
- `ARCHITECTURE.md`
- `kernel/controlplane/schedule_fires.go`
- `kernel/controlplane/standing.go`
- `kernel/runtime/subagent.go`
- `cmd/agezt/auto_repair.go`

### 3. Governance is part of runtime, not just UI

Tool policy, trust levels, denials, approval surfaces, and tool effects are core concepts. The current plan notes that loop tool calls and direct `RunTool` paths journal policy decisions.

Evidence to inspect:

- `kernel/agent/agent.go`
- `kernel/runtime/toolrun.go`
- `kernel/edict/`
- `kernel/controlplane/tool.go`
- `frontend/src/components/AgentDetail.tsx`

### 4. Event-sourced explainability

AGEZT is designed around an event log and journal inspection rather than treating chat history as the source of truth. The CLI exposes journal and `why` flows; API and UI surfaces fold events into run/agent status.

Evidence to inspect:

- `kernel/event/`
- `kernel/journal/`
- `cmd/agt/journal_*`
- `cmd/agt/why.go`
- `kernel/controlplane/runs.go`

### 5. Security-conscious high-risk tools

The file, HTTP, shell, and code-execution tools show explicit attention to containment, output caps, environment scrubbing, SSRF resistance, and audit. This is a stronger posture than treating tools as opaque function calls.

Evidence to inspect:

- `plugins/tools/file/file.go`
- `plugins/tools/http/http.go`
- `plugins/tools/shell/shell.go`
- `plugins/tools/codeexec/codeexec.go`
- `kernel/netguard/netguard.go`
- `kernel/warden/warden.go`

## Where AGEZT should not overclaim yet

The project is active pre-release work. These are the areas where AGEZT should be precise rather than absolute.

### 1. Effective authority is still being hardened end-to-end

The UI exposes authority concepts, and runtime policy enforcement exists, but the strongest product claim requires a single explainable path from displayed permissions to API representation to runtime decision to journal event.

Do not claim: "All displayed permissions are fully enforced everywhere."

Safer claim: "AGEZT treats authority as a first-class runtime concern and is tightening end-to-end effective-policy proof."

Recommended next proof:

```text
agt agent authority <slug> --explain
```

The command should show effective tool allow/deny, trust ceiling, approval requirements, memory scope, config access, schedule/channel wake permissions, and the source of each rule.

### 2. Schedule target hardening is still a differentiator to finish

AGEZT's schedule philosophy is strong: schedules are typed infrastructure, not hidden prompt storage. The remaining work is to prove each target type end-to-end and prevent system-task/tool payloads from smuggling arbitrary agent instructions.

Do not claim: "Schedules are fully hardened across every target type."

Safer claim: "AGEZT models schedules as typed triggers and is hardening each target path with explicit audit fields."

### 3. Strong isolation varies by platform

The warden layer is honest about effective isolation. Linux can enforce stronger process controls than Windows/macOS in some paths; shell and code execution remain high-blast-radius capabilities and must be governed by policy.

Do not claim: "All tool execution is sandboxed equally on every platform."

Safer claim: "AGEZT records requested vs effective isolation and treats high-risk tools as governed, auditable capabilities."

### 4. The product surface is broad and needs guided demos

AGEZT has many surfaces: CLI, daemon, Web UI, REST, OpenAI-compatible API, ACP, SDKs, channels, plugins, marketplace, schedules, workflows, Pulse, memory, world, skills, vault, and tunnel. Breadth is an advantage only when a user can quickly see the core value.

Do not claim: "The full platform is self-evident from the README."

Safer claim: "AGEZT has a broad platform surface; the clearest adoption path is through focused autonomous-agent demos."

## Best comparison demos

To position AGEZT well, compare using runnable scenarios rather than abstract feature lists.

### Demo 1: Policy denial and audit

**Status: implemented** — `examples/autonomous/policy-denial-audit/`

Show a high-risk tool request being denied, journaled, and visible in agent diagnostics.

```bash
bash examples/autonomous/policy-denial-audit/run.sh
```

### Demo 2: Mailbox wake and agent hierarchy

**Status: implemented** — `examples/autonomous/mailbox-delegation/`

Show durable agent creation, parent/child ownership, effective authority, and wake causality.

```bash
bash examples/autonomous/mailbox-delegation/run.sh
```

### Demo 3: Typed schedule, not prompt storage

Show a schedule targeting a system task or agent identity with typed payload and inspectable fire history.

Evidence command shape:

```bash
agt schedule add --system-task catalog_sync --every 24h
agt schedule fires --json
agt why <schedule_fire_event_id>
```

### Demo 4: Effective authority proof

**Status: implemented** — `agt agent authority <slug>`

Show the effective runtime authority for an agent, merged from profile + live policy overlay.

```bash
agt agent authority <slug>
agt agent authority <slug> --json
```

### Demo 5: Plugin/tool governance

Show a plugin or tool being listed, governed, invoked, and audited, including what happens on failure.

Evidence command shape:

```bash
agt plugin list
agt tool list
agt run "use the approved tool to fetch status"
agt why <tool_event_id>
```

## Messaging guidance

Use this language:

- "durable agents, not prompt sessions"
- "autonomy under policy"
- "wake causality and audit trail"
- "typed schedules, not cron-wrapped prompts"
- "workflow is a chain, agent is an identity"
- "requested vs effective isolation is explicit"
- "operator authority remains visible"

Avoid this language unless backed by a runnable test/demo:

- "fully autonomous"
- "secure sandbox on every platform"
- "production-ready"
- "better than [specific project]"
- "complete governance"
- "zero-risk tool execution"

## Recommended roadmap to strengthen positioning

### Priority 1: Comparison evidence pack

**Done.** Four runnable demos exist:

- `examples/autonomous/policy-denial-audit/` — implemented
- `examples/autonomous/mailbox-delegation/` — implemented
- `examples/autonomous/typed-schedule-system-task/` — implemented
- `examples/autonomous/plugin-governance/` — implemented

### Priority 2: Effective authority proof

**Done.** `agt agent authority <slug> [--json] [--explain]` is implemented and tested. It merges the agent profile with the live Edict policy snapshot and renders effective tool allow/deny, trust ceiling (with cap annotations), memory scope, approval mode, capability levels, and the hard-deny floor.

See: `cmd/agt/agent.go`, `cmd/agt/agent_authority_test.go`.

### Priority 3: Threat model

**Done.** `docs/THREAT-MODEL.md` covers prompt injection, tool misuse, process isolation (with platform caveats), secret exposure, token exposure, channel abuse, plugin compromise, tenant boundary, SSRF, and workspace escape.

### Priority 4: Operations proof

**Done.** `docs/OPERATIONS.md` covers health/readiness probes, metrics, cost management, policy triage, event audit, backup/restore drills, vault management, incident runbooks, and a monitoring checklist.

### Remaining priorities

Both original priorities are now complete:

- ~~Harden schedule typed-target execution end-to-end~~ — done (`fix(cadence): harden typed schedule target validation`)
- ~~Add high-risk approval visibility~~ — done (`feat(agents): surface high-risk approval history in agent diagnostics`)

Next priorities for platform maturity:

- Add memory/world/skill audit CLI commands (`agt memory audit`, `agt world audit`, `agt skill eval`) so epistemic hygiene is operator-visible.
- Make plugin protocol versioning machine-checkable (manifest/protocol version field).
- Add behavioral SDK parity tests beyond route-string coverage (typed request/response, error semantics, per endpoint).

## Bottom line

AGEZT's strongest position is not "more agents" or "more tools." The strongest position is:

> an autonomous-agent runtime where identity, authority, memory, schedule, communication, action, and audit are all first-class system objects.

That is meaningfully different from generic agent frameworks. The remaining work is to package the claim as evidence: runnable demos, effective authority proof, threat model, and operations guidance.

## Related documentation

| Document | Status | Covers |
|---|---|---|
| `docs/COMPARISON.md` | this document | positioning vs generic agent frameworks |
| `docs/THREAT-MODEL.md` | implemented | T1–T10 threats, controls, limitations, deployment checklist |
| `docs/OPERATIONS.md` | implemented | health, metrics, cost, triage, backup/restore, runbooks |
| `examples/autonomous/policy-denial-audit/` | implemented | governance is runtime, not just UI |
| `examples/autonomous/mailbox-delegation/` | implemented | durable identity, authority, wake causality |
| `agt agent authority <slug>` | implemented | effective runtime policy proof |
| `examples/autonomous/typed-schedule-system-task/` | implemented | typed schedules, not cron-wrapped prompts |
| `examples/autonomous/plugin-governance/` | implemented | plugin trust, allowlists, audit |
| `docs/PLUGIN-SECURITY.md` | implemented | plugin trust model, pinning, crash/reload |
| `docs/API-STABILITY.md` | implemented | public/private surface stability, versioning, SDK parity |
| `docs/SDK-PARITY.md` | implemented | generated `/api/v1` route coverage across SDKs |
