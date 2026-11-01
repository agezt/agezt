// SPDX-License-Identifier: MIT

package runtime

import (
	"github.com/agezt/agezt/kernel/delegation"
	"github.com/agezt/agezt/kernel/event"
)

// subAgentSpendMicrocents sums the spend (budget.consumed cost_microcents, M47)
// of every run descended from parentCorr — its sub-agents and their sub-agents,
// transitively — excluding parentCorr's own direct spend. It backs the M48
// spend cap: a single forward journal pass builds the parent→children links
// (from subagent.spawned) and the per-run spend (from budget.consumed), then
// totals the spend over parentCorr's transitive descendants. Stateless and
// race-free: every prior delegation's spend is already durably journaled by the
// time the next delegate calls this. Only invoked when the cap is enabled.
//
// The sub-agent's PROMPT half used to live beside this one
// (subAgentInjectedSystem); it moved to the runs module's prompt service in
// W3.76e. What stayed is the BUDGET rule, and a budget rule belongs to the
// delegation family — not to the prompt.
func (k *Kernel) subAgentSpendMicrocents(parentCorr string) int64 {
	childrenOf := map[string][]string{}
	spendOf := map[string]int64{}
	_ = k.journal.Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindSubAgentSpawned:
			if child, parent := delegation.SpawnLink(e.Payload); child != "" && parent != "" {
				childrenOf[parent] = append(childrenOf[parent], child)
			}
		case event.KindBudgetConsumed:
			if e.CorrelationID != "" {
				spendOf[e.CorrelationID] += delegation.BudgetCostMicrocents(e.Payload)
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
