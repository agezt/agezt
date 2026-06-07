# M541 — Verify the peer-delegation federation loop guard (client side)

## Context
The mesh federation loop guard (M209) bounds cross-node delegation chains so node A→B→A…
can't recurse forever, each hop a real governed run. It has two sides: the **server**
(restapi refuses an inbound run past the hop limit — pinned M513) and the **client**
(the `peer` remote_run tool refuses to delegate further and stamps hop+1 on forward). This
milestone verifies the client side by negative control. `GOMAXPROCS=3`.

## peer remote_run loop guard — verified solid
```
if maxHops := meshctx.MaxHopsFromEnv(); meshctx.Hop(ctx) >= maxHops { … refuse … }
…
req.Header.Set(meshctx.HopHeader, strconv.Itoa(meshctx.Hop(ctx)+1))   // forward hop+1
```

Negative control against `peer_hop_test.go` (`TestRemoteRun_RefusesAtHopLimit`):
- client guard `Hop(ctx) >= maxHops → >` — **killed** (a run exactly at the limit must
  refuse to delegate rather than make a doomed round-trip).
- forward increment `Hop(ctx)+1 → +0` (no increment) — **killed** (a non-incrementing hop
  never grows → the chain is unbounded → federation loop).
- forward increment `+1 → +2` — **killed** (over-increment would terminate chains early /
  inconsistently).

The two sides are consistent: the client refuses at `Hop >= maxHops`, so the deepest hop it
forwards is `maxHops` (from a run at `maxHops-1`), and the server accepts up to
`hopIn == maxHops` (refusing only `> maxHops`). No off-by-one between them.

## Verification / gate
- No code change; `go test ./plugins/tools/peer/` passes (`GOMAXPROCS=3`).
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Federation loop guard — complete (both sides)
restapi server refusal (M513, real gap closed) + peer client refusal & hop increment
(M541, verified solid) + meshctx config parser (M521). The cross-node delegation runaway
protection is verified end to end.
