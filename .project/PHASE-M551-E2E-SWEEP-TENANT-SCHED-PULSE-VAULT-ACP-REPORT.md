# M551 — E2E sweep: multi-tenant, scheduler, pulse, vault, ACP

## Context
Continuing criterion 7 (runtime/E2E) of the zero-defect goal. Drove the real
daemon (built from HEAD) across the CLI-reachable surfaces with the keyless echo
mock under temp `AGEZT_HOME`. Each surface must complete its flow with **0 panics**
and **0 error-level journal events** and shut down cleanly.

## Surfaces verified green
- **Multi-tenant** (`AGEZT_MULTITENANT=on`): `tenant create acme` → per-tenant dir +
  token; `tenant list` shows it; `agt run --tenant acme` returns the echo answer;
  **isolation proven** — the primary journal head stayed at seq=0 after the tenant
  run (the tenant run lives in acme's own journal, not the primary's).
- **Scheduler**: `schedule add "…" --every 1h` → `sched-…` id; `schedule list`
  shows it `[operator,enabled]` with a computed next-fire time. (HITL approvals
  path still PARTIAL — pending a follow-up.)
- **Pulse**: `pulse status` → running, dial=balanced, digest pending=0; `budget`
  reports global spend vs ceiling.
- **Vault** (offline): `provider creds set` stores masked; `vault encrypt` →
  `aes-256-gcm`, `pbkdf2-hmac-sha256 (200000 iterations)`; **at-rest check**: the
  ciphertext file contains 0 occurrences of the plaintext secret; `vault rotate`
  re-encrypts under a new passphrase with entries preserved.
- **ACP server**: `agt acp` over stdio answers an `initialize` JSON-RPC request
  with a well-formed result (`agentInfo` agezt 1.0.0, `protocolVersion` 1,
  capability flags).

## Health
Across both daemon sessions: graceful shutdown via `agt shutdown` (exited cleanly),
**0 panics / runtime errors** in the logs, **0 error-level journal events** (the
only `error`-matching log line is the skill-auto-quarantine description text).

## Remaining §7 surfaces (next)
Outbound webhooks (HMAC sink), out-of-process plugin + MCP bridge, mesh two-node
peer delegation, and the HITL approvals path. See `.project/ACCEPTANCE.md`.

## Note
No code change this milestone — pure runtime verification + ACCEPTANCE ledger
update. (M550 was the one code fix this dimension surfaced so far.)
