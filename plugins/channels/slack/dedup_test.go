// SPDX-License-Identifier: MIT

package slack

import "testing"

// The replay-guard dedup is a bounded set: it must remember a key while it's
// inside the window (so a replayed event is dropped) AND evict the oldest key
// once the ring exceeds its capacity (so a hostile flood can't grow it without
// bound). TestSlack_ReplayDeduped covers the "remembers a duplicate" half; this
// pins the eviction boundary, which the integration test never reaches.
//
// NOTE seenBefore is not a pure query: checking an ABSENT key records it (and may
// itself evict). So the still-remembered keys are checked before the absent one.
//
// With cap 3 and keys a,b,c,d inserted in order:
//   - a,b,c are all new (ring == cap, no eviction yet) and a is still remembered;
//   - inserting d pushes len past cap → the OLDEST (a) is evicted from both ring
//     and seen, so a is treated as new again, while b/c remain remembered.
//
// This kills: the boundary `len(ring) > cap → >= cap` (would evict a one insert
// too early, so a wouldn't be remembered after a,b,c), dropping the `delete(seen,
// old)` (a would stay remembered after eviction), and evicting the wrong slot
// (`ring[0] → ring[1]`, which would forget b instead of a).
func TestSlack_DedupEvictsOldestPastCap(t *testing.T) {
	d := newDedup(3)

	for _, k := range []string{"a", "b", "c"} {
		if d.seenBefore(k) {
			t.Fatalf("%q reported as seen on first insert", k)
		}
	}
	// At exactly cap, nothing has been evicted yet: the oldest key is still known.
	// (a is present, so this check does not mutate the ring.)
	if !d.seenBefore("a") {
		t.Error("at exactly cap, the oldest key (a) must still be remembered (evicted too early)")
	}

	// Insert a genuinely new key to push the ring one past cap and evict the oldest (a).
	if d.seenBefore("d") {
		t.Fatal("d reported as seen on first insert")
	}
	// b and c are still inside the window and must remain remembered. Check these
	// first — they don't mutate the ring (both present).
	for _, k := range []string{"b", "c"} {
		if !d.seenBefore(k) {
			t.Errorf("%q was inside the window but was evicted/forgotten", k)
		}
	}
	// a was the oldest distinct key — it must now be forgotten (evicted). Checked
	// last, because as an absent key this records a and would evict b.
	if d.seenBefore("a") {
		t.Error("after exceeding cap, the oldest key (a) must have been evicted, but it's still remembered")
	}
}
