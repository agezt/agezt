# AGEZT Documentation Index

Start here when evaluating, operating, or integrating AGEZT.

## Positioning and architecture

| Document | Use when you need to... |
|---|---|
| [COMPARISON.md](COMPARISON.md) | understand how AGEZT differs from generic agent frameworks without unverifiable competitor claims |
| [../ARCHITECTURE.md](../ARCHITECTURE.md) | understand the core agent identity, runtime, Web UI, and source-of-truth layout |
| [../ARCHITECTURAL-REPORT.md](../ARCHITECTURAL-REPORT.md) | read the broader generated architecture report and module map |
| [SYSTEM-REVIEW.md](SYSTEM-REVIEW.md) | inspect the latest system-wide review artifact and current-state findings |
| [MISSING-PARTS-REPORT.md](MISSING-PARTS-REPORT.md) | review missing features, documentation gaps, and quality/test risks |
| [MISSING-PARTS-PLAN.md](MISSING-PARTS-PLAN.md) | plan which missing parts to implement, defer, or document |

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
| [GRAVEYARD-POLICY.md](GRAVEYARD-POLICY.md) | understand retired-agent retention posture and the bar for any future destructive automation |
| [CONNECT.md](CONNECT.md) | connect providers and messaging channels, including OAuth and multiple accounts |
| [CONSOLE.md](CONSOLE.md) | understand and operate the embedded Web UI console |

## APIs and SDKs

| Document | Use when you need to... |
|---|---|
| [API-STABILITY.md](API-STABILITY.md) | understand public/private surface stability, versioning policy, and SDK parity rules |
| [EVENT-SCHEMA.md](EVENT-SCHEMA.md) | understand event/journal compatibility rules for audit consumers and demos |
| [SDK-PARITY.md](SDK-PARITY.md) | inspect generated `/api/v1` route coverage across Go/Python/TypeScript/Rust SDKs |

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
