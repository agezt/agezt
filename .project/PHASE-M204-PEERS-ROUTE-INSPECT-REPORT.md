# M204 — `agt peers route <model>` (inspect mesh routing decisions)

## Why
M203 made `remote_run` auto-route a task to a peer that serves a requested model.
But the decision was a black box: an operator had no way to ask "which peer *would*
a task for `opus` go to, and what happens if that peer is down?" without actually
dispatching a (side-effecting, Ask-gated) run. `agt peers models` shows each peer's
inventory (the forward index), but answering the routing question meant reading every
peer's list and mentally replaying the selection.

M204 surfaces the router's decision directly, read-only: `agt peers route <model>`
reports the chosen peer and the fallback order, mirroring the M203 algorithm exactly,
so mesh wiring can be verified and routing outcomes explained before anything runs.

## What
`cmd/agt/peers.go`:
- **New verb `route <model>`** (`agt peers route <model> [--json]`). The parser now
  accepts a third verb and treats the bare positional as the model id for `route`
  (a peer name for `models`); `route` without a model is a usage error (exit 2).
- **`peersRoute`** queries every peer's `GET /api/v1/models` (via the existing
  `fetchPeerModels`, bounded-read) **in name order** — the same deterministic order
  the M203 auto-router uses — and marks the first reachable peer that serves the model
  as `chosen`, the remaining servers as fallback, the rest as non-servers, and any
  unreachable peer with its error. Exits `1` when no reachable peer serves the model
  (so it gates scripts), `0` otherwise.
- **Output**: text prints a `model "<m>" — would route to: <peer>` header (or
  `no reachable peer serves it`) followed by a per-peer line
  (`serves (chosen)` / `serves (fallback)` / `does not serve` / `UNREACHABLE: …`).
  `--json` emits a `peerRoute[]` (`name,url,reachable,serves,chosen,error`). Tokens
  are never printed.

This is pure inspection over the existing peer REST surface — no new endpoint, no new
dependency, and it dispatches nothing.

## Tests (+5)
`cmd/agt/peers_route_test.go` (reusing the `modelsServer` httptest helper):
- `TestPeers_Route_ChoosesSortedFirstServer` — two servers → sorted-first is `chosen`,
  the other `serves` but not chosen.
- `TestPeers_Route_Text` — text output carries the `would route to: <peer>` header,
  `chosen`, and `does not serve`.
- `TestPeers_Route_NoServerExits1` — a model no peer serves → exit 1 +
  `no reachable peer serves it`.
- `TestPeers_Route_SkipsUnreachable` — a 500ing first peer is shown unreachable and the
  next server is chosen (mirrors the M203 skip-unreachable behaviour).
- `TestPeers_Route_RequiresModel` — `route` with no model → exit 2.

## Live proof (network-free)
- `agt peers --help` lists the new `route` verb and its description.

## Verification
- `go test ./...` — 1641 passing (1636 + 5 new), 0 failing.
- `go vet ./cmd/agt/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only).
- Local commit only (no push); standard trailer.

## Files
- `cmd/agt/peers.go` — `route` verb, parser update, `peerRoute`, `peersRoute`.
- `cmd/agt/peers_route_test.go` — new routing-inspection tests.

## Mesh thread (M8) so far
- **M200** — bounded the `agt peers` health read.
- **M201** — `agt peers models` (discover a peer's models).
- **M202** — `remote_run {model}` (pin a model on a named peer).
- **M203** — `remote_run` auto-routes by model (cross-node routing).
- **M204** — `agt peers route <model>` (inspect the routing decision).

Discovery, dispatch, automatic routing, and now routing *observability* are all in
place. A natural follow-on — load/cost-aware choice among multiple servers, and
caching discovery — remains deliberately deferred so each milestone stays single-
purpose. `route` and the auto-router share the same name-order selection, so the
inspection stays faithful to the dispatch.
