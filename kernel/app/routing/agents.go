// SPDX-License-Identifier: MIT

package routing

import "github.com/agezt/agezt/kernel/contract/agentapi"

// Agents maps the roster's profiles into the routing view's agents: the
// handle the operator addresses, the model the agent runs and its ordered
// fallback chain.
//
// The conversion belongs here, beside the row type: the host reads the
// roster's profiles and hands them over, and the application's own row
// stays the application's business.
func Agents(profiles []agentapi.Profile) []Agent {
	agents := make([]Agent, 0, len(profiles))
	for _, p := range profiles {
		agents = append(agents, Agent{Slug: p.Slug, Model: p.Model, Fallbacks: p.Fallbacks})
	}
	return agents
}
