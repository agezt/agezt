# M200 — Bound the `agt peers` health response (mesh client surface)

## Why
`agt peers` (the M8 mesh health check) pings each configured peer's
`GET /api/v1/health` and decodes the reply:

```go
if err := json.NewDecoder(resp.Body).Decode(&body); err != nil { … }
```

There was no size cap on `resp.Body`. A legitimate health doc is a few dozen bytes
(`{"status":"ok","version":"…","model_count":N}`), but a hostile or misconfigured
peer — or a wrong URL pointing at something that streams forever — could feed an
unbounded body straight into the operator's CLI and exhaust its memory.

This is the same unbounded-read class already closed for the plugin host (M177),
the MCP bridge (M185), the control plane (M188), and the network-exposed HTTP APIs
(M198). It is notable here because the sibling `remote_run` tool in the **same peer
package** already bounds its peer responses (`io.ReadAll(io.LimitReader(resp.Body,
1<<20))`) — so `agt peers` was the lone inconsistent reader on the federation
surface. M200 closes that gap and completes the bounded-read guarantee on the mesh
client side.

## What
- `cmd/agt/peers.go`:
  - New `const maxPeerHealthBytes = 1 << 20` (1 MiB) — matches the `remote_run`
    tool's peer-response cap so the whole peer surface bounds reads at the same size.
  - `checkPeer` decodes through `io.LimitReader(resp.Body, maxPeerHealthBytes)`. An
    over-limit body is cut off; the truncated JSON fails to decode, so the peer is
    reported **unreachable** with a `bad health response: …` error rather than being
    ingested unbounded. (`io` was already imported.)

A health doc never approaches 1 MiB, so the cap never rejects a legitimate peer; it
only fences off a pathological one.

## Tests
`cmd/agt/peers_oversize_test.go`:
- `TestPeers_OversizedHealthBody` — a peer whose health body is a single valid-prefix
  JSON object with a multi-megabyte `version` string. A decoder reading the whole
  value would succeed and report the peer reachable with a giant version; with the
  cap the read stops mid-string and Decode errors, so the peer is reported
  **unreachable** with a `bad health response` error. The observable outcome
  (unreachable) is what distinguishes capped from uncapped — a precise, deterministic
  assertion that the limit is active.
- `TestPeers_NormalHealthBodyUnaffected` — an ordinary small health doc still parses
  and reports the peer reachable with the right version/model count (no regression).

Both drive the real `cmdPeers` against an `httptest` peer, exercising the actual
HTTP + decode path.

## Verification
- `go test ./...` — 1622 passing (1620 + 2 new), 0 failing.
- `go vet ./cmd/agt/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only: `io`).
- Local commit only (no push); standard trailer.

## Files
- `cmd/agt/peers.go` — `maxPeerHealthBytes` + `io.LimitReader` in `checkPeer`.
- `cmd/agt/peers_oversize_test.go` — new oversize + regression tests.

## Scope note
This bounds the operator-facing mesh **health** read. The `remote_run` task path was
already bounded. Both peer-facing reads now share the 1 MiB cap, so the federation
client surface is uniformly protected against an unbounded peer response.
