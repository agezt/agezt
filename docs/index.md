# AGEZT Documentation Index

Start here when evaluating, operating, or integrating AGEZT.

## Positioning and architecture

| Document | Use when you need to... |
|---|---|
| [COMPARISON.md](COMPARISON.md) | understand how AGEZT differs from generic agent frameworks without unverifiable competitor claims |
| [OPENCLAW-HERMES-ROADMAP.md](OPENCLAW-HERMES-ROADMAP.md) | track the competitive roadmap for OpenClaw/Hermes parity and AGEZT advantage |
| [COMPETITOR-PARITY.md](COMPETITOR-PARITY.md) | inspect the checkable OpenClaw/Hermes parity ledger behind `agt compare audit` |
| [DEER-FLOW-AGEZT-REPORT.md](DEER-FLOW-AGEZT-REPORT.md) | review what AGEZT can pragmatically borrow from ByteDance DeerFlow |
| [DEER-FLOW-IMPLEMENTATION-PLAN.md](DEER-FLOW-IMPLEMENTATION-PLAN.md) | track the concrete implementation plan for DeerFlow-inspired AGEZT changes |
| [ARCHITECTURE.md](ARCHITECTURE.md) | understand the core agent identity, runtime, Web UI, and source-of-truth layout |
| [AGENT-LOOP-INVARIANTS.md](AGENT-LOOP-INVARIANTS.md) | preserve policy, tool, compaction, delegation, and reload ordering while changing the loop |
| [ARCHITECTURAL-REPORT.md](ARCHITECTURAL-REPORT.md) | read the broader generated architecture report and module map |
| [ARCHITECTURE-DEEP.md](ARCHITECTURE-DEEP.md) | go past the product view to a measurement-based map of every package in the repo |

## Security and governance

| Document | Use when you need to... |
|---|---|
| [THREAT-MODEL.md](THREAT-MODEL.md) | review prompt-injection, tools, plugins, tokens, tenant, network, and isolation threats |
| [PLUGIN-SECURITY.md](PLUGIN-SECURITY.md) | understand plugin trust, BLAKE3 pinning, allowlists, callback bounds, and crash/reload behavior |
| [../DEPENDENCIES.md](../DEPENDENCIES.md) | review Go dependency justifications and the depscheck allowlist policy |

## Operations

| Document | Use when you need to... |
|---|---|
| [OPERATIONS.md](OPERATIONS.md) | run day-2 operations: health, metrics, cost, policy triage, backup/restore, incident runbooks |
| [../Install.md](../Install.md) | bootstrap, verify, and build the repo with repeatable commands |
| [GRAVEYARD-POLICY.md](GRAVEYARD-POLICY.md) | understand retired-agent retention posture and the bar for any future destructive automation |
| [CONNECT.md](CONNECT.md) | connect providers and messaging channels, including OAuth and multiple accounts |
| [CONSOLE.md](CONSOLE.md) | understand and operate the embedded Web UI console |

## APIs and SDKs

| Document | Use when you need to... |
|---|---|
| [API-STABILITY.md](API-STABILITY.md) | understand public/private surface stability, versioning policy, and SDK parity rules |
| [EVENT-SCHEMA.md](EVENT-SCHEMA.md) | understand event/journal compatibility rules for audit consumers and demos |
| [SDK-PARITY.md](SDK-PARITY.md) | inspect generated `/api/v1` route coverage across Go/Python/TypeScript/Rust SDKs |
| [AGENT-SDK-ARCHITECTURE.md](AGENT-SDK-ARCHITECTURE.md) | understand how agent-written code in a subprocess reaches back into AGEZT (gateway, tokens, capabilities) |

## Engineering program

Working documents for in-flight refactoring and repair. These describe intent and progress, not
the shipped contract — read the architecture and operations docs above for what the system does.

| Document | Use when you need to... |
|---|---|
| [REFACTORING-INDEX.md](REFACTORING-INDEX.md) | enter the refactoring effort: every plan, its evidence, and the dependency graph between them |
| [REFACTORING-SCAN-2026-08.md](REFACTORING-SCAN-2026-08.md) | follow the current master scan and phase-by-phase action plan (Phases 0–3 merged, 4 in progress) |
| [VERIFICATION-GATES-REPAIR-PLAN.md](VERIFICATION-GATES-REPAIR-PLAN.md) | see what the `make check` gates guard, how two of them silently broke, and how they were repaired |
| [DEAD-CODE-AUDIT.md](DEAD-CODE-AUDIT.md) | review the 2026-06-28 dead-code cleanup and the findings behind the deadcode gate |

## Point-in-time status reports

Snapshots taken against a specific commit, kept for history. Each carries the date and HEAD it was
measured at — check that stamp before trusting a claim, and verify against current source.

| Document | Use when you need to... |
|---|---|
| [JARVIS-VISION-2026.md](JARVIS-VISION-2026.md) | read the strategic position and roadmap toward the autonomous-assistant goal |
| [NEXT.md](NEXT.md) | pick up the continuation plan and handoff notes for the next contributor |
| [SPEC-IMPLEMENTATION-STATUS.md](SPEC-IMPLEMENTATION-STATUS.md) | trace the SPEC ↔ code status matrix |
| [SYSTEM-AUDIT-REPORT.md](SYSTEM-AUDIT-REPORT.md) | review the audited list of missing items, incompletes, and to-dos |
| [MISSING-PARTS-REPORT.md](MISSING-PARTS-REPORT.md) + [MISSING-PARTS-PLAN.md](MISSING-PARTS-PLAN.md) | inspect the raw missing-pieces inventory and the plan that answers it |

## Runnable positioning demos

| Demo | Proves |
|---|---|
| [../examples/autonomous/policy-denial-audit/](../examples/autonomous/policy-denial-audit/) | governance is runtime-enforced and auditable |
| [../examples/autonomous/mailbox-delegation/](../examples/autonomous/mailbox-delegation/) | agents are durable identities with wake causality and hierarchy |
| [../examples/autonomous/typed-schedule-system-task/](../examples/autonomous/typed-schedule-system-task/) | schedules are typed infrastructure, not cron-wrapped prompts |
| [../examples/autonomous/plugin-governance/](../examples/autonomous/plugin-governance/) | tools/plugins are governed, visible, and hash-pinned where external |

## Generated / checkable artifacts

| Artifact | Check command |
|---|---|
| [SDK-PARITY.md](SDK-PARITY.md) | `go run ./tools/sdkparity -check docs/SDK-PARITY.md` |
| [../DEPENDENCIES.md](../DEPENDENCIES.md) + `tools/depscheck/allowlist.txt` | `go run ./tools/depscheck` |
| [COMPETITOR-PARITY.md](COMPETITOR-PARITY.md) | `agt compare audit --json` |
| repository-local reachability (no unreachable code outside the allowlists) | `go run ./tools/deadcodecheck` |
| `CHANGELOG/` split layout | `go run ./tools/changelog-lint` |
| `contract/gen/types.gen.go` ↔ `.project/agezt-contract.jsonc` | `make gen` then verify no diff |
