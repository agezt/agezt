# M548 — Pin the Slack replay-guard dedup eviction boundary

## Context
The Slack channel guards against event replay with a bounded "recently-seen keys"
set (`dedup`, cap 4096): a `channel:ts` key seen twice means a replayed delivery,
which must not drive a second agent run. The set is bounded so a hostile event
flood can't grow it without limit — it evicts the oldest key once the ring exceeds
capacity. `GOMAXPROCS=3`.

## The gap (closed)
```go
func (d *dedup) seenBefore(key string) bool {
	if _, ok := d.seen[key]; ok { return true }
	d.seen[key] = struct{}{}
	d.ring = append(d.ring, key)
	if len(d.ring) > d.cap {          // ← eviction boundary
		old := d.ring[0]
		d.ring = d.ring[1:]
		delete(d.seen, old)
	}
	return false
}
```
`TestSlack_ReplayDeduped` (integration) covers the "remembers a duplicate" half —
the same signed message twice runs the handler exactly once. But it never inserts
enough distinct keys to reach the eviction branch, so the bound itself was
unpinned: `len(d.ring) > d.cap → >= d.cap` (evict one insert too early, shrinking
the replay window) and evicting the wrong slot (`d.ring[0] → d.ring[1]`, which
deletes a still-windowed key from `seen` while dropping the genuinely-oldest from
`ring` — leaving the two out of sync) both survived.

## Fix
Added `TestSlack_DedupEvictsOldestPastCap` (unit, cap 3): a,b,c all new and `a`
still remembered at exactly cap; inserting `d` pushes one past cap → `a` (oldest)
is evicted from both `ring` and `seen` while `b`,`c` remain. Ordered so the
still-remembered keys are checked before the absent one (seenBefore records an
absent key, so checking `a`-evicted last avoids it evicting `b`).

## Negative control (manual, CPU-capped)
- `len(d.ring) > d.cap → >= d.cap`: FAIL (a evicted one insert early, no longer
  remembered at exactly cap).
- `old := d.ring[0] → d.ring[1]`: FAIL (b deleted from seen while a dropped from
  ring → b reported as unseen inside its window).
- Dropping `delete(d.seen, old)` does not compile (`old` unused) — not a viable
  mutant, so it cannot silently regress.
Source restored byte-for-byte (`git diff --ignore-all-space` empty).

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.
