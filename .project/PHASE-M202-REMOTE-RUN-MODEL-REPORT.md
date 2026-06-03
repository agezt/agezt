# M202 — `remote_run` model selection (capability-aware delegation)

## Why
The M8 mesh `remote_run` tool delegates a task to a peer node by POSTing
`{"intent": task}` to the peer's `/api/v1/runs`. It had **no way to choose the
peer's model** — the peer always used its own default. So even after M201 told an
operator *which* peer can serve a given model, the agent still couldn't actually
*request* that model on the peer. The dispatch half of capability-aware delegation
was missing.

The peer's `restapi` `runRequest` already carries a `model` field and falls back to
the peer's default when it is empty. The fix is therefore small and backward-safe:
forward a caller-pinned model into the body; omit it otherwise.

## What
`plugins/tools/peer/peer.go`:
- **Tool schema + description**: `remote_run` gains an optional `model` property
  ("Pin the model the peer routes this task to … Omit to let the peer use its own
  default model"), and the description points at it.
- **Invoke**: parses `model`, trims it, and includes it in the POST body **only when
  non-empty**:
  ```go
  payload := map[string]string{"intent": task}
  if model != "" { payload["model"] = model }
  ```
  An absent or whitespace-only model produces the exact same `{"intent":…}` body as
  before — default delegations are byte-for-byte unchanged.
- **Result footer**: the peer echoes the model it actually routed to (`runRequest`
  response includes `model`); `render` now records it as
  `[peer=<name> model=<id> correlation=<corr>]` when present (and the original
  `[peer=… correlation=…]` when the peer reports no model), giving the delegating
  node an auditable trail of which remote model produced the answer.

No new endpoint, no new dependency; the peer still runs the task through its own
governed loop (its Edict/journal/governor), so delegation does not bypass the peer's
policy — it just selects the model within it.

## Tests (+3)
`plugins/tools/peer/peer_model_test.go` (reusing the `fakePost` body-capture seam):
- `TestRemoteRun_ForwardsModel` — a pinned `model:"opus"` appears as `"model":"opus"`
  in the POST body alongside the intent, and the footer shows `model=opus`.
- `TestRemoteRun_OmitsModelByDefault` — with no model, the body carries only the
  intent (no `model` key) and the footer has no `model=` segment — proving the
  default path is unchanged.
- `TestRemoteRun_BlankModelTreatedAsUnset` — a whitespace-only model is trimmed to
  empty and not forwarded.

The pre-existing `TestRemoteRun_HappyPath` (whose scripted response has no `model`)
still passes unchanged, confirming the footer is backward-compatible.

## Verification
- `go test ./...` — 1631 passing (1628 + 3 new), 0 failing.
- `go vet ./plugins/tools/peer/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only).
- Local commit only (no push); standard trailer.

## Files
- `plugins/tools/peer/peer.go` — `model` input, conditional forward, footer echo.
- `plugins/tools/peer/peer_model_test.go` — new model-forwarding tests.

## Scope note
M201 (discover which peer serves a model) + M202 (request that model on the peer)
together complete *manual* capability-aware delegation. The remaining automation —
the tool itself picking the peer for a requested model — is deliberately left to a
future milestone to keep this one single-purpose and free of an extra discovery
round-trip per call.
