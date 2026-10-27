# AGEZT Documentation Index

Start here when evaluating, operating, or integrating AGEZT.

The **canonical architecture** lives in [`../architecture/`](../architecture/00-README.md) — the numbered documents there
describe the tree as it is, and `../architecture/NEXT.md` is the living handoff. This index is the public face: the
operation guides, the policies and the evidence.

## Positioning and architecture

| Document | Use when you need to... |
|---|---|
| [COMPARISON.md](COMPARISON.md) | understand how AGEZT differs from generic agent frameworks without unverifiable competitor claims |
| [OPENCLAW-HERMES-ROADMAP.md](OPENCLAW-HERMES-ROADMAP.md) | track the competitive roadmap for OpenClaw/Hermes parity and AGEZT advantage |
| [COMPETITOR-PARITY.md](COMPETITOR-PARITY.md) | inspect the checkable OpenClaw/Hermes parity ledger behind `agt compare audit` |
| [AGENT-LOOP-INVARIANTS.md](AGENT-LOOP-INVARIANTS.md) | preserve policy, tool, compaction, delegation, and reload ordering while changing the loop |
| [../architecture/00-README.md](../architecture/00-README.md) | read the canonical architecture: the package index and the per-area map |

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
| [CONSOLE-IA.md](CONSOLE-IA.md) | follow the console's information-architecture law while extending it |

## APIs and SDKs

| Document | Use when you need to... |
|---|---|
| [API-STABILITY.md](API-STABILITY.md) | understand public/private surface stability, versioning policy, and SDK parity rules |
| [EVENT-SCHEMA.md](EVENT-SCHEMA.md) | understand event/journal compatibility rules for audit consumers and demos |
| [SDK-PARITY.md](SDK-PARITY.md) | inspect generated `/api/v1` route coverage across Go/Python/TypeScript/Rust SDKs |
| [AGENT-SDK-ARCHITECTURE.md](AGENT-SDK-ARCHITECTURE.md) | understand how agent-written code in a subprocess reaches back into AGEZT (gateway, tokens, capabilities) |

## Engineering program

Working documents for the refactoring program. These describe intent and progress, not the shipped contract — read the
architecture and operations docs above for what the system does. The current program is the **W0–W5 migration**, tracked
in [`../architecture/21-migration-roadmap.md`](../architecture/21-migration-roadmap.md).

| Document | Use when you need to... |
|---|---|
| [REFACTORING-INDEX.md](REFACTORING-INDEX.md) | enter the refactoring effort: every plan, its evidence, and the dependency graph between them |
| [REFACTORING-SCAN-2026-08.md](REFACTORING-SCAN-2026-08.md) | follow the master scan that preceded the W program, and what it found |
| [FRONTEND-REFACTOR-PLAN.md](FRONTEND-REFACTOR-PLAN.md) | see the frontend carve-out template the shipped console followed |

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
| the documentation index itself (every row points at a file that exists) | `go test ./tools/docsindex/` |

## Archived documents

These describe the tree as it was. They are kept in place — the history is in the git log — and each one names its
canonical successor in its own banner. **Do not read them as current**: the successor column says where the live
answer is.

| Archived document | What it was | Canonical successor |
|---|---|---|
| [ARCHITECTURE.md](ARCHITECTURE.md) | an architecture map naming packages that have since moved | [`../architecture/00-README.md`](../architecture/00-README.md) |
| [ARCHITECTURE-DEEP.md](ARCHITECTURE-DEEP.md) | a measured package map from 2026-08-04 | [`../architecture/00-README.md`](../architecture/00-README.md) |
| [ARCHITECTURAL-REPORT.md](ARCHITECTURAL-REPORT.md) | a generated architecture report at HEAD `ef7b412d` | [`../architecture/00-README.md`](../architecture/00-README.md) |
| [NEXT.md](NEXT.md) | the June 2026 handoff plan | [`../architecture/NEXT.md`](../architecture/NEXT.md) |
| [SYSTEM-AUDIT-REPORT.md](SYSTEM-AUDIT-REPORT.md) | the 2026-07-04 audit | [`../architecture/00-README.md`](../architecture/00-README.md) |
| [MISSING-PARTS-REPORT.md](MISSING-PARTS-REPORT.md) | the raw missing-pieces inventory | [`../architecture/21-migration-roadmap.md`](../architecture/21-migration-roadmap.md) |
| [MISSING-PARTS-PLAN.md](MISSING-PARTS-PLAN.md) | the plan that answered that inventory | [`../architecture/21-migration-roadmap.md`](../architecture/21-migration-roadmap.md) |
| [SPEC-IMPLEMENTATION-STATUS.md](SPEC-IMPLEMENTATION-STATUS.md) | the SPEC ↔ code status matrix | [`../architecture/21-migration-roadmap.md`](../architecture/21-migration-roadmap.md) |
| [DEAD-CODE-AUDIT.md](DEAD-CODE-AUDIT.md) | the 2026-06-28 cleanup snapshot | `tools/deadcodecheck` |
| [JARVIS-VISION-2026.md](JARVIS-VISION-2026.md) | the 2026-07-02 strategy snapshot | [`../architecture/21-migration-roadmap.md`](../architecture/21-migration-roadmap.md) |
| [VERIFICATION-GATES-REPAIR-PLAN.md](VERIFICATION-GATES-REPAIR-PLAN.md) | the repair plan for the gates themselves | the `Makefile` |
| [REFACTORING-SCAN.md](REFACTORING-SCAN.md) | the July scan | [`../architecture/21-migration-roadmap.md`](../architecture/21-migration-roadmap.md) |
| [REFACTOR-A1-CONTROLPLANE-PLAN.md](REFACTOR-A1-CONTROLPLANE-PLAN.md) | the control-plane plan | [`../architecture/21-migration-roadmap.md`](../architecture/21-migration-roadmap.md) |
| [REFACTOR-A2-LOG-PAGINATION-PLAN.md](REFACTOR-A2-LOG-PAGINATION-PLAN.md) | the log-pagination plan | [`../architecture/21-migration-roadmap.md`](../architecture/21-migration-roadmap.md) |
| [REFACTOR-A3-HTTPSERVER-PLAN.md](REFACTOR-A3-HTTPSERVER-PLAN.md) | the first HTTP-server plan | [`../architecture/03-control-plane-and-http.md`](../architecture/03-control-plane-and-http.md) |
| [REFACTOR-A3-B5-AUTH-HTTPSERVER-PLAN.md](REFACTOR-A3-B5-AUTH-HTTPSERVER-PLAN.md) | the auth HTTP-server plan | [`../architecture/03-control-plane-and-http.md`](../architecture/03-control-plane-and-http.md) |
| [REFACTOR-C2-LIB-CLASSIFICATION.md](REFACTOR-C2-LIB-CLASSIFICATION.md) | the frontend library classification | [`../architecture/11-frontend-console.md`](../architecture/11-frontend-console.md) |
| [REFACTOR-C4-CHAT-DECOMPOSITION.md](REFACTOR-C4-CHAT-DECOMPOSITION.md) | the chat-view decomposition | [`../architecture/11-frontend-console.md`](../architecture/11-frontend-console.md) |
| [AUTONOMOUS-SYSTEM-REFACTOR-PLAN.md](AUTONOMOUS-SYSTEM-REFACTOR-PLAN.md) | the program before the W program | [`../architecture/21-migration-roadmap.md`](../architecture/21-migration-roadmap.md) |
| [DEEP-RESEARCH-HARNESS-PLAN.md](DEEP-RESEARCH-HARNESS-PLAN.md) | the deep-research harness plan | [`../architecture/07-autonomy-and-extensibility.md`](../architecture/07-autonomy-and-extensibility.md) |
| [DEER-FLOW-AGEZT-REPORT.md](DEER-FLOW-AGEZT-REPORT.md) | the external DeerFlow review | [`../architecture/07-autonomy-and-extensibility.md`](../architecture/07-autonomy-and-extensibility.md) |
| [DEER-FLOW-IMPLEMENTATION-PLAN.md](DEER-FLOW-IMPLEMENTATION-PLAN.md) | the plan that answered that review | [`../architecture/07-autonomy-and-extensibility.md`](../architecture/07-autonomy-and-extensibility.md) |
| [CHANGELOG-REORG-PLAN.md](CHANGELOG-REORG-PLAN.md) | the changelog reorganisation plan | `CHANGELOG/` |
