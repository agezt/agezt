// SPDX-License-Identifier: MIT

// Provenance queries over the journal: correlation grouping (Why), the
// causation-ancestry walk (Causes), and the child->parent delegation link
// (ParentOf). Pure sequential Range scans; moved from kernel/runtime
// (Phase 3.1 C).

package journal

import (
	"encoding/json"
	"fmt"

	"github.com/agezt/agezt/kernel/event"
)

// Why returns every event with the same correlation_id as the named event,
// in seq order. It is the M0.5 form of `agt why` — useful to see what task
// produced an event. A richer tree-form view lands later.
func (j *Journal) Why(eventID string) ([]*event.Event, error) {
	var (
		target *event.Event
		all    []*event.Event
	)
	err := j.Range(func(e *event.Event) error {
		all = append(all, e)
		if e.ID == eventID {
			target = e
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, fmt.Errorf("journal: event %s not found", eventID)
	}
	if target.CorrelationID == "" {
		// Singleton event with no correlation — just return it.
		return []*event.Event{target}, nil
	}
	out := make([]*event.Event, 0, 16)
	for _, e := range all {
		if e.CorrelationID == target.CorrelationID {
			out = append(out, e)
		}
	}
	return out, nil
}

// Causes returns the causation ancestry of an event: the chain of events linked
// by causation_id from the root cause down to (and including) the target,
// ordered oldest-first (root → … → target). This is the provenance walk
// SPEC-01 §7.1 describes ("agt why walks the chain backwards"), and it is
// distinct from Why, which groups by correlation_id.
//
// Causation crosses correlation boundaries where correlation cannot: a Pulse
// delta/salience/initiative event carries its OWN per-chain correlation but
// links to the originating tick (a different correlation) only via causation_id
// — so the tick is reachable here yet invisible to Why. Likewise a channel
// reply links to the inbound message that caused it.
//
// The walk is cycle-guarded (a corrupt or forged journal could contain a
// causation loop; the runtime never emits one) and stops at the root
// (causation_id == "") or a dangling link (the referenced parent is absent).
// Returns the target alone when it has no causation parent.
func (j *Journal) Causes(eventID string) ([]*event.Event, error) {
	byID := make(map[string]*event.Event)
	var target *event.Event
	err := j.Range(func(e *event.Event) error {
		byID[e.ID] = e
		if e.ID == eventID {
			target = e
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, fmt.Errorf("journal: event %s not found", eventID)
	}
	// Walk causation_id backwards from the target, newest-first.
	chain := make([]*event.Event, 0, 8)
	seen := make(map[string]struct{})
	for cur := target; cur != nil; {
		if _, dup := seen[cur.ID]; dup {
			break // cycle guard: a forged causation loop must terminate
		}
		seen[cur.ID] = struct{}{}
		chain = append(chain, cur)
		if cur.CausationID == "" {
			break // reached the root cause
		}
		cur = byID[cur.CausationID] // nil if the parent is missing → walk ends
	}
	// Reverse in place to oldest-first (root → … → target).
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// ParentOf returns the lead run's correlation for a sub-agent run, or "" if
// childCorr was not spawned via delegation (M42). It scans the journal for a
// subagent.spawned event whose payload names childCorr as its child — the
// spawn lives under the PARENT correlation, so this is the only way to walk
// child→parent (the parent→child direction is already visible because the
// spawn is in the parent's own chain). A single forward scan; the last match
// wins (a child has exactly one spawn in practice).
func (j *Journal) ParentOf(childCorr string) string {
	if childCorr == "" {
		return ""
	}
	parent := ""
	_ = j.Range(func(e *event.Event) error {
		if e.Kind != event.KindSubAgentSpawned {
			return nil
		}
		var p struct {
			Child  string `json:"child_correlation"`
			Parent string `json:"parent"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return nil
		}
		if p.Child == childCorr && p.Parent != "" {
			parent = p.Parent
		}
		return nil
	})
	return parent
}
