# W2.44a typed chat summarize

The chat history summarizer, `chat_summarize`, now runs on the shared
dispatcher in `kernel/app/chat`, beside the chat suggestions. The Chat view
calls it through `/api/chat/summarize` when a thread outgrows the history
window. It then sends the briefing as a leading system turn instead of
dropping the oldest turns.

The handler body, its constants and the turn parser moved verbatim into
`Summarizer`. It reads two ports: the kernel's provider and its default model.

## The first typed LIVE operation that never streams

The native command was `StreamLive` so that a disconnected client cancels the
provider call. It never sent an event frame.

The typed operation keeps `StreamLive` metadata, so the native dispatch still
ties its context to the connection. Its emitter is never called. As the shared
adapter requires for any stream, it declares kernel event frames, with the
event wire schema.

The remaining live cognition commands follow the same pattern.

## Preserved behavior

- **Turns** are `[{role,text}]`. Non-object entries, and entries with a blank
  or non-text role or text, are skipped. No usable turn is refused with
  `args.turns required`.
- **Provider check:** a missing provider is refused before the model is
  examined.
- **Model:** a present `model` must be text, so `null` is refused. Empty text
  means the kernel's default model.
- **The call:** the transcript (prior summaries hoisted by
  `convo.TranscriptIntent`) keeps its last 24 KiB. One `summarize`-routed call
  with 2048 max tokens follows the fixed instruction.
- **Result:** provider errors pass through. The briefing is trimmed, and a
  blank one is refused. The result is the briefing and the number of turns
  folded.
- **Audit and tenancy:** the operation stays audited, and tenant tokens are
  refused.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any audit or provider call.

The controlplane no longer imports `kernel/convo`, so the archcheck ratchet
drops that edge.

## Runnable comparison and regression evidence

The harness runs the pre-slice handler on one kernel and the registered
operation on another. Each side has a provider that answers from the request:
it echoes the model, task type, max tokens and prompt length. It returns a
blank briefing for one marker and an error for another, and it records every
request.

90 steps run twenty times: fifteen inputs under primary, wrong and tenant
tokens, in normal and canceled contexts. The inputs:

- missing, empty and non-array turns;
- turns that are all malformed;
- a valid thread with no model, and with a numeric, `null`, empty or
  HTML-bearing model;
- a prior summary with a stray entry;
- a transcript over the cap;
- the blank and error markers;
- multi-byte text with extra and `tenant` arguments;
- a model without turns.

The results:

- Responses are byte-exact.
- Audit journals are equal.
- Every provider request is equal, side by side.
- Refused callers never reach the provider.
- Asserted: each error, the default and explicit models, the folded-turn
  count, and the capped prompt (the 24 KiB tail plus the 209-byte
  instruction).
- The 15 canceled primary-token steps return the admission error. They journal
  nothing and never call the provider.

Permanent tests cover:

- the parser, both models, the request's shape, routing and cap, each refusal
  and its order, the blank briefing, and the provider error;
- the spec, including its LIVE stream and event emission;
- a native binding test of the kernel's provider and default model and the
  registry's live, audited flags.

The existing summarize suites pass unchanged through the typed path.

Fifteen independent mutations fail tests.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
and scripted providers only.
