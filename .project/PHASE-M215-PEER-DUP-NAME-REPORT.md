# M215 — Reject a duplicate peer name in `AGEZT_PEERS`

## Why
`ParsePeers` builds the peer set as a `map[string]Peer` keyed by name, assigning
`peers[name] = …` for each entry with no collision check. So a spec like

    AGEZT_PEERS="a=http://x:1,b=http://y:2,a=http://z:3"

silently parses to **two** peers — the second `a` overwrites the first — when the
operator clearly intended three. Two concrete failures follow: the mesh quietly has one
fewer node than configured (capacity/failover you think you have, you don't), and a
`remote_run` naming `a` hits whichever URL won the overwrite, not the one the operator
expects. Every other malformed-spec case in `ParsePeers` is a hard startup error; a
duplicate name should be too, rather than a silent routing bug discovered much later.

## What
`plugins/tools/peer/peer.go` (`ParsePeers`):
- Before inserting a parsed entry, check whether the name is already present; if so,
  return `peer "<name>" is defined more than once`. Because the daemon's boot path
  (`cmd/agezt/main.go`) and `agt peers` / `agt doctor` all call `ParsePeers`, the
  misconfiguration now surfaces immediately — a hard startup error for the daemon, a
  clear message for the CLI — instead of becoming a silent shadowed-peer bug.
- Distinct names that happen to share a URL remain valid (two labels for one node is
  unusual but not wrong); only a repeated *name* (the routing key) is rejected.

## Tests (+1)
`plugins/tools/peer/peer_test.go`:
- `TestParsePeers_RejectsDuplicateName` — `a=…,b=…,a=…` → error naming `a` and "more than
  once"; and `a=http://x:1,b=http://x:1` (distinct names, same URL) still parses cleanly.

The existing `TestParsePeers` (valid spec, missing `name=`, non-http URL, empty spec)
remains and passes.

## Verification
- `go test ./...` — 1680 passing (1679 + 1 new), 0 failing.
- `go vet ./plugins/tools/peer/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only).
- Local commit only (no push); standard trailer.

## Files
- `plugins/tools/peer/peer.go` — duplicate-name guard in `ParsePeers`.
- `plugins/tools/peer/peer_test.go` — duplicate-name test.

## Mesh thread (M8) so far
- **M200**–**M214** (discovery, routing, failover, cache, loop-guard with audit /
  tunability / validation / auth posture, doctor + status integration).
- **M215** — reject a duplicate peer name (this milestone): config hygiene closing a
  silent mesh-node-loss bug.

The mesh's config surface (`AGEZT_PEERS`) is now strict: bad URL, missing `name=`, and a
duplicate name are all caught at startup. Larger remaining items (per-tenant peer sets,
load/cost-aware routing, refused-loop count surfacing) stay separately scoped.
