# M557 — E2E: multi-tenant HTTP auth/data isolation + scheduler firing + reflection

## Context
Continuing the deep e2e hunt (M550 proved running surfaces finds real bugs). This
milestone targets the highest-value un-exercised surface: the **multi-tenant
authorization boundary over the HTTP API** (a cross-tenant token bypass would be a
serious security defect), plus tenant data isolation, real scheduled firing, and
reflection. All against the real daemon, 0 panics.

## Multi-tenant HTTP auth isolation — AIRTIGHT (no defect)
`AGEZT_MULTITENANT=on`, OpenAI API, two tenants (alpha, bravo) each with its own
token + the daemon admin token. `X-Agezt-Tenant` header + `Authorization: Bearer`:

| token → tenant | result | expected |
|---|---|---|
| alpha → alpha | 200 | ✅ |
| alpha → bravo | **401** | ✅ cross-tenant denied |
| bravo → alpha | **401** | ✅ cross-tenant denied |
| admin → alpha | 200 | ✅ admin authorizes any |
| admin → bravo | 200 | ✅ |
| alpha → (no tenant header) | **401** | ✅ a tenant token can't drive the primary |
| wrong token → alpha | 401 | ✅ |

No principal can act beyond its grant. (Matches the unit-level controlplane
tenant-token verification, M530, now confirmed over the live HTTP surface.)

## Tenant data isolation
alpha and bravo have **separate journal directories**
(`tenants/alpha/journal/…`, `tenants/bravo/journal/…`), each its own hash chain —
a tenant's runs never touch another tenant's or the primary's journal.

## Scheduler firing — works (resolution is by design, not a defect)
A `schedule add --every 2s` registers and **fires**: `schedule list` shows
`last: completed`, and a new governed run appears in the journal. It fires at the
cadence engine's **10 s `DefaultResolution`** (the ticker wakes every 10 s), so a
sub-10 s interval effectively fires per-tick, not every 2 s. This is documented
design (`cadence.go DefaultResolution`; each fire is a full agent run — sub-10 s
scheduling isn't a real use case; `MinInterval=1s` is only a busy-loop floor), not
a correctness bug: the scheduled intent runs and completes correctly.

## Reflection
`agt reflect run` returns a reflection report (tasks/pulse/skills counters); 0
panics.

## Health
0 panics / runtime errors across the session; graceful shutdown; working tree clean
(all e2e in a temp home).

## Verdict
No new defect. The multi-tenant security boundary — the highest-stakes item checked
— is airtight over the live API. No code change.
