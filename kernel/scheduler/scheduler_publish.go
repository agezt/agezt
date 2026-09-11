// SPDX-License-Identifier: MIT

// Scheduler event publishers + small helpers: publishPlanStarted/Completed/Failed + publishNodeStarted/Completed/Failed + nodeIDs + resultKeys + invariantSnapshot + setKeys + errorKeys.
// Code extracted from scheduler.go during the Day-64 god-file split. Public API unchanged.
package scheduler


import (
	"github.com/agezt/agezt/kernel/event"
	"maps"
	"sort"
)



func (e *Executor) publishPlanStarted(planID string, plan Plan) {
	if e.bus == nil {
		return
	}
	_, _ = e.bus.Publish(event.Spec{
		Subject:       "plan." + planID + ".lifecycle",
		Kind:          event.KindPlanStarted,
		Actor:         "scheduler",
		CorrelationID: planID,
		Payload: map[string]any{
			"plan_name":  plan.Name,
			"node_count": len(plan.Nodes),
			"node_ids":   nodeIDs(plan.Nodes),
		},
	})
}

func (e *Executor) publishPlanCompleted(planID string, plan Plan, res *PlanResult) {
	if e.bus == nil {
		return
	}
	_, _ = e.bus.Publish(event.Spec{
		Subject:       "plan." + planID + ".lifecycle",
		Kind:          event.KindPlanCompleted,
		Actor:         "scheduler",
		CorrelationID: planID,
		Payload: map[string]any{
			"plan_name":    plan.Name,
			"node_count":   len(plan.Nodes),
			"results_keys": resultKeys(res.NodeResults),
		},
	})
}

func (e *Executor) publishPlanFailed(planID string, plan Plan, res *PlanResult) {
	if e.bus == nil {
		return
	}
	failed := make([]string, 0, len(res.Errors))
	for id := range res.Errors {
		failed = append(failed, id)
	}
	sort.Strings(failed)
	_, _ = e.bus.Publish(event.Spec{
		Subject:       "plan." + planID + ".lifecycle",
		Kind:          event.KindPlanFailed,
		Actor:         "scheduler",
		CorrelationID: planID,
		Payload: map[string]any{
			"plan_name":  plan.Name,
			"failed_ids": failed,
		},
	})
}

func (e *Executor) publishNodeStarted(planID string, n Node) {
	if e.bus == nil {
		return
	}
	_, _ = e.bus.Publish(event.Spec{
		Subject:       "plan." + planID + ".node." + n.ID(),
		Kind:          event.KindNodeStarted,
		Actor:         "scheduler",
		CorrelationID: planID,
		Payload: map[string]any{
			"node_id":   n.ID(),
			"node_kind": string(n.Kind()),
			"deps":      n.DependsOn(),
		},
	})
}

func (e *Executor) publishNodeCompleted(planID string, n Node, r Result) {
	if e.bus == nil {
		return
	}
	payload := map[string]any{
		"node_id":      n.ID(),
		"node_kind":    string(n.Kind()),
		"output_bytes": len(r.Output),
	}
	maps.Copy(payload, r.Detail)
	_, _ = e.bus.Publish(event.Spec{
		Subject:       "plan." + planID + ".node." + n.ID(),
		Kind:          event.KindNodeCompleted,
		Actor:         "scheduler",
		CorrelationID: planID,
		Payload:       payload,
	})
}

func (e *Executor) publishNodeFailed(planID string, n Node, err error) {
	if e.bus == nil {
		return
	}
	_, _ = e.bus.Publish(event.Spec{
		Subject:       "plan." + planID + ".node." + n.ID(),
		Kind:          event.KindNodeFailed,
		Actor:         "scheduler",
		CorrelationID: planID,
		Payload: map[string]any{
			"node_id":   n.ID(),
			"node_kind": string(n.Kind()),
			"error":     err.Error(),
		},
	})
}

func nodeIDs(nodes []Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.ID()
	}
	sort.Strings(out)
	return out
}

func resultKeys(m map[string]Result) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func invariantSnapshot(planID string, plan Plan, phase InvariantPhase, nodeID string, started, completed map[string]struct{}, errs map[string]error) InvariantSnapshot {
	return InvariantSnapshot{
		PlanID:    planID,
		PlanName:  plan.Name,
		Phase:     phase,
		NodeID:    nodeID,
		Started:   setKeys(started),
		Completed: setKeys(completed),
		Failed:    errorKeys(errs),
	}
}

func setKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func errorKeys(m map[string]error) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}