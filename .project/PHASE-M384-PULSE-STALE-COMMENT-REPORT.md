# M384 — Fix stale pulse "Deferred" comment + relevance-wiring lock-in (priority-C)

## Audit (read-vs-code)
The goal's category-C example named `pulse.go:15` ("adaptive cadence deferred").
Auditing that comment's full Deferred list against the code:

```
// Deferred: Chronos, standing orders, adaptive cadence,
// autonomous `act`, Telegram delivery, world-model relevance, reflection.
```

- **adaptive cadence** — genuinely deferred (only fixed `defaultCadence = 60s` +
  static `AGEZT_PULSE_CADENCE`; no adaptive/dynamic logic). Comment ACCURATE.
- **Chronos / standing orders / autonomous `act` / reflection** — genuinely
  deferred (no Chronos; DispAct branches to "ask"; pulse never triggers
  kernel/reflect). ACCURATE.
- **Telegram delivery** — STALE. `briefing.go` has a `BriefSink` abstraction and
  the daemon wires `buildTelegram` (+ Slack/Discord/webhook/email) brief sinks
  into `buildPulse(... combineSinks(tgSink, slSink, dcSink, whSink, emSink))`.
  Brief delivery to channels is implemented.
- **world-model relevance** — STALE. `salience.go` has the `Relevance` interface
  + `relevanceBoost` (SPEC-05 §3.4), `engine.go` wires `relevance: cfg.Relevance`,
  and the daemon passes `Relevance: k.World()`. The relevance boost is live.

So two of the seven listed items are no longer deferred — a stale comment.

## What
- **`kernel/pulse/pulse.go`** — rewrote the package-doc note: moved channel brief
  delivery (Telegram/Slack/Discord/webhook/email sinks) and world-model relevance
  out of "Deferred" into "Wired beyond that gate since", keeping the genuinely
  deferred items (Chronos, standing orders, adaptive cadence, autonomous `act`,
  reflection).

## Verification (lock-in for the un-deferred claim)
The two un-deferred features were already unit-tested (`TestSalienceRelevanceBoostLiftsBand`,
`TestSinkFunc`/`TestMultiSink…`) and the engine sink path is covered
(`capturingSink` in `engine_test.go`). The gap: **no engine-tick test set
`Config.Relevance`**, so nothing proved the relevance signal is plumbed through
the FULL tick (engine → salience), only the Salience unit in isolation.

- **`kernel/pulse/relevance_wiring_test.go`** `TestTickPlumbsWorldModelRelevance`:
  a full `tickOnce` with `Config.Relevance = fakeRelevance{known:["Lictor"]}` and
  a delta about "Lictor" → the journaled `salience.scored` reason names the match
  ("relevant to Lictor"); with no Relevance the same delta scores plainly. This
  ties the comment's "world-model relevance is wired" claim to an engine-level
  assertion.
- **Negative control:** setting `engine.go`'s `relevance: cfg.Relevance` to `nil`
  → the test FAILs (reason `"severity=medium"`, no match); restored byte-identical.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2166** passing (was 2165; +1). No CHANGELOG (internal comment + test only).

## Scope notes
- The remaining deferred items in the (now-corrected) comment were re-verified as
  genuinely deferred, so this fix does not introduce a new staleness.
