# M203 — `remote_run` automatic peer routing by model (cross-node routing)

## Why
M201 lets an operator *discover* which peer serves a model (`agt peers models`); M202
lets a caller *pin* a model when delegating to a *named* peer. The missing piece was
**automatic cross-node routing**: an agent that wants a task run on model `opus` still
had to know which node has `opus` and name it. With no peer named, `remote_run` simply
errored ("a peer name is required").

M203 closes that: when a `model` is requested but no `peer` is named, the tool picks a
peer that serves the model itself. This is the routing layer the ROADMAP's federated
mesh (M8) needs — the delegating node routes by capability, not by hard-coded node name.

## What
`plugins/tools/peer/peer.go`:
- **`selectPeer(ctx, name, model)`** decides the target:
  - a **named** peer is used as-is (discovery is skipped entirely);
  - **no name + a model + >1 peer** → auto-route via `resolvePeerForModel`;
  - otherwise → the existing `resolve` (sole-peer / ambiguous-name rules). The
    single-peer and named-peer paths are byte-for-byte unchanged.
- **`resolvePeerForModel(ctx, model)`** queries peers **in name order** (deterministic)
  and returns the first that lists the model. A peer that can't be reached for
  discovery is recorded and skipped, not fatal. If none serve the model, the error
  names the peers checked and any that were unreachable.
- **`lister`** — a new injectable seam (`func(ctx, Peer) ([]string, error)`) for the
  model-discovery call, mirroring the existing `poster`. Defaulted in `New` to
  **`httpListModels`**: `GET {url}/api/v1/models` with the peer's bearer token, a
  bounded-read (1 MiB) decode, returning the `models` array. Discovery runs under the
  same bounded context as the run (the 5-minute default timeout is applied before
  selection, so discovery + dispatch share one deadline).
- **Tool description** updated: "if you set `model` but omit `peer`, a peer that serves
  that model is chosen automatically."

The peer still runs the delegated task through its own governed loop; routing only
chooses *where*, never bypassing the peer's Edict/journal/governor.

## Tests (+5)
`plugins/tools/peer/peer_route_test.go` (new `fakeList` seam + a two-peer fixture):
- `TestRemoteRun_AutoRoutesByModel` — model `opus` served only by `bravo` → the POST
  hits `bravo`'s `/api/v1/runs` with `"model":"opus"`.
- `TestRemoteRun_AutoRouteNoPeerServesModel` — an unserved model → error naming the
  model and the peers checked.
- `TestRemoteRun_AutoRouteDeterministic` — both peers serve the model → sorted-first
  (`alpha`) is chosen.
- `TestRemoteRun_AutoRouteSkipsUnreachable` — `alpha` (first) fails discovery, `bravo`
  serves the model → routes to `bravo` without error.
- `TestRemoteRun_NamedPeerSkipsDiscovery` — naming a peer makes zero lister calls
  (discovery bypassed) and dispatches to the named peer.

All prior peer tests (named dispatch, single-peer default, ambiguous-name error, model
pinning) still pass unchanged.

## Verification
- `go test ./...` — 1636 passing (1631 + 5 new), 0 failing.
- `go vet ./plugins/tools/peer/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only).
- Local commit only (no push); standard trailer.

## Files
- `plugins/tools/peer/peer.go` — `lister`, `selectPeer`, `resolvePeerForModel`,
  `httpListModels`, description update.
- `plugins/tools/peer/peer_route_test.go` — new auto-routing tests.

## Mesh thread (M8) so far
- **M200** — bounded the `agt peers` health read (federation client hardening).
- **M201** — `agt peers models` peer model discovery.
- **M202** — `remote_run {model}` model pinning on a named peer.
- **M203** — `remote_run` auto-routes by model (this milestone).

## Scope note
Routing picks the first serving peer deterministically. Load-aware or cost-aware
selection across multiple serving peers, and caching discovery results to avoid a
per-call round-trip, are deliberately left to future milestones to keep this one
single-purpose. The round-trip cost is bounded by the run timeout and only paid on the
auto-route path (a named peer skips it).
