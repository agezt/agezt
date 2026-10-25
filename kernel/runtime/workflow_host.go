// SPDX-License-Identifier: MIT

package runtime

// The runtime's face of the workflows module. The module owns the durable
// store, the graph engine, the trigger runner and the copilot; this file
// keeps the host's public names, hands the module the ports it needs and
// forwards the calls.
//
// A module may not reach into another module's internals (archcheck's
// principle P5), so the module never imports the runtime: the runtime maps
// its own wake context onto the module's provenance and lets the module
// publish through the host's bus.
//
// The host holds PORTS (W3.48): the store it reads (the contract's read
// side) and the engine it drives (the contract's eight-verb engine port).
// The engine itself is built by the composition root — over the store and
// over the host's own ports — because a module's implementation is not the
// host's to name.

import (
	"context"
	"errors"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/workflowapi"
	"github.com/agezt/agezt/kernel/event"
)

// errNoWorkflowEngine is the answer a kernel gives when it holds no workflow
// engine.
//
// The engine used to be built by the kernel, so the field was never nil and
// the code never had to say what happens without one. Now the engine is the
// host's (W3.48) and a host may open a kernel without one — and the answer is
// a refusal, not a panic.
var errNoWorkflowEngine = errors.New("runtime: the workflow engine is not available on this daemon (no workflow engine)")

// The results the host and the module both name are declared once, in the
// contract layer. The runtime keeps its own names as aliases, so every
// importer compiles unchanged while the implementation lives in the module.
type (
	// RunWorkflowResult carries one run's outcome: per-node outputs (by node
	// id) and the ordered list of executed node ids.
	RunWorkflowResult = workflowapi.RunResult
	// NodeTestResult is one tested node's outcome.
	NodeTestResult = workflowapi.NodeTestResult
)

// Workflows returns the durable workflow store (M798) — the contract's read
// side, so the host and the agent-facing tool read the same rows.
func (k *Kernel) Workflows() workflowapi.Store { return k.workflows }

// WorkflowPorts are the host capabilities the module calls out to. Every
// one of them is the host's own governed path — the module holds no
// provider, no tool registry and no bus of its own, so a workflow run keeps
// the host's policy checks, its audit arc and its spend attribution.
//
// The composition root builds the engine over these: the engine needs the
// host's capabilities, and only the host has them.
func (k *Kernel) WorkflowPorts() workflowapi.Ports {
	return workflowapi.Ports{
		Publish: func(spec event.Spec) {
			if k.bus == nil {
				return
			}
			_, _ = k.bus.Publish(spec)
		},
		Complete:   k.completeAux,
		InvokeTool: k.RunTool,
		RunCode: func(ctx context.Context, corr, callPrefix, language, code, input string) (toolapi.Result, error) {
			runner := k.cfg.ScriptRunner
			if runner == nil {
				return toolapi.Result{}, errors.New("runtime: no code sandbox")
			}
			res, _, err := k.runCode(ctx, corr, callPrefix, runner, language, code, input)
			return res, err
		},
		ScriptRunnerReady: func() bool { return k.cfg.ScriptRunner != nil },
		Approve: func(ctx context.Context, spec approval.SubmitSpec) approval.Outcome {
			return k.approvals.Submit(ctx, spec)
		},
		Model: func(ctx context.Context) string {
			model, _ := resolveRunModel(ctx, k.effectiveConfig(ctx))
			return model
		},
		DefaultModel: func(ctx context.Context) string { return k.effectiveConfig(ctx).Model },
		HasProvider:  func() bool { return k.cfg.Provider != nil },
		Halted: func() bool {
			k.runsMu.Lock()
			defer k.runsMu.Unlock()
			return k.halted
		},
	}
}

// SetWorkflowEngine injects the workflow engine (from cmd/agezt, built over
// the host's ports and the store the daemon opened).
func (k *Kernel) SetWorkflowEngine(e workflowapi.Engine) { k.wfEngine = e }

// workflowEngine returns the engine port, or the refusal.
func (k *Kernel) workflowEngine() (workflowapi.Engine, error) {
	if k.wfEngine == nil {
		return nil, errNoWorkflowEngine
	}
	return k.wfEngine, nil
}

// workflowProvenance reads the host's wake context and hands the module the
// run's provenance. The module never sees another package's context keys.
func workflowProvenance(ctx context.Context) workflowapi.Provenance {
	w := wakeContextFromCtx(ctx)
	return workflowapi.Provenance{
		Source:            w.Source,
		Reason:            w.Reason,
		ScheduleID:        w.ScheduleID,
		StandingID:        w.StandingID,
		StandingName:      w.StandingName,
		TriggerSubject:    w.TriggerSubject,
		ParentCorrelation: w.ParentCorrelation,
	}
}

// SaveWorkflow validates and upserts a workflow (the canvas posts the whole
// graph), journaling workflow.saved with created/updated.
func (k *Kernel) SaveWorkflow(corr string, w workflowapi.Workflow) (workflowapi.Workflow, bool, error) {
	engine, err := k.workflowEngine()
	if err != nil {
		return workflowapi.Workflow{}, false, err
	}
	return engine.Save(corr, w)
}

// RestoreWorkflow restores a checkpointed workflow while preserving its
// stored identity and enabled state. It is the rollback counterpart to
// SaveWorkflow and journals workflow.restored.
func (k *Kernel) RestoreWorkflow(corr string, w workflowapi.Workflow, reason string) (workflowapi.Workflow, bool, error) {
	engine, err := k.workflowEngine()
	if err != nil {
		return workflowapi.Workflow{}, false, err
	}
	return engine.Restore(corr, w, reason)
}

// SetWorkflowEnabled arms/disarms a workflow's triggers, journaling
// workflow.updated.
func (k *Kernel) SetWorkflowEnabled(corr, ref string, enabled bool) (workflowapi.Workflow, error) {
	engine, err := k.workflowEngine()
	if err != nil {
		return workflowapi.Workflow{}, err
	}
	return engine.SetEnabled(corr, ref, enabled)
}

// RemoveWorkflow deletes a workflow, journaling workflow.removed when it
// existed.
func (k *Kernel) RemoveWorkflow(corr, ref string) (bool, error) {
	engine, err := k.workflowEngine()
	if err != nil {
		return false, err
	}
	return engine.Remove(corr, ref)
}

// RunWorkflow executes one stored workflow under corr. payload becomes
// {{trigger.payload}}. Halted kernels refuse; the run respects ctx
// cancellation between and inside nodes. The first failing node fails the
// run (error branching arrives with M800).
func (k *Kernel) RunWorkflow(ctx context.Context, corr, ref string, payload any) (RunWorkflowResult, error) {
	engine, err := k.workflowEngine()
	if err != nil {
		return RunWorkflowResult{}, err
	}
	res, err := engine.Run(ctx, corr, ref, payload, workflowProvenance(ctx))
	if errors.Is(err, workflowapi.ErrHalted) {
		return RunWorkflowResult{}, ErrHalted
	}
	return res, err
}

// TestWorkflowNode executes one node of w against the supplied data map
// (keys are upstream node ids, values their {"output": …} shape; the
// "trigger" entry is derived from payload). The graph must validate — a
// half-edited canvas is refused with the validator's reason, not run on a
// guess. Halted kernels refuse.
func (k *Kernel) TestWorkflowNode(ctx context.Context, corr string, w workflowapi.Workflow, nodeID string, data map[string]any, payload any) (NodeTestResult, error) {
	engine, err := k.workflowEngine()
	if err != nil {
		return NodeTestResult{}, err
	}
	res, err := engine.TestNode(ctx, corr, w, nodeID, data, payload)
	if errors.Is(err, workflowapi.ErrHalted) {
		return NodeTestResult{}, ErrHalted
	}
	return res, err
}

// DraftWorkflow asks the configured provider to design a workflow from a
// plain-language description. name, when non-empty, overrides whatever the
// model chose (the canvas knows the workflow it's editing). The returned
// workflow is validated and auto-laid-out but NOT saved.
func (k *Kernel) DraftWorkflow(ctx context.Context, corr, name, description string) (workflowapi.Workflow, error) {
	engine, err := k.workflowEngine()
	if err != nil {
		return workflowapi.Workflow{}, err
	}
	return engine.Draft(ctx, corr, name, description)
}

// RefineWorkflow (M805) revises an existing graph from a plain-language
// instruction: the provider sees the CURRENT graph JSON (the canvas's truth,
// unsaved edits included) plus the change request, and answers with the full
// revised graph under the same contract. The base's name is preserved and
// the result is returned UNSAVED, exactly like a fresh draft.
func (k *Kernel) RefineWorkflow(ctx context.Context, corr string, base workflowapi.Workflow, instruction string) (workflowapi.Workflow, error) {
	engine, err := k.workflowEngine()
	if err != nil {
		return workflowapi.Workflow{}, err
	}
	return engine.Refine(ctx, corr, base, instruction)
}

// FireWorkflow runs a workflow that a trigger fired. The trigger's source
// rides the run's provenance, so a cron-triggered run is journalled as
// source:"cron" and an event-triggered one as source:"event" — never as
// "manual", which is what a trigger-less run is.
func (k *Kernel) FireWorkflow(ctx context.Context, w workflowapi.Workflow, payload any, t workflowapi.Trigger) {
	rctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	rctx = WithWakeContext(rctx, WakeContext{
		Source:         t.Source,
		Reason:         t.Reason,
		TriggerSubject: t.Subject,
	})
	_, _ = k.RunWorkflow(rctx, k.NewCorrelation(), w.Name, payload)
}
