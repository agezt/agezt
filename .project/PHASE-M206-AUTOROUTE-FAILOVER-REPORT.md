# M206 — `remote_run` auto-routing failover (node fault tolerance)

## Why
M203 made `remote_run` auto-route a task to a peer that serves a requested model, but
it picked exactly **one** peer. If that node was down, the whole delegation failed —
even when another configured peer served the same model. A federated mesh needs to
tolerate a single node's failure: route around it.

The hard part is doing this **without risking double execution**. A delegated task
runs through the peer's governed loop and may have side effects. If a peer accepted
the request and then reported a failure, re-running the task on another peer could
execute those side effects twice. So failover must be limited to the one case where
the task provably never ran: a **transport** failure (no HTTP response at all).

## What
`plugins/tools/peer/peer.go`:
- **`routeCandidates` / `serversForModel`** replace M203's single-peer `selectPeer` /
  `resolvePeerForModel`. For an auto-route they return **every** peer that serves the
  model, in name order (primary first, the rest fallbacks). Named-peer and sole-peer
  dispatch return a one-element list, so those paths are unchanged. The "no peer serves
  the model" error is byte-for-byte the same as M203.
- **`Invoke` now loops** over the candidates:
  - A `post` that returns a **transport error** (`err != nil` — no response) means the
    task never reached that peer, so the loop records it and tries the next candidate.
    If it was the last (or only) candidate, the error is surfaced: the original
    `remote_run: POST … failed: …` for a single peer, or
    `all N peers serving "<model>" unreachable (…)` for a multi-peer auto-route.
  - A peer that **responds** — success **or** an error status (non-2xx /
    `status:"failed"`) — ends the loop immediately: a success returns the answer, a
    failure is surfaced as-is. Such a peer is **never** retried elsewhere, because it
    may already have run side effects.

So failover happens only on unreachable nodes, never on nodes that ran the task and
failed. Discovery still benefits from the M205 cache; named dispatch still skips
discovery entirely.

## Tests (+4)
`plugins/tools/peer/peer_failover_test.go` (new `postByEndpoint` seam that fails by
host at the transport level):
- `TestAutoRoute_FailsOverToNextServer` — `alpha` (primary, both serve `opus`) fails its
  `/runs` POST at transport level → the task runs on `bravo`; the answer carries
  `peer=bravo` and the POST was attempted on `alpha` then `bravo`.
- `TestAutoRoute_AllServersUnreachable` — both serving peers unreachable → error names
  "all 2 peers serving" and both peers.
- `TestAutoRoute_DoesNotFailOverOnRunFailure` — `alpha` responds `502 status:failed`
  (it ran the task) → that failure is surfaced and `bravo` is **never** contacted
  (exactly one POST), proving no double-execution.
- `TestNamedPeer_TransportErrorUnchanged` — a single named peer failing at transport
  level keeps the original `remote_run: POST … failed` message.

All prior peer tests (auto-route, model pinning, discovery cache, named dispatch) pass
unchanged.

## Verification
- `go test ./...` — 1647 passing (1643 + 4 new), 0 failing.
- `go vet ./plugins/tools/peer/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only).
- Local commit only (no push); standard trailer.

## Files
- `plugins/tools/peer/peer.go` — `routeCandidates`, `serversForModel`, failover loop
  in `Invoke`.
- `plugins/tools/peer/peer_failover_test.go` — new failover/no-double-execute tests.

## Mesh thread (M8) so far
- **M200** bounded peer health read · **M201** `agt peers models` discovery ·
  **M202** `remote_run {model}` pinning · **M203** auto-route by model ·
  **M204** `agt peers route` inspector · **M205** discovery cache ·
  **M206** auto-route failover (this milestone).

## Scope note
Failover is deliberately transport-only (no response = safe to retry). It does **not**
retry idempotency-unknown failures, and it does not yet do load/cost-aware ordering
among serving peers — those remain deferred to keep this milestone single-purpose and
safe.
