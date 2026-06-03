# M205 — `remote_run` auto-routing discovery cache (bounded TTL)

## Why
M203 made `remote_run` auto-route a task to a peer that serves a requested model by
probing each candidate peer's `GET /api/v1/models`. But it re-probed on **every**
invocation: an agent loop delegating many tasks to the mesh would fan out a fresh
discovery request to (up to) every peer per call. Model inventories change rarely, so
that repeated probing is wasted latency and load on the mesh — a real cost once
auto-routing is used in anger.

## What
A bounded-TTL cache for the auto-routing model discovery, on the `Tool`:

- **`cachedModels(ctx, p)`** wraps the `lister`: within the cache TTL it returns the
  peer's last discovered model list; otherwise it probes and stores the result.
  `resolvePeerForModel` now calls `cachedModels` instead of `list` directly.
- **TTL** is `Tool.CacheTTL`, defaulting to **`DefaultCacheTTL` = 60s** when unset.
  Short enough that a model added/removed on a peer is picked up quickly; long enough
  that a burst of auto-routes costs at most one probe per peer.
- **Errors are not cached** — a transient discovery failure (unreachable peer) returns
  the error without storing it, so the next route re-probes and can recover.
- **Concurrency**: the cache is guarded by a `sync.Mutex`, but the network `list` call
  runs **outside** the lock (read-check-unlock, fetch, relock-store), so concurrent
  discoveries of different peers don't serialize and the lock is never held across I/O.
  The map is lazily initialized so directly-constructed `Tool` literals (as in tests)
  work without `New`.
- **Clock seam**: an injectable `now func() time.Time` (nil → `time.Now`) makes TTL
  expiry deterministically testable.

A **named-peer** dispatch still skips discovery entirely (so the cache is irrelevant
there), and the single-peer path is unchanged. The only behavioural change is that an
auto-route may reuse a model list up to `CacheTTL` old.

## Tests (+2)
`plugins/tools/peer/peer_cache_test.go` (injected clock + call-counting lister):
- `TestAutoRoute_CachesDiscovery` — first auto-route probes both peers (2 calls); a
  second within the TTL adds **no** calls (served from cache); advancing the clock past
  the TTL re-probes both (4 total). Proves both the hit and the expiry.
- `TestAutoRoute_DoesNotCacheErrors` — an unreachable peer (`alpha`) errors on every
  route while a reachable one (`bravo`) is cached after the first: two routes yield 1
  `bravo` call + 2 `alpha` calls = 3, proving errors aren't cached.

All prior peer tests (named dispatch, model pinning, auto-routing, single-peer) pass
unchanged — the cache is transparent to them.

## Verification
- `go test ./...` — 1643 passing (1641 + 2 new), 0 failing.
- `go vet ./plugins/tools/peer/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only: `sync`).
- `-race` not run (the toolchain here has no cgo); the cache uses the conventional
  mutex pattern with no I/O under the lock.
- Local commit only (no push); standard trailer.

## Files
- `plugins/tools/peer/peer.go` — `DefaultCacheTTL`, `modelCacheEntry`, cache fields,
  `clock`, `cachedModels`; `resolvePeerForModel` uses the cache.
- `plugins/tools/peer/peer_cache_test.go` — new cache hit/expiry/error tests.

## Mesh thread (M8) so far
- **M200** — bounded the `agt peers` health read.
- **M201** — `agt peers models` discovery.
- **M202** — `remote_run {model}` model pinning.
- **M203** — `remote_run` auto-routes by model.
- **M204** — `agt peers route <model>` routing inspector.
- **M205** — auto-routing discovery cache (this milestone).

## Scope note
The cache makes repeated auto-routing cheap. Remaining deferred follow-ons —
load/cost-aware selection among multiple servers, and sharing the cache with the
`agt peers route` inspector — stay out of scope to keep this milestone single-purpose.
The 60s TTL is a deliberate, conservative default; it is a `Tool` field so an embedder
can tune it.
