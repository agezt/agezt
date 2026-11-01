// SPDX-License-Identifier: MIT

package delegation

import (
	"github.com/agezt/agezt/kernel/event"
)

// spendMicrocents sums the spend (budget.consumed cost_microcents, M47) of
// every run descended from parentCorr — its sub-agents and their sub-agents,
// transitively — excluding parentCorr's own direct spend. It backs the M48
// spend cap: a single forward journal pass builds the parent→children links
// (from subagent.spawned) and the per-run spend (from budget.consumed), then
// totals the spend over parentCorr's transitive descendants. Stateless and
// race-free: every prior delegation's spend is already durably journaled by
// the time the next delegate calls this. Only invoked when the cap is enabled.
//
// The spend RULE moved here with the rest of the delegation family in
// W3.76f: a budget rule belongs to the delegation engine, not to the daemon.
// The JOURNAL it reads stays the daemon's — the module asks for it through
// the host's RangeJournal door.
func (s *Service) spendMicrocents(parentCorr string) int64 {
	childrenOf := map[string][]string{}
	spendOf := map[string]int64{}
	_ = s.host.RangeJournal(func(e *event.Event) error {
		switch e.Kind {
		case event.KindSubAgentSpawned:
			if child, parent := SpawnLink(e.Payload); child != "" && parent != "" {
				childrenOf[parent] = append(childrenOf[parent], child)
			}
		case event.KindBudgetConsumed:
			if e.CorrelationID != "" {
				spendOf[e.CorrelationID] += BudgetCostMicrocents(e.Payload)
			}
		}
		return nil
	})

	// Sum spend over the transitive descendants of parentCorr (BFS over the
	// links), excluding parentCorr itself — the cap bounds sub-agent spend, not
	// the lead's own. A `seen` set guards against a malformed cyclic link.
	var total int64
	seen := map[string]bool{parentCorr: true}
	queue := append([]string{}, childrenOf[parentCorr]...)
	head := 0
	for head < len(queue) {
		corr := queue[head]
		head++
		if seen[corr] {
			continue
		}
		seen[corr] = true
		total += spendOf[corr]
		queue = append(queue, childrenOf[corr]...)
	}
	return total
}
