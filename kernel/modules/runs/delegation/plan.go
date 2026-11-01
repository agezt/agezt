// SPDX-License-Identifier: MIT

package delegation

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/modules/runs/steer"
)

// Request is one delegation request: what to delegate, optionally as which
// named agent, on which model, and for which routing task type.
type Request struct {
	// Task is the complete, self-contained instruction for the sub-agent.
	Task string
	// Model overrides the daemon's default model for this delegation.
	Model string
	// TaskType selects the routing chain whose models provide the fallbacks.
	TaskType string
	// AgentRef is the roster slug of the agent to run AS.
	AgentRef string
}

// Plan is a fully resolved, guarded and journaled delegation: everything the
// child run needs, and nothing the child still has to decide. The daemon's
// prepareSubAgent produced the same thing as a private struct; it is
// exported now because the module's Prepare is the only writer.
type Plan struct {
	// ChildCtx is the child's context: depth bumped, actor and correlation
	// retargeted, root carried, agent profile attached.
	ChildCtx context.Context
	// ChildCorr is the child's own correlation id.
	ChildCorr string
	// ParentCorr is the spawning run's correlation id.
	ParentCorr string
	// RootCorr is the delegation tree's root correlation id.
	RootCorr string
	// LinkCorr is the correlation the spawn event is journaled under.
	LinkCorr string
	// Actor is the child's actor name.
	Actor string
	// Task is the delegation instruction.
	Task string
	// System is the child's system prompt: the sub-agent preamble plus the
	// named agent's soul, or the daemon's live default identity.
	System string
	// SubModel is the model the child runs on.
	SubModel string
	// ModelChain is the child's ordered fallback models.
	ModelChain []string
	// TaskType is the child's routing task type.
	TaskType string
	// MaxRunCost is the child's own per-run spend ceiling.
	MaxRunCost int64
	// AgentSlug is the named agent the child ran as ("" for an ad-hoc one).
	AgentSlug string
	// AgentDailyMc is the named agent's daily ceiling.
	AgentDailyMc int64
	// RetryPolicy is the named agent's retry policy (nil = no retries).
	RetryPolicy *agentapi.RetryPolicy
	// Steer is the child's live-steering control surface.
	Steer *steer.Control
}
