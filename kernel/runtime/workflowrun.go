// SPDX-License-Identifier: MIT

// Workflow kernel integration: Workflows accessor + CRUD (SaveWorkflow, RestoreWorkflow, SetWorkflowEnabled, RemoveWorkflow) + RunWorkflow entry + workflowRunProvenance + mergeWorkflowRunPayload.
// Code extracted from workflowrun.go during the Day-47 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/workflow"
)



// Workflows returns the durable workflow store (M798). Always non-nil after
// Open.
func (k *Kernel) Workflows() *workflow.Store { return k.workflows }

// SaveWorkflow validates and upserts a workflow (the canvas posts the whole
// graph), journaling workflow.saved with created/updated.
func (k *Kernel) SaveWorkflow(corr string, w workflow.Workflow) (workflow.Workflow, bool, error) {
	saved, created, err := k.workflows.Save(w)
	if err != nil {
		return workflow.Workflow{}, false, err
	}
	action := "updated"
	if created {
		action = "created"
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "workflow." + saved.Name, Kind: event.KindWorkflowSaved, Actor: "workflow",
		CorrelationID: corr,
		Payload:       map[string]any{"id": saved.ID, "name": saved.Name, "action": action, "nodes": len(saved.Nodes), "edges": len(saved.Edges)},
	})
	return saved, created, nil
}

// RestoreWorkflow restores a checkpointed workflow while preserving its stored
// identity and enabled state. It is the rollback counterpart to SaveWorkflow and
// journals workflow.restored.
func (k *Kernel) RestoreWorkflow(corr string, w workflow.Workflow, reason string) (workflow.Workflow, bool, error) {
	restored, created, err := k.workflows.Restore(w)
	if err != nil {
		return workflow.Workflow{}, false, err
	}
	payload := map[string]any{"id": restored.ID, "name": restored.Name, "created": created, "nodes": len(restored.Nodes), "edges": len(restored.Edges)}
	if reason != "" {
		payload["reason"] = reason
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "workflow." + restored.Name, Kind: event.KindWorkflowRestored, Actor: "workflow",
		CorrelationID: corr,
		Payload:       payload,
	})
	return restored, created, nil
}

// SetWorkflowEnabled arms/disarms a workflow's triggers, journaling
// workflow.updated.
func (k *Kernel) SetWorkflowEnabled(corr, ref string, enabled bool) (workflow.Workflow, error) {
	w, err := k.workflows.SetEnabled(ref, enabled)
	if err != nil {
		return workflow.Workflow{}, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "workflow." + w.Name, Kind: event.KindWorkflowUpdated, Actor: "workflow",
		CorrelationID: corr,
		Payload:       map[string]any{"id": w.ID, "name": w.Name, "enabled": enabled},
	})
	return w, nil
}

// RemoveWorkflow deletes a workflow, journaling workflow.removed when it
// existed.
func (k *Kernel) RemoveWorkflow(corr, ref string) (bool, error) {
	gone, ok, err := k.workflows.Remove(ref)
	if err != nil {
		return false, err
	}
	if ok {
		_, _ = k.bus.Publish(event.Spec{
			Subject: "workflow." + gone.Name, Kind: event.KindWorkflowRemoved, Actor: "workflow",
			CorrelationID: corr,
			Payload:       map[string]any{"id": gone.ID, "name": gone.Name},
		})
	}
	return ok, nil
}

// workflowStepCap is a defense-in-depth bound on executed steps per run —
// validation already rejects cycles, so this can only fire on an engine bug.
const workflowStepCap = 256

// RunWorkflowResult carries one run's outcome: per-node outputs (by node id)
// and the ordered list of executed node ids.
type RunWorkflowResult struct {
	Outputs  map[string]any
	Executed []string
}

// RunWorkflow executes one stored workflow under corr. payload becomes
// {{trigger.payload}}. Halted kernels refuse; the run respects ctx
// cancellation between and inside nodes. The first failing node fails the
// run (error branching arrives with M800).
func (k *Kernel) RunWorkflow(ctx context.Context, corr, ref string, payload any) (RunWorkflowResult, error) {
	k.runsMu.Lock()
	halted := k.halted
	k.runsMu.Unlock()
	if halted {
		return RunWorkflowResult{}, ErrHalted
	}
	w, found := k.workflows.Get(ref)
	if !found {
		return RunWorkflowResult{}, workflow.ErrNotFound
	}
	if err := workflow.Validate(w); err != nil { // defense: stores can predate rules
		return RunWorkflowResult{}, err
	}
	ctx = agent.WithCorrelation(ctx, corr)
	runMeta := workflowRunProvenance(ctx)

	_, _ = k.bus.Publish(event.Spec{
		Subject: "workflow." + w.Name, Kind: event.KindWorkflowStarted, Actor: "workflow",
		CorrelationID: corr,
		Payload:       mergeWorkflowRunPayload(map[string]any{"id": w.ID, "name": w.Name, "nodes": len(w.Nodes)}, runMeta),
	})

	res, err := k.runWorkflowGraph(ctx, corr, w, payload)
	if err != nil {
		_, _ = k.bus.Publish(event.Spec{
			Subject: "workflow." + w.Name, Kind: event.KindWorkflowFailed, Actor: "workflow",
			CorrelationID: corr,
			Payload:       mergeWorkflowRunPayload(map[string]any{"id": w.ID, "name": w.Name, "error": err.Error(), "executed": res.Executed}, runMeta),
		})
		return res, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "workflow." + w.Name, Kind: event.KindWorkflowCompleted, Actor: "workflow",
		CorrelationID: corr,
		Payload:       mergeWorkflowRunPayload(map[string]any{"id": w.ID, "name": w.Name, "executed": res.Executed}, runMeta),
	})
	return res, nil
}

func workflowRunProvenance(ctx context.Context) map[string]any {
	wake := wakeContextFromCtx(ctx)
	source := strings.TrimSpace(wake.Source)
	if source == "" {
		source = "manual"
	}
	agentSlug := strings.TrimSpace(agent.AgentFromContext(ctx))
	runner := source
	if agentSlug != "" {
		runner = "agent"
	}
	out := map[string]any{
		"source": source,
		"runner": runner,
	}
	if agentSlug != "" {
		out["agent"] = agentSlug
	}
	if wake.ScheduleID != "" {
		out["schedule_id"] = wake.ScheduleID
	}
	if wake.StandingID != "" {
		out["standing_id"] = wake.StandingID
	}
	if wake.StandingName != "" {
		out["standing_name"] = wake.StandingName
	}
	if wake.TriggerSubject != "" {
		out["trigger_subject"] = wake.TriggerSubject
	}
	if wake.ParentCorrelation != "" {
		out["parent_correlation_id"] = wake.ParentCorrelation
	}
	return out
}

func mergeWorkflowRunPayload(base, extra map[string]any) map[string]any {
	for k, v := range extra {
		base[k] = v
	}
	return base
}