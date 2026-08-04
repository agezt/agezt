# DeerFlow -> AGEZT Adoptable Ideas Report

> Review date: 2026-06-26
> DeerFlow source: https://github.com/bytedance/deer-flow
> Reviewed commit: `7a6c4a994a86583d2a3c056ee9d0f157d4f030c2` (`fix(channels): serialize per-chat thread creation to avoid duplicate threads (#3799)`)
> AGEZT sources: `README.md`, `docs/ARCHITECTURE.md`, `docs/NEXT.md`, `kernel/runtime`, `kernel/agent`, `kernel/skill`, `kernel/workflow`, `frontend/src/views/FlowStudio.tsx`

## Short conclusion

DeerFlow 2.0 is not an architecture that should replace AGEZT. DeerFlow is a Python/LangGraph-based "super agent harness"; AGEZT is a Go-based agentic operating system centered on event-journal/policy/durable agents. So what should be adopted is not LangGraph or the Python runtime, but some of the patterns DeerFlow has productized:

- decomposing agent behavior into small, observable middleware pieces,
- making skill activation explicit, auditable, and decoupled from the UI,
- pinning sub-agent/backend/frontend contracts with a shared fixture,
- reducing schema load in MCP and large tool catalogs via deferred discovery,
- not losing skill instructions during context compaction,
- making setup/doctor/onboarding coding-agent friendly,
- presenting the docs and demo surface as a "real, working harness".

AGEZT already has areas where it is stronger than DeerFlow: durable roster identity, typed schedules, wake evidence, Edict policy, BLAKE3 hash-chain journal, plugin governance, multi-channel surface, SDK parity, and a single static Go daemon. The report's recommendation: rather than copying DeerFlow as a core, selectively adopt its ergonomics and contract discipline on top of this strong foundation.

## What does DeerFlow do?

DeerFlow 2.0 positions itself as "Deep Exploration and Efficient Research Flow" and a "super agent harness". Repo structure:

- Backend: Python 3.12+, FastAPI, LangGraph/LangChain agent runtime, `backend/packages/harness/deerflow/*`.
- Frontend: Next.js 16 / React 19 / TypeScript, Nextra docs, thread/artifact UI.
- Runtime concepts: lead agent, middleware chain, sandbox, skills, subagents, memory, run manager, event store, checkpointer, MCP/tool search.
- Operations: `make setup`, `make doctor`, Docker/provisioner sandbox, config hot reload boundary, coding-agent oriented `Install.md`.

The main idea in DeerFlow's design documents is "a harness, not a framework": lead agent + tool routing + middleware + sandbox + skills + memory + subagents + config together, forming a ready-made working environment for long tasks.

## Differences from AGEZT

| Area | DeerFlow | Implication for AGEZT |
|---|---|---|
| Runtime language | Python + LangGraph/LangChain | Preserve AGEZT's Go/std-lib-first line. Adopting LangGraph would introduce a dependency in the wrong direction. |
| Agent model | Lead agent and named custom agents, thread/checkpoint centric | AGEZT's durable roster identity model is stronger; thread ergonomics and named-agent UX ideas can be taken from DeerFlow. |
| Tool governance | Middleware guardrails, sandbox audit, deferred tools | Edict/journal in AGEZT is stronger; DeerFlow's schema deferral and contract fixture patterns should be added. |
| Skills | `SKILL.md`, slash activation, tool allowlists, skill rescue | AGEZT's Forge/skill system is more auditable; explicit activation and compaction rescue should be adopted. |
| Subagents | `task` tool, status contract, token usage aggregation | AGEZT's `delegate`/`delegate_await` is more advanced; the frontend/backend result contract discipline should be adopted. |
| Context | Summarization middleware, skill rescue, dynamic context reminders | AGEZT has context budget/elision; the skill/resource preservation layer should be strengthened. |
| Docs/onboarding | Website docs, demos, `Install.md`, setup wizard | AGEZT's docs set is strong but the adoption flow is not as "try it and see" oriented as DeerFlow's. |

## Recommended adoptions

### 1. Shared backend/frontend contract fixtures

In DeerFlow, `contracts/subagent_status_contract.json`, the backend `status_contract.py`, the frontend `subtask-result.ts`, and the tests on both sides all use the same fixture. The goal is to prevent the frontend card lifecycle from silently breaking when the backend changes the "task result succeeded/failed" format.

AGEZT equivalent:

- Add a `contracts/*.json` fixture set for the event/payload contracts the UI folds, such as `subagent.spawned`, `subagent.completed`, `delegate_await`, `policy.decision`, `agent.wake.autonomy_runbook`, `approval.*`, and `run status`.
- Have Go tests and frontend Vitest tests read the same fixture.
- In particular, bind surfaces like `last_autonomy_runbook`, the mailbox wake badge, delegated/doctor lineage, and the policy denial passport to these contracts.

Priority: high. Large impact, low risk.

### 2. Explicit skill activation

In DeerFlow, when the user types `/skill-name task`, `SkillActivationMiddleware` reads the relevant `SKILL.md` content from a safe root, passes it to the model as hidden context concealed from the UI, and audits the activation event. AGEZT currently performs skill retrieval via automatic keyword/recency scoring; this is good, but the operator lacks a deterministic "use this skill in this run" path.

AGEZT equivalent:

- Support `agt run --skill <name> "<intent>"` and `/skill-name` inside the Web UI chat.
- Write it to the correlation via `skill.activation` or an additional payload on the existing skill event family.
- The activation context should not land in the UI conversation history as ordinary user text, but should remain visible via `agt why`/journal.
- The existing `skill op=files/read` routing must be preserved for skill resources.

Priority: high. Makes AGEZT's Forge/skill claim more usable.

### 3. Skill/resource rescue during context compaction

DeerFlow's summarization middleware tries to preserve skill file read results; it prevents recently loaded skill instructions from being lost when summarizing older messages. AGEZT has `ContextBudget`, `ContextProtectFirst/Last`, and `SummarizeElided`; that is a good foundation, but it is unclear whether skill/resource read results are specially protected.

AGEZT equivalent:

- On the context budget path (similar to `compactMessages`), mark `skill` tool results and injected skill blocks as a special class.
- Keep the last N skill/resource reads, or up to a total token/char budget, as "protected context".
- When a skill rescue happens, add fields such as `skill_rescued_count` and `skill_rescued_chars` to the `context.compacted` payload.

Priority: high. Prevents skill behavior from degrading in long tasks.

### 4. Deferred MCP/tool schema discovery

DeerFlow's `tool_search` pattern does not bind the entire set of MCP tools to the model up front. It lists tool names briefly, and the model promotes the schema via `tool_search` when needed. Promotion state is scoped by a catalog hash and is established after policy filtering.

AGEZT equivalent:

- In AGEZT, tools become callable in the next run after an MCP attach; tool schema clutter can grow with large MCP catalogs.
- Combine this with the existing `ToolSelector`/semantic discovery to promote schemas on demand.

Priority: medium-high. Becomes necessary as the MCP marketplace grows.

### 5. Invariant documentation for middleware/pipeline ordering

In DeerFlow's lead agent file, the middleware order and the placement of tracing callbacks are written down as explicit invariants. In AGEZT the agent loop is deliberately first-party and monolithic; that is an advantage. But as ordering dependencies grow — policy, tool timeout, context compaction, steering, tool memo, artifact offload, observation taint, run lifecycle — pinning the invariants with tests and documentation becomes more valuable.

AGEZT equivalent:

- I am not proposing a large refactor. First, add a "loop phase order" document for `kernel/agent` plus small golden tests.
- Afterwards, internal hook/pipeline types can be extracted only in low-risk areas: `BeforeModel`, `BeforeToolGate`, `AfterToolResult`, `BeforeCompact`.
- Edict gating and journal order must absolutely not be moved: deterministic policy decision first, then tool invoke, then result.

Priority: medium. Done correctly it eases maintenance; a rushed refactor is risky.

### 6. Config reload boundary registry

DeerFlow keeps, in `reload_boundary.py`, a single source of truth for which config fields hot reload and which require a restart. AGEZT has distinctions for provider reload, catalog/vault reload, runtime env, and daemon startup; but a single registry answers the operator's "does this change require a restart?" question better.

AGEZT equivalent:

- A "startup-only/runtime-reloadable" registry under `kernel/configcenter` or docs.
- Have `agt doctor` and the Web UI Config Center generate warnings from this registry.
- Bind the existing provider/catalog/vault reload behaviors to this registry.

Priority: medium. Improves day-2 ops quality.

### 7. Sandbox security messaging and effective isolation visibility

DeerFlow states plainly that host bash is not a safe boundary for the local sandbox, and disables the bash subagent by default. AGEZT has warden, netguard, Edict, and tool effects — stronger, but the answer to "what is the actual isolation on this platform?" should be more visible to the user.

AGEZT equivalent:

- Requested vs effective isolation in `agt doctor` and the Web UI Tools/Sandbox screen.
- Clear badges for shell/code_exec/file tools such as "host-level", "warden-limited", "containerized", "remote".
- Show Windows/macOS/Linux differences without hiding them.

Priority: medium.

### 8. Coding-agent oriented onboarding

DeerFlow wrote its `Install.md` specifically so that coding agents can set up the repo: idempotent, Docker-first, no secret reads, exact next command. AGEZT has `agt quickstart` and docs; but a single-file setup prompt to hand to an external coding agent is separately valuable.

AGEZT equivalent:

- `Install.md` (existing) or, later, a separate `CODING-AGENT-INSTALL.md` variant.
- Rules such as "do not read secret-bearing `.env`", "run `make build`/`agt quickstart` first", "return the exact next command before starting the daemon".
- A small checklist to make `agt doctor` output more actionable.

Priority: medium.

### 9. Public demo gallery and artifact-first docs

DeerFlow's frontend docs carry real demo thread/artifact examples. AGEZT has runnable autonomous demos, but they read more like in-repo test evidence. For an external user, "what did this produce?" should be visible faster.

AGEZT equivalent:

- `examples/autonomous/` (the existing runnable demos) or, later, a "Demos" page inside the Web UI.
- The expected event timeline and artifact output for the policy denial, mailbox delegation, typed schedule, and plugin governance demos.
- A "which AGEZT claim does this prove?" section for each demo.

Priority: medium.

### 10. Maintainer-orchestrator security pattern

DeerFlow's maintainer orchestrator notes teach a good security lesson: first confine the agent to a reversible surface such as comment-only, and expand authority gradually on public surfaces.

AGEZT equivalent:

- A reversible-surface policy for AGEZT system agents like "repo guardian" / "PR reviewer".
- In the first stage it produces comments/drafts only; no branch/merge/release authority.
- This pattern fits AGEZT's Edict/trust ceiling model very well.

Priority: medium-low, but it makes a good showcase.

## What should not be adopted

- Do not move the LangGraph/LangChain runtime into the AGEZT core. It conflicts with AGEZT's first-party Go loop, its policy/journal ordering, and the single-binary goal.
- Do not switch to a Next.js/Nextra frontend. AGEZT's embedded Vite SPA approach fits daemon distribution better.
- Do not copy the complexity of DeerFlow's large `config.example.yaml` as-is. If AGEZT's config surface grows, it should be opened up gradually through the config center/doctor.
- Do not put DeerFlow's thread-centric mental model ahead of AGEZT's agent identity model. In AGEZT, thread/chat must stay subordinate to durable agent identity.
- Do not market a host-local sandbox as if it were secure. DeerFlow is honest about this; AGEZT should also show effective isolation per platform openly.

## Suggested implementation order

1. Contract fixtures: `contracts/autonomy_runbook.json`, `contracts/subagent_result.json`, `contracts/policy_decision.json`; Go + frontend tests.
2. Explicit skill activation: CLI `--skill`, chat `/skill`, journal event, agent-scope validation.
3. Skill rescue in context compaction: protected skill/resource messages, `context.compacted` payload extension.
4. Deferred MCP discovery design doc + minimal implementation behind a feature flag.
5. Config reload boundary registry + doctor warning.
6. Demo gallery and coding-agent install doc.
7. Optional internal loop phase invariant docs/tests before any middleware refactor.

## Quick wins

- Make this report visible from the AGEZT docs index.
- Check some of the doc references in the README: a previous review found a missing "system review" reference; in this checkout the correct target is now `docs/SYSTEM-AUDIT-REPORT.md`. Likewise, the `docs/MISSING-PARTS-REPORT.md` and `docs/MISSING-PARTS-PLAN.md` references inside `docs/NEXT.md` are now verified.
- Adding a DeerFlow-style `Install.md` is low cost and makes it easier for external agents to try AGEZT.

## Source files

DeerFlow:

- https://github.com/bytedance/deer-flow/tree/7a6c4a994a86583d2a3c056ee9d0f157d4f030c2
- `README.md`
- `Install.md`
- `backend/packages/harness/deerflow/agents/lead_agent/agent.py`
- `backend/packages/harness/deerflow/agents/middlewares/tool_error_handling_middleware.py`
- `backend/packages/harness/deerflow/agents/middlewares/skill_activation_middleware.py`
- `backend/packages/harness/deerflow/agents/middlewares/summarization_middleware.py`
- `backend/packages/harness/deerflow/agents/middlewares/deferred_tool_filter_middleware.py`
- `backend/packages/harness/deerflow/tools/builtins/tool_search.py`
- `backend/packages/harness/deerflow/sandbox/security.py`
- `backend/packages/harness/deerflow/config/reload_boundary.py`
- `contracts/subagent_status_contract.json`
- `frontend/src/content/en/introduction/core-concepts.mdx`
- `frontend/src/content/en/harness/design-principles.mdx`

AGEZT:

- `README.md`
- `docs/ARCHITECTURE.md`
- `docs/NEXT.md`
- `docs/COMPARISON.md`
- `kernel/agent/agent.go`
- `kernel/runtime/runtime.go`
- `kernel/runtime/subagent.go`
- `kernel/skill/skill.go`
- `kernel/skill/retrieve.go`
- `kernel/workflow/workflow.go`
- `plugins/tools/mcptool/tool.go`
- `frontend/src/views/FlowStudio.tsx`
