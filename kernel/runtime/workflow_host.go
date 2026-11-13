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
//
// The engine's OPERATIONS are the module's (W3.76u): the daemon used to hold
// eight forwarders over them — `SaveWorkflow`, `RestoreWorkflow`,
// `SetWorkflowEnabled`, `RemoveWorkflow`, `RunWorkflow`, `TestWorkflowNode`,
// `DraftWorkflow` and `RefineWorkflow` — a second spelling of every verb the
// contract's engine port already names. The module's engine answers in the
// contract's names now, and the daemon keeps three things: the library's
// FILE, the engine's WIRING and the engine's FACE.

import (
	"context"
	"errors"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/workflowapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
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
			model, _ := runctx.ResolveModel(ctx, func() string { return k.EffectiveModel(ctx) })
			return model
		},
		DefaultModel: func(ctx context.Context) string { return k.effectiveConfig(ctx).Model },
		HasProvider:  func() bool { return k.cfg.Provider != nil },
		Halted: func() bool {
			k.registry.Lock()
			defer k.registry.Unlock()
			return k.registry.Halted()
		},
		// Provenance answers WHY a run exists: the host reads its own wake
		// context and hands the module the row (the module may not read
		// another package's context keys).
		Provenance: workflowProvenance,
	}
}

// SetWorkflowEngine injects the workflow engine (from cmd/agezt, built over
// the host's ports and the store the daemon opened).
func (k *Kernel) SetWorkflowEngine(e workflowapi.Engine) { k.wfEngine = e }

// WorkflowEngine returns the workflows module's engine — the module's FACE.
//
// It is the port the canvas, the agent-facing tool and the daemon's own
// trigger path drive, and it is nil when the daemon holds no engine: the
// callers refuse then, rather than calling into nothing.
//
// The daemon does not speak FOR the engine (W3.76u): it used to hold eight
// forwarders over the engine's own operations — a second spelling of every
// verb the contract's engine port already names. The engine answers in the
// contract's names now.
func (k *Kernel) WorkflowEngine() workflowapi.Engine { return k.wfEngine }

// workflowProvenance reads the host's wake context and hands the module the
// run's provenance. The module never sees another package's context keys.
func workflowProvenance(ctx context.Context) workflowapi.Provenance {
	w := runctx.WakeContextFromCtx(ctx)
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

// FireWorkflow runs a workflow that a trigger fired. The trigger's source
// rides the run's provenance, so a cron-triggered run is journalled as
// source:"cron" and an event-triggered one as source:"event" — never as
// "manual", which is what a trigger-less run is.
//
// It is the HOST's own path — the wake context is the host's — and it drives
// the module's engine.
func (k *Kernel) FireWorkflow(ctx context.Context, w workflowapi.Workflow, payload any, t workflowapi.Trigger) {
	rctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	rctx = runctx.WithWakeContext(rctx, runctx.WakeContext{
		Source:         t.Source,
		Reason:         t.Reason,
		TriggerSubject: t.Subject,
	})
	engine := k.WorkflowEngine()
	if engine == nil {
		return
	}
	_, _ = engine.Run(rctx, k.NewCorrelation(), w.Name, payload)
}
