# M550 — REAL BUG: OpenAI streaming chat dropped a non-streaming provider's answer

## Context
First milestone of the "EKSIKSIZ, sıfır-hata çalışan agentic mimari" goal — the
new **Runtime/E2E** dimension (criterion 7). I built a keyless e2e harness (temp
`AGEZT_HOME` + `AGEZT_DEMO_ECHO=1` mock) and drove the real daemon across every
HTTP surface. This surfaced a genuine production bug that no unit test had caught.

## The bug
`kernel/openaiapi` `streamChat` relays the kernel's `llm.token` bus events as
`chat.completion.chunk` content deltas. The agent loop only emits those events
when the provider implements `StreamingProvider` (`CompleteStream`); a provider
that satisfies only `Complete` (non-streaming) runs through the plain path and
emits **no token events**. So a `stream:true` request served by a non-streaming
provider produced only the opening `role` chunk and the `stop` chunk — the
**answer was silently dropped** — even though the *same* provider via non-stream
chat returns it in full.

Proof (real daemon, echo mock = non-streaming provider), BEFORE the fix:
```
data: {... "delta":{"role":"assistant"} ...}
data: {... "delta":{},"finish_reason":"stop" ...}
data: [DONE]              ← no content delta; the answer is gone
```

This was a true inconsistency, not a mock artifact: `kernel/openaiapi/responses.go`
(the `/v1/responses` endpoint) **already** had the exact guard
`if full.Len() == 0 && res.answer != "" { emitDelta(res.answer) }` — so `/v1/responses`
handled non-streaming providers correctly while `/v1/chat/completions` did not.

## Fix
In `streamChat`, capture `RunModel`'s returned answer (was discarded as `_, err`)
and, when the run completes with no streamed token deltas (`full.Len() == 0`),
emit the assembled answer as a single content delta before the finish chunk —
mirroring the long-standing `responses.go` behavior. Feeds `full` so the optional
usage chunk stays correct.

```go
} else if full.Len() == 0 && r.ans != "" {
    sendContent(r.ans)   // non-streaming provider: don't drop the answer
}
```

## Verification
- Unit: new `TestChatCompletionStreaming_NonStreamingProviderStillSendsAnswer`
  (fakeEngine with `tokens: nil`, `answer` set) asserts the content delta is
  present. Negative control: reverting the fix makes it FAIL ("answer dropped").
  The existing `TestChatCompletionStreaming` (streaming provider) still passes —
  the fallback only triggers when nothing was streamed.
- E2E: rebuilt the daemon; `stream:true` against the echo mock now emits
  `"content":"[echo]\nSTREAMFIXED"` between role and stop. 0 panics in the log.
- Gate: gofmt staged-blobs clean, `go vet`/`staticcheck` clean on the package,
  full `go test ./...` exit 0 (GOMAXPROCS=3, -p 2), `go.mod`/`go.sum` unchanged.

## E2E surfaces verified green this milestone (criterion 7)
Daemon boot→ready→graceful shutdown; `agt run`/`status`/`doctor`/`journal`;
OpenAI `/v1/models`, `/v1/chat/completions` (+ streaming, now fixed),
`/v1/responses`; native REST `/api/v1/runs`; Web UI (200 with token, 401 without).
All with 0 panics and 0 error-level journal events. See `.project/ACCEPTANCE.md`.

## Note
In production all shipped providers implement `CompleteStream`, so this path was
not reachable with a real provider today — but the OpenAI-compatible contract and
the system's explicit support for non-streaming providers make the drop a real
defect, and the fix removes a foot-gun for any future/local non-streaming provider.
