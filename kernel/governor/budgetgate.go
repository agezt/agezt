// SPDX-License-Identifier: MIT

// The daily spend ceilings, as one shape. Split from governor.go (Phase 3.6).

package governor

import (
	"fmt"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)

// budgetScope is one daily spend ceiling. Three of them exist — the daemon-wide
// cap, the per-task-type cap (M1.zz), and a named agent's own cap (M793) — and
// they were three separate shapes: two returned (exceeded, spent, ceiling)
// triples that each caller unpacked and journaled itself, while the third was
// written inline in the middle of the pre-flight cascade. The result was three
// nearly-identical event payloads that had already drifted: the global one was
// the only one without a "scope" field, so a consumer filtering by scope
// silently saw no global-budget events at all.
//
// All three are the same decision: read today's spend for a scope under the
// rollover lock, compare it to that scope's ceiling, and on breach journal
// budget.exceeded and refuse. One shape, three instances.
type budgetScope struct {
	// Name is the scope's identity in the journal payload.
	Name string
	// Limits reports this scope's spend and ceiling for the request, and
	// whether the scope applies at all (a request with no TaskType has no
	// per-task ceiling; an unnamed agent has no agent ceiling). Reads happen
	// under g.mu after rollover so the decision is a consistent snapshot.
	Limits func(g *Governor, req *agent.CompletionRequest) (spent, ceiling int64, applies bool)
	// Detail adds the scope's identifying field to the journal payload
	// ("task_type", "agent"). Nil for the daemon-wide scope, which has none.
	Detail func(req *agent.CompletionRequest) (key, value string)
	// Err builds the refusal, wrapping this scope's sentinel error so callers
	// can still distinguish which ceiling stopped them.
	Err func(req *agent.CompletionRequest, spent, ceiling int64) error
}

// budgetScopes are checked in widening-to-narrowing order: the daemon-wide cap
// first (if the daemon is out of money, nothing else matters), then the task
// type's, then the individual agent's.
//
// Every one of these is a SOFT cap, not a reservation. The check and the later
// recordUsage are separate critical sections with the provider call between
// them, so N concurrent calls can all observe headroom, all proceed, and
// together overshoot by up to (N-1) calls' worth. That is the intended design
// for a $20/day-class cap with bounded per-call cost — reaffirmed 2026-06 — a
// hard cap would need a pessimistic pre-call cost estimate and would reject
// valid calls near the ceiling, which is the worse trade.
var budgetScopes = []budgetScope{
	{
		Name: "global",
		Limits: func(g *Governor, _ *agent.CompletionRequest) (int64, int64, bool) {
			g.mu.Lock()
			g.rolloverIfNeededLocked()
			ceiling := g.effectiveCeilingLocked()
			spent := g.spentToday.Load()
			g.mu.Unlock()
			return spent, ceiling, ceiling > 0
		},
		Err: func(_ *agent.CompletionRequest, spent, ceiling int64) error {
			return fmt.Errorf("%w (spent=%d, ceiling=%d microcents)", ErrBudgetExceeded, spent, ceiling)
		},
	},
	{
		Name: "task",
		Limits: func(g *Governor, req *agent.CompletionRequest) (int64, int64, bool) {
			if req.TaskType == "" || len(g.cfg.TaskBudgets) == 0 {
				return 0, 0, false
			}
			ceiling, ok := g.cfg.TaskBudgets[req.TaskType]
			if !ok || ceiling <= 0 {
				return 0, 0, false
			}
			g.mu.Lock()
			g.rolloverIfNeededLocked()
			spent := g.spentByTaskToday[req.TaskType]
			g.mu.Unlock()
			return spent, ceiling, true
		},
		Detail: func(req *agent.CompletionRequest) (string, string) { return "task_type", req.TaskType },
		Err: func(req *agent.CompletionRequest, spent, ceiling int64) error {
			return fmt.Errorf("%w (task=%s, spent=%d, ceiling=%d microcents)",
				ErrTaskBudgetExceeded, req.TaskType, spent, ceiling)
		},
	},
	{
		Name: "agent",
		Limits: func(g *Governor, req *agent.CompletionRequest) (int64, int64, bool) {
			if req.Agent == "" || req.AgentDailyCeilingMc <= 0 {
				return 0, 0, false
			}
			g.mu.Lock()
			g.rolloverIfNeededLocked()
			spent := g.spentByAgentToday[req.Agent]
			g.mu.Unlock()
			return spent, req.AgentDailyCeilingMc, true
		},
		Detail: func(req *agent.CompletionRequest) (string, string) { return "agent", req.Agent },
		Err: func(req *agent.CompletionRequest, spent, ceiling int64) error {
			return fmt.Errorf("%w (agent=%s, spent=%d, ceiling=%d microcents)",
				ErrAgentBudgetExceeded, req.Agent, spent, ceiling)
		},
	},
}

// evalBudgetScope is the single evaluation path for one ceiling: apply the
// scope's limits to the request and decide. A scope that does not apply reports
// "not exceeded" with a zero ceiling, which is what makes an unconfigured cap a
// no-op rather than a refusal.
func (g *Governor) evalBudgetScope(scope budgetScope, req *agent.CompletionRequest) (exceeded bool, spent, ceiling int64) {
	spent, ceiling, applies := scope.Limits(g, req)
	if !applies {
		return false, spent, 0
	}
	return spent >= ceiling, spent, ceiling
}

func budgetScopeNamed(name string) budgetScope {
	for _, s := range budgetScopes {
		if s.Name == name {
			return s
		}
	}
	panic("governor: unknown budget scope " + name) // table is a compile-time constant
}

// budgetExceeded reports the daemon-wide ceiling's state. Kept as a named
// helper because it is the boundary the budget tests drive directly — the
// decision itself lives in the table.
func (g *Governor) budgetExceeded() (bool, int64, int64) {
	return g.evalBudgetScope(budgetScopeNamed("global"), &agent.CompletionRequest{})
}

// taskBudgetExceeded reports the per-task-type ceiling's state (M1.zz).
// (false, 0, 0) when no cap is configured for that type.
func (g *Governor) taskBudgetExceeded(taskType string) (bool, int64, int64) {
	return g.evalBudgetScope(budgetScopeNamed("task"), &agent.CompletionRequest{TaskType: taskType})
}

// gateBudgets refuses the call if any applicable ceiling is already spent.
func (g *Governor) gateBudgets(req *agent.CompletionRequest) error {
	for _, scope := range budgetScopes {
		exceeded, spent, ceiling := g.evalBudgetScope(scope, req)
		if !exceeded {
			continue
		}
		payload := map[string]any{
			"scope":              scope.Name,
			"spent_microcents":   spent,
			"ceiling_microcents": ceiling,
		}
		if scope.Detail != nil {
			key, value := scope.Detail(req)
			payload[key] = value
		}
		g.publish(event.Spec{
			Subject:       "governor.budget",
			Kind:          event.KindBudgetExceeded,
			Actor:         "governor",
			CorrelationID: req.CorrelationID,
			Payload:       payload,
		})
		return scope.Err(req, spent, ceiling)
	}
	return nil
}
