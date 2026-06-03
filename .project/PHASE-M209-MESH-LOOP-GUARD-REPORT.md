# M209 — Mesh delegation loop guard (bounded cross-node hops)

## Why
The M8 mesh lets one node hand a task to a peer via `remote_run` → the peer's
`POST /api/v1/runs`. Nothing bounded the *chain*: node A delegates to B, B (while
running the delegated task) delegates back to A, A back to B… each hop a real
governed run that spends budget and never terminates. A misconfigured or adversarial
mesh — or simply two nodes that each name the other as a peer — could loop forever.
This is the central **safety** gap for a federated mesh, distinct from the failover
(M206) and discovery work.

A correct fix has to bound the chain end-to-end, and it must not change anything for
the common single-node / non-delegated case.

## What
A hop counter travels with each cross-node delegation, enforced at the receiving end.

- **New `kernel/meshctx` package** — carries the hop count through a run's
  `context.Context` (`WithHop` / `Hop`, default 0, negatives clamped), plus the shared
  `MaxHops = 8` bound and the `X-Agezt-Mesh-Hop` header name. It exploits the fact that
  the run context already threads from the REST handler all the way down to
  `tool.Invoke` (`r.Context()` → `RunModel` → `RunWith`/`WithTimeout` → agent loop
  `toolCtx := ctx` → `Invoke`), so a value set by the handler reaches the tool with no
  new plumbing.
- **`kernel/restapi` (`handleRunsRoot`)** — reads `X-Agezt-Mesh-Hop`. A run arriving
  with a hop **greater than `MaxHops`** is refused with **`508 Loop Detected`**
  (`mesh_hop_limit`) before it executes. Otherwise the inbound hop is stored in the run
  context (`r = r.WithContext(meshctx.WithHop(...))`), so it reaches both the sync and
  streaming run paths — and this node's own `remote_run`, if it fires, forwards hop+1.
- **`plugins/tools/peer` (`remote_run`)** —
  - `httpPost` sets `X-Agezt-Mesh-Hop: Hop(ctx)+1` on every delegation, so the receiving
    node can enforce the bound and the chain keeps incrementing.
  - `Invoke` refuses locally once the current run is already at `MaxHops` (delegating
    would be refused by the peer anyway) — a clear error instead of a doomed round-trip.

A run that did **not** arrive from a peer (local `agt run`, a schedule, a channel) has
no header → hop 0 → the chain starts fresh. Single-node operation is unchanged.

## Tests (+9)
- `kernel/meshctx/meshctx_test.go` — `Hop` default 0; `WithHop` round-trip and
  overwrite; negative clamps to 0.
- `kernel/restapi/mesh_hop_test.go` — over-limit hop (`MaxHops+1`) → `508` and the run
  does **not** execute; at-limit hop (`MaxHops`) still runs (it's the last allowed hop);
  no header → runs normally.
- `plugins/tools/peer/peer_hop_test.go` — `Invoke` at `MaxHops` refuses without
  POSTing; `httpPost` forwards `hop+1` (3 → "4") and from a non-delegated run (0 → "1"),
  asserted against a real `httptest` server.

All prior peer/restapi tests pass unchanged.

## Verification
- `go test ./...` — 1662 passing (1653 + 9 new), 0 failing.
- `go vet ./kernel/meshctx/ ./kernel/restapi/ ./plugins/tools/peer/` — clean.
- `gofmt -l` (CRLF-normalized) clean on all touched/new files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only: `context`, `strconv`).
- Local commit only (no push); standard trailer.

## Files
- `kernel/meshctx/meshctx.go` (+ test) — new hop-context package.
- `kernel/restapi/restapi.go` — `handleRunsRoot` hop read + `508` guard + context inject.
- `plugins/tools/peer/peer.go` — `Invoke` local guard + `httpPost` hop header.
- `kernel/restapi/mesh_hop_test.go`, `plugins/tools/peer/peer_hop_test.go` — new tests.

## Mesh thread (M8) so far
- **M200** bounded peer health read · **M201** `agt peers models` · **M202**
  `remote_run {model}` · **M203** auto-route by model · **M204** `agt peers route`
  inspector · **M205** discovery cache · **M206** auto-route failover · **M207**
  `agt doctor` mesh check · **M208** `agt status` mesh config · **M209** delegation loop
  guard (this milestone).

The mesh now has discovery, routing, failover, observability — and is bounded against
runaway federation loops. The bound is a hard ceiling (`MaxHops = 8`); making it
operator-tunable, and emitting a journal event when a loop is refused, are natural
follow-ons left deferred to keep this milestone single-purpose.
