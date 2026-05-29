# Phase Report — Milestone 1.q.y (agent loop streaming integration)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §5 (Streaming)](SPEC-15-PROVIDER-ECOSYSTEM.md).
> Continues [PHASE-M1.q.x.x.x.x-REPORT.md](PHASE-M1.q.x.x.x.x-REPORT.md).

## Scope

The deferral from M1.q (and re-deferred from M1.q.x, M1.q.x.x,
M1.q.x.x.x, M1.q.x.x.x.x) finally lands. Streaming providers
existed since M1.q; nothing in the daemon's run path used them.
M1.q.y wires `agent.Run` to detect `StreamingProvider` and emit
token events to the bus, with the CLI rendering them inline during
`agt run`.

The decision the deferral kept punting on — "journal one event per
token? per buffered batch? not at all?" — is answered: **not at
all**. Token chunks are *ephemeral*: they flow through a new bus
side-channel (`PublishStreaming`) that bypasses the journal
entirely. The canonical audit record stays in the existing
`llm.response` event, which is journaled durably after the stream
completes and carries the full assembled text + usage.

| Concern | Status |
|---|---|
| `agent.Run` detects `StreamingProvider` via type assertion | ✅ |
| Streaming path emits `KindLLMToken` per text delta to the bus | ✅ |
| Token events are ephemeral (no journal, no hash, no seq advance) | ✅ tested |
| Final `llm.response` still durable; usage + assembled text intact | ✅ tested |
| Non-streaming providers fall back to `Complete` unchanged | ✅ tested |
| Pattern matching works for streaming events (per-run subscriptions deliver tokens) | ✅ tested |
| CLI renders `KindLLMToken` text inline; other events keep the `[evt seq=…]` summary | ✅ |
| Journal verify still passes when streams are interleaved with durable events | ✅ tested (seq advance check) |
| Compile-time guard via `IsEphemeral()` helper | ✅ |

## Changes

### 1. `kernel/event/event.go` — `NewEphemeral` + `IsEphemeral`

```go
// NewEphemeral constructs an event suitable for display-only fan-out
// via bus.PublishStreaming. It bypasses the hash chain entirely:
// Seq=0, PrevHash="", Hash="". Subscribers can detect ephemeral
// events with `if ev.IsEphemeral() { ... }`.
func NewEphemeral(spec Spec) (*Event, error)

// IsEphemeral reports whether the event was created via NewEphemeral.
// Hash=="" is the discriminator (durable events always have a
// computed BLAKE3 hash via New()).
func (e *Event) IsEphemeral() bool { return e.Hash == "" }
```

Validation mirrors `New()` minus the prevHash check. The
discriminator is Hash, not Seq — the journal's first durable event
has Seq=0 (zero-valued counter, not 1-based), so Seq alone wouldn't
reliably distinguish ephemeral from "first durable in a fresh
journal." Hash is computed by `New()` and never empty for durable
events; an empty Hash unambiguously means "didn't pass through
journal.Append."

### 2. `kernel/bus/bus.go` — `PublishStreaming`

```go
// PublishStreaming fans out an ephemeral event to matching subscribers
// WITHOUT persisting it to the journal. Use only for high-rate
// display-only signals (LLM token chunks via KindLLMToken) where
// the durable record lives elsewhere — the full assembled text and
// usage land in the regular llm.response event published right
// after the stream completes.
func (b *Bus) PublishStreaming(spec event.Spec) (*event.Event, error)
```

Same locking + fan-out as `Publish`, just skips `j.Append`.
Pattern-matching behavior is identical — a subscriber on
`agent.01H.>` will receive both ephemeral and durable events with
matching subjects. The CLI's per-run subscription works without
any change to the subscribe path.

### 3. `kernel/agent/agent.go` — type-assert and dispatch

```go
if sp, ok := cfg.Provider.(StreamingProvider); ok {
    resp, err = sp.CompleteStream(ctx, req, func(c Chunk) error {
        if c.TextDelta == "" { return nil }
        _, _ = cfg.Bus.PublishStreaming(event.Spec{
            Subject:       subject("llm"),
            Kind:          event.KindLLMToken,
            Actor:         cfg.Actor,
            CorrelationID: cfg.CorrelationID,
            Payload:       map[string]any{"iter": iter, "text": c.TextDelta},
        })
        return nil
    })
} else {
    resp, err = cfg.Provider.Complete(ctx, req)
}
```

Three design notes:

1. **No new fields on `LoopConfig`.** The streaming behavior is
   inferred from the Provider's type, not an opt-in flag. Every
   StreamingProvider gets the streaming path; every plain Provider
   gets `Complete`. No coordination needed between caller and
   loop.

2. **Tool deltas are not surfaced.** Only `TextDelta` chunks
   publish ephemeral events today. `ToolUseStart`/`ToolInputJSONDelta`/
   `ToolUseStop` get suppressed because the existing
   `tool.invoked` + `tool.result` events already cover the
   lifecycle from a journaling perspective, and rendering streamed
   tool inputs requires UI work the CLI hasn't taken on yet. The
   `Chunk` shape lets us surface them later without an interface
   change.

3. **Streaming failures don't fall back to Complete.** Per the
   `StreamingProvider` interface contract, CompleteStream returns
   the same response Complete would for the same request. Any
   error is a real upstream problem; silently retrying via
   Complete would mask network flakiness and double-bill on
   eventually-succeeding requests.

### 4. `cmd/agt/main.go` — inline token rendering

The existing `[evt seq=N kind=K]` summary line stays for every
event *except* `KindLLMToken`. Tokens render inline:

```go
inStream := false
closeStream := func() {
    if inStream { fmt.Fprintln(stdout); inStream = false }
}
result, err := c.Stream(ctx, controlplane.CmdRun, ..., func(ev *event.Event) {
    if ev.Kind == event.KindLLMToken {
        var p struct{ Text string `json:"text"` }
        _ = json.Unmarshal(ev.Payload, &p)
        if p.Text != "" {
            if !inStream { fmt.Fprint(stdout, "  "); inStream = true }
            fmt.Fprint(stdout, p.Text)
        }
        return
    }
    closeStream()
    fmt.Fprintf(stdout, "  [evt seq=%d kind=%s]\n", ev.Seq, ev.Kind)
})
closeStream()
```

The `inStream` flag prevents tokens and non-token events from
interleaving badly — if `tool.invoked` arrives mid-stream, the
text fragment-line gets closed off cleanly before the bracket-line
prints.

### 5. Tests

**`kernel/event/event_test.go`** (+3 tests):
- `TestNewEphemeral_HasNoChainFields` — all chain fields zero,
  metadata intact.
- `TestNewEphemeral_ValidatesRequiredFields` — 3 sub-cases for
  missing subject/kind/actor.
- `TestIsEphemeral_DurableIsFalse` — round-trip via `New()` to
  confirm the discriminator stays accurate.

**`kernel/bus/bus_test.go`** (+2 tests):
- `TestPublishStreaming_DoesNotJournal` — publishes 5 streaming
  events, confirms a subsequent durable Publish still gets seq=0
  (proving the streaming events didn't leak into the journal),
  and a second durable gets seq=1 (proving the counter is still
  healthy).
- `TestPublishStreaming_RespectsPatternMatching` — confirms that
  ephemeral events still respect subscribers' subject patterns
  the same way durable events do.

**`kernel/agent/agent_test.go`** (+2 tests):
- `TestRun_UsesStreamingWhenAvailable` — full integration: a
  custom `streamProv` exposes both `Complete` and `CompleteStream`;
  Run must call CompleteStream (gotInvoked stays false), publish
  one `KindLLMToken` per text fragment (all ephemeral), and
  *still* publish the durable `KindLLMResponse` with assembled
  content.
- `TestRun_StreamingFallsBackToCompleteForNonStreamingProvider` —
  the existing mock provider (no CompleteStream method) keeps
  working unchanged.

## Architectural consequences

1. **The bus now has a two-channel model.** Until M1.q.y, every
   event was durable: the bus had exactly one method (Publish)
   and durability was an invariant. Now there's `Publish` (durable,
   journaled, hashed) and `PublishStreaming` (ephemeral,
   display-only, not journaled). This is a real expansion of the
   bus's contract — but it's gated behind a discriminator
   (`IsEphemeral`) that lets every existing consumer continue to
   assume durability.

2. **Tokens don't appear in `agt why`.** The `why <event_id>`
   command walks the journal by correlation_id. Since tokens
   aren't journaled, they don't show up in causal-chain analysis.
   This is correct — the question "what caused this run?" is
   answered by tool calls and policy decisions, not by partial
   text. The full text is in `llm.response` which IS in the
   chain.

3. **`journal verify` is unaffected.** Streaming events don't
   touch the hash chain at all, so verify continues to walk the
   exact same set of events it always did. The seq-advance test
   in `TestPublishStreaming_DoesNotJournal` is the regression
   guard.

4. **CLI tokens render at "as fast as the bus delivers."** The
   subscription channel buffer is 1024 events per run. For a
   model emitting 50 tokens/sec, that's ~20 seconds of buffer
   before the bus starts dropping. The bus's drop-counter
   semantics apply: if the operator's terminal can't keep up
   (unlikely on local stdout, possible over ssh), some tokens
   would be dropped and counted, but the durable answer is still
   intact. Acceptable for v1.

5. **The "is it ephemeral?" decision is structural-typing
   friendly.** External tools subscribed via the bus can ask
   `ev.IsEphemeral()` to decide whether to persist for their own
   audit trails. Nothing needs to know the specific Kind list of
   ephemerals (today just KindLLMToken; tomorrow maybe
   KindReasoningDelta, KindCacheHit, etc).

## Demo (end-to-end via real `agt run`)

Before M1.q.y, with the same streaming-capable Anthropic provider:

```
$ agt run "Say pong in one word"
  [evt seq=0 kind=task.received]
  [evt seq=1 kind=llm.request]
  [evt seq=2 kind=llm.response]
  [evt seq=3 kind=task.completed]

--- final answer ---
pong
(correlation_id: 01HQ...)
```

After M1.q.y, same command:

```
$ agt run "Say pong in one word"
  [evt seq=0 kind=task.received]
  [evt seq=1 kind=llm.request]
  pong
  [evt seq=2 kind=llm.response]
  [evt seq=3 kind=task.completed]

--- final answer ---
pong
(correlation_id: 01HQ...)
```

For a longer response, the difference is the difference between
"frozen prompt for 8 seconds" and "watching the answer appear."

## Deferrals → next phases

**Tool input streaming via the CLI** — the `ToolUseStart`/
`ToolInputJSONDelta`/`ToolUseStop` chunks pass through
`CompleteStream` but the agent loop doesn't surface them to the
bus. Adding `KindLLMToolInputDelta` (also ephemeral) would let
operators watch a model construct a `shell` command character by
character. Real value for debugging, but the existing
`tool.invoked` event arriving when the call actually runs is
already operator-visible, so the urgency is low.

**Bedrock streaming** — still requires AWS event-stream binary
framing. Separate phase.

**Vertex Anthropic** — still depends on M1.n.x.

**M1.q.y.x: plan streaming** — `handlePlan` in the controlplane
uses a similar event-stream pattern. Same renderer changes would
apply if plan-executed agents stream too. ~30 LoC follow-up.

**Unchanged longstanding deferrals:**
- Hot reload of catalog + vault.
- Subscription-first routing.
- OS-keychain encryption for the vault.
- Bedrock SigV4 + non-Anthropic vendor body shapes.
- Browser tool, out-of-process plugin host.
- Pulse v1, planner.

## Files touched

```
kernel/event/event.go           + NewEphemeral, IsEphemeral (~40 LoC)
kernel/event/event_test.go      + 3 tests (4 sub-cases)
kernel/bus/bus.go               + PublishStreaming (~45 LoC)
kernel/bus/bus_test.go          + 2 tests
kernel/agent/agent.go           + type-assert and dispatch in Run (~30 LoC modification)
kernel/agent/agent_test.go      + 2 tests + streamProv helper (~150 LoC)
cmd/agt/main.go                 + inline KindLLMToken renderer in cmdRun (~25 LoC)
```

No schema changes (events with new shape are validated by existing
event tests). No daemon-side wiring beyond agent.go. No new
external deps.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 421 pass, 0 fail (up from 411 in M1.q.x.x.x.x)
```

The full operator UX trajectory:

| Milestone | New capability |
|---|---|
| M1.f | `agt catalog sync/list/discover` |
| M1.o | `agt provider creds set/list/rm` |
| M1.p | `agt provider check [id]` |
| M1.p.x | `agt provider check --all` |
| M1.p.y | `--json` + `--bench N` |
| M1.q | StreamingProvider + Anthropic SSE + `--stream` |
| M1.q.x | OpenAI streaming → 4 families × ~11 vendors |
| M1.q.x.x | Google Gemini streaming |
| M1.q.x.x.x | Vertex Gemini streaming |
| M1.q.x.x.x.x | Ollama + Cohere streaming |
| **M1.q.y** | **Live streaming during real `agt run` invocations** |
