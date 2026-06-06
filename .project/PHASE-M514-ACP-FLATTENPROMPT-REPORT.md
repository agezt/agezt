# M514 — Mutation testing acp: pin flattenPrompt block selection

## Context
Twenty-fifth package in the mutation pass: `kernel/acp` (the Agent Client Protocol
server — JSON-RPC 2.0 over stdio that IDEs like Zed drive). Run with `GOMAXPROCS=3`
(CPU-capped). go-mutesting score 0.552, 64 survivors; working tree restored clean.

## Triage — notification path is defended-in-depth (equivalent), not a gap
The dispatch `default` branch guards `if len(req.ID) > 0` before replying method-not-found
(no reply to a notification). A first candidate test (unknown-method notification → no
reply) did NOT kill `len(req.ID) > 0 → >= 0`: `replyError` has its OWN `len(id) == 0`
guard, so each guard masks the other — both single mutations are equivalent. That test was
reverted rather than committed as a false gap-closure (honesty over score).

## The genuine gap (closed)
`flattenPrompt(blocks)` concatenates a prompt's text content blocks into one intent:

```
if b.Type == "text" || (b.Type == "" && b.Text != "") {
    if out != "" { out += "\n" }
    out += b.Text
}
```

Every other test sends a SINGLE `{"type":"text"}` block, so three properties were
unpinned and survived (confirmed by hand-applied negative control against the existing
suite):
- `b.Type == "" → !=` (F2) — survived: which blocks count flips.
- `b.Text != "" → ==` (F3) — survived: an empty-text empty-type block would be included.
- the branch `&& → ||` (F4) — survived: a non-text typed block (e.g. image) with a text
  field would be wrongly folded into the intent.
The newline join and the `type=="text"` selector were already pinned (the single-block
tests check the exact intent string).

## Fix
Added `TestFlattenPrompt_BlockSelection` (unit test, `package acp`): a 5-block prompt —
text / type-omitted-with-text / image-with-text / empty-empty / text — must flatten to
`"one\ntwo\nthree"`, exercising the join, the lenient type-omitted branch, and the
exclusion of non-text and empty blocks.

## Negative control (manual, CPU-capped)
F2 (`== → !=`), F3 (`!= → ==`), F4 (`&& → ||`), and F1 (join `!= "" → ==`) each FAIL
under the new test. Restored byte-for-byte (`git diff --ignore-all-space` on acp.go
empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — twenty-five packages (M490–M514)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus,
cadence, runtime, tenant, worldmodel, approval, memory, skill, standing, catalog, plugin,
webhook, channel, anomaly, restapi, acp — plus the controlplane primary-token auth gate
verified solid. The acp notification/auth handling is defended-in-depth (equivalent
survivors); the gap was the untested multi-block prompt flattening.
