# M552 — E2E: outbound HMAC webhooks + HITL approvals

## Context
Continuing criterion 7. Two more product surfaces driven against the real daemon
with the keyless mock, each with **0 panics** and clean shutdown.

## Outbound webhooks (HMAC) — PASS
A standalone Go sink (HMAC-verifying) was pointed at by
`AGEZT_WEBHOOKS='http://127.0.0.1:PORT/hook|>|secret'` with
`AGEZT_WEBHOOK_ALLOW_LOOPBACK=1`. A single `agt run` produced 6 journal events, and
all 6 were POSTed to the sink, **each with a valid `X-Agezt-Signature: sha256=…`
(`sig_valid=true`, 0 invalid)**: task.received, llm.request, routing.decision,
budget.consumed, llm.response, task.completed. The daemon journaled 6 matching
`webhook.delivered` events.

Security note (working as designed): the egress guard (M416) **blocks the loopback
POST by default** — without `WEBHOOK_ALLOW_LOOPBACK=1` the deliveries became
`webhook.failed` (the netguard default-deny on private/loopback). The expected
exception flag is `=1` (not `=on`).

## HITL approvals — PASS
With `AGEZT_APPROVAL_MODE=prompt` (AskPrompt → live HITL) and a tool-calling demo,
a run that writes a file **blocked on a real `approval.requested`** (capability
`file.write`, level L2). `agt approvals` listed it (id, tool, reason, actor,
input). `agt approve <id>` → journal `approval.granted` → `tool.invoked` →
`tool.result`; the file was actually written to the workspace and the run completed
("…wrote notes.txt and edited it in place"). The deny resolution is the symmetric
branch (`approval.denied`), unit-covered (M504).

## Health
0 panics / runtime errors across both daemon sessions; graceful `agt shutdown`.
The webhook `webhook.failed` events seen first were the egress guard correctly
refusing an un-allowlisted loopback target — not a defect (no failures once the
loopback exception was set).

## Method note
The Go sink first wrote its log to the Windows `/tmp` (not git-bash's), masking the
result; switching it to stderr (captured by the bash redirect) surfaced the HMAC
verdicts. Recorded so the e2e harness logs to stderr, not a `/tmp` file.

## Remaining §7 surfaces
Out-of-process plugin + MCP bridge, and mesh two-node peer delegation. See
`.project/ACCEPTANCE.md`.

## No code change
Pure runtime verification (webhooks + HITL both behaved correctly). ACCEPTANCE
ledger updated.
