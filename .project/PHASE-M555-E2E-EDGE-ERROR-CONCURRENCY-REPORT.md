# M555 — E2E hardening: deeper agentic flows + error/edge/concurrency/lifecycle

## Context
Criterion 7 had verified the happy path of every surface. "devam et" → push past
happy paths into the deeper agentic flows and the adversarial/edge/concurrency/
lifecycle scenarios where real defects hide (M550 proved running-the-thing finds
what unit tests miss). All against the real daemon; **0 panics throughout**.

## Deeper agentic flows (demo-scripted, real tool loop)
| Flow | Env | Result |
|---|---|---|
| Multi-agent delegation | `AGEZT_DEMO_DELEGATE` | lead → sub-agent → report; completed, 0 panics |
| Loop guard | `AGEZT_DEMO_LOOP` | correctly fired `max iterations exceeded` (graceful task.failed, not a hang/panic) |
| Prompt cache | `AGEZT_DEMO_CACHED` | mostly-cached answer; completed |
| SSRF egress guard | `AGEZT_DEMO_SSRF` | the cloud-metadata fetch was **refused by the egress guard** |
| Vision attachment path | `AGEZT_DEMO_VISION` | image-attachment path exercised (0 images via CLI); completed |

The loop-guard `task.failed` is the **correct** outcome (a runaway loop must be
stopped), reached without panic or hang — not a defect.

## Error / edge / input-validation (OpenAI + REST APIs)
- bad token → 401; no token → 401.
- malformed JSON → 400; empty messages → 400 ("no usable message content").
- raw 17 MB non-JSON → 400 (JSON parse fails first); **valid 17 MB JSON → 413**
  (the genuine `MaxBytesReader` size-limit path).
- unknown model id → handled (model is advisory; the governor routes) — echoed answer.
- REST: bad-auth 401, malformed 400, empty-intent 400.

## Concurrency + lifecycle
- **10 simultaneous** chat requests → all 200 (no races, no corruption).
- **halt → run refused** (`kernel is halted`) → **resume → run works**.
- **Journal BLAKE3 hash chain verified `{"ok":true}` after ~73 events** spanning
  runs + halt + concurrency; doctor all-[OK]; graceful shutdown under accumulated
  state (exited cleanly).

## Verdict
No new defect surfaced — the daemon handles adversarial input, edge cases,
concurrency, and lifecycle transitions gracefully with 0 panics and an intact audit
chain. This strengthens criterion 7 beyond happy-path coverage. No code change.
