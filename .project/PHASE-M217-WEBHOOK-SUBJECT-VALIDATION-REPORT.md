# M217 — Validate webhook subject filters at parse time

## Why
An outbound webhook sink is configured in `AGEZT_WEBHOOKS` as `url|subject|secret`, where
`subject` is a NATS-style filter (e.g. `agent.>`, `edict.*.applied`) selecting which
journal events to deliver. `ParseSinks` validated the URL but accepted the subject filter
verbatim. The event router decides delivery with `bus.MatchSubject`, which **returns false
on a malformed pattern** rather than erroring. So a sink with a typo'd filter — an empty
token (`agent..tool`), or `>` not in the final position (`>.agent`) — was silently
accepted at startup and then matched *nothing*: the webhook delivered no events, with no
error anywhere, presenting to the operator as a baffling "my webhook just never fires".

This is the same "silent misconfiguration" class as the M215/M216 spec fixes: catch it at
parse time with a clear message instead of letting it become confusing runtime behaviour.

## What
- **`kernel/bus`** — exported `ValidatePattern(p string) error`, a thin wrapper over the
  existing internal `parsePattern`: it reports whether `p` is a well-formed pattern
  (non-empty, no empty tokens, `>` only as the last token), returning `ErrPattern`-wrapped
  detail otherwise. This gives config parsers a way to validate a pattern up front using the
  exact same rules the live subscription / `MatchSubject` use.
- **`kernel/webhook` (`ParseSinks`)** — after resolving the sink's subject (defaulting to
  `>` when the field is empty), validate it with `bus.ValidatePattern` and return a hard
  error on failure: `webhook: invalid subject filter "<s>": bus: bad pattern: …`. (webhook
  already imported `bus`; no new dependency, no import cycle — `bus` is lower-level.)

A sink with no explicit filter still defaults to `>` (all events), which is valid, so the
common case is unaffected.

## Tests (+2)
- `kernel/bus/bus_test.go` — `TestValidatePattern`: `>`, `agent.>`, `agent.*.tool`, `a.b.c`,
  `*` pass; `""`, `a..b`, `>.a`, `a.>.b` error.
- `kernel/webhook/webhook_test.go` — `TestParseSinks_RejectsBadSubjectFilter`: an empty
  token and a misplaced `>` are rejected; an empty filter still defaults to `>` (one sink,
  no error); a valid explicit filter (`agent.*.tool`) parses.

The existing `TestParseSinks` (valid specs, bad URL, empty spec) remains and passes.

## Verification
- `go test ./...` — 1684 passing (1682 + 2 new), 0 failing.
- `go vet ./kernel/bus/ ./kernel/webhook/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only; webhook→bus already existed).
- Local commit only (no push); standard trailer.

## Files
- `kernel/bus/bus.go` — exported `ValidatePattern`.
- `kernel/webhook/webhook.go` — validate the subject filter in `ParseSinks`.
- `kernel/bus/bus_test.go`, `kernel/webhook/webhook_test.go` — new tests.

## Theme
Continues the config-hygiene thread (M215 peers, M216 plugin pins/allowlist): catch a
silent misconfiguration at startup rather than letting it surface as confusing runtime
behaviour — here, a webhook that's configured but silently delivers nothing.
