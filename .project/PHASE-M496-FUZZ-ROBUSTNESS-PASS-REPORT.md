# M496 — Active fuzz robustness pass (all 16 targets re-run clean)

## Context
The prior arc (M444–M454) *created* 16 fuzz targets over every untrusted/external
parser. This milestone *actively re-runs* them — the strongest robustness check —
rather than relying on the historical "clean" claim, and records the result.

## Targets exercised (bounded time each; no crashers)
All 16 ran to completion with **exit 0, no crash, no panic** — no reproducer was
written to any `testdata/fuzz/` directory:

| Area | Target | Package |
|------|--------|---------|
| Control plane request parse (network-facing) | FuzzRequestParse | kernel/controlplane |
| Secret redaction | FuzzRedact | kernel/redact |
| Policy decision (incl. normalization) | FuzzDecide | kernel/edict |
| Journal open / segment scan | FuzzJournalOpen | kernel/journal |
| Catalog API-file parse | FuzzParseAPIFile | kernel/catalog |
| OpenAI-compat content shape | FuzzChatMessageContent | kernel/openaiapi |
| Pricing arithmetic | FuzzCostMicrocents | kernel/governor |
| Webhook HMAC verify | FuzzVerify | plugins/channels/webhook |
| Slack signature verify | FuzzVerify | plugins/channels/slack |
| Discord signature verify | FuzzVerify | plugins/channels/discord |
| Provider stream parse ×6 | FuzzParseStream / FuzzParseEventStream | anthropic, openai, google, cohere, ollama, bedrock |

## CPU note (operator feedback, applied)
Go fuzzing defaults its worker count to `GOMAXPROCS`; on this 32-core machine that
pegged the CPU at 100%. Per operator feedback, fuzz/test runs are now capped with
`GOMAXPROCS=3` (+ `-parallel=3` for fuzzing) — 3 workers instead of 32 — which keeps
the machine usable during runs with no loss of correctness coverage (only slower
exploration). This practice is recorded for future sessions.

## Verification
- 16/16 fuzz targets exit 0; no `testdata/fuzz/` reproducers created; tracked tree
  clean; `go.mod`/`go.sum` unchanged.
- This is a verification milestone (no code change); it confirms the fuzz criterion in
  `.project/HARDENING.md` by execution, not by assertion.
