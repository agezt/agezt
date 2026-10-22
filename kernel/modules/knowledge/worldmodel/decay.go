// SPDX-License-Identifier: MIT

package worldmodel

import "github.com/agezt/agezt/kernel/event"

// DecayOptions tune Decay. Zero values fall back to sane defaults in Decay.
type DecayOptions struct {
	// StaleAfterMS: an entity not referenced (LastSeenMS) within this window
	// is considered stale and decayed. Default 14 days.
	StaleAfterMS int64
	// Factor multiplies a stale entity's weight (0<Factor<1). Default 0.8.
	Factor float64
	// Floor is the minimum weight decay will drive toward; weight never goes
	// below it (so a decayed entity still resolves, just ranks lower).
	// Default 0.1.
	Floor float64
}

const (
	defaultStaleAfterMS = int64(14 * 24 * 60 * 60 * 1000)
	defaultDecayFactor  = 0.8
	defaultDecayFloor   = 0.1
)

// Decay is the reflection loop's "world-model weights: decay unused" adjustment
// (SPEC-05 §6.3). It lowers the weight of active entities that haven't been
// referenced within the staleness window, floored so nothing drops out of
// resolve entirely — a stale project just ranks below the active ones. Each
// change is journaled as a worldmodel.entity.upserted with action "decay"
// (decay is just a downward reinforce — no new event kind), under corr, so
// `agt why` links the weight drop to the reflection that caused it.
//
// Decay only ever LOWERS weight (never raises) and never tombstones — it is a
// safe, reversible adjustment. Returns the number of entities decayed.
func (g *Graph) Decay(corr string, opts DecayOptions) (int, error) {
	staleMS := opts.StaleAfterMS
	if staleMS <= 0 {
		staleMS = defaultStaleAfterMS
	}
	factor := opts.Factor
	if factor <= 0 || factor >= 1 {
		factor = defaultDecayFactor
	}
	floor := opts.Floor
	if floor <= 0 {
		floor = defaultDecayFloor
	}

	// Decay is a read-all-then-write-each maintenance pass; hold the lock for the
	// whole pass so a concurrent Upsert/reinforce can't be clobbered by a write
	// computed from a now-stale snapshot (M421). The graph is bounded and decay is
	// periodic, so the coarse hold is acceptable per the store's write-volume contract.
	g.mu.Lock()
	defer g.mu.Unlock()

	all, err := g.store.AllEntities()
	if err != nil {
		return 0, err
	}
	nowMS := g.now().UnixMilli()
	decayed := 0
	for _, e := range all {
		if !e.Active() {
			continue
		}
		if nowMS-e.LastSeenMS < staleMS {
			continue // fresh enough; leave it (this is "strengthen active")
		}
		newWeight := e.Weight * factor
		if newWeight < floor {
			newWeight = floor
		}
		if newWeight >= e.Weight {
			continue // already at/below floor — nothing to do
		}
		old := e.Weight
		e.Weight = newWeight
		// Note: LastSeenMS is intentionally NOT refreshed — decay is not a
		// reference, so a still-unused entity keeps decaying on later passes.
		if err := g.store.PutEntity(e); err != nil {
			return decayed, err
		}
		g.publish(event.KindWorldEntityUpserted, corr, map[string]any{
			"action": "decay", "id": e.ID, "kind": string(e.Kind), "name": e.Name,
			"weight": e.Weight, "from_weight": old,
		})
		decayed++
	}
	return decayed, nil
}
