// SPDX-License-Identifier: MIT

package workflow

// The workflow engine: the module's public surface for running, drafting and
// curating graphs.
//
// The engine holds no provider, no tool registry, no bus and no sandbox of
// its own. Every capability it needs arrives through Ports, so a run keeps
// the host's policy gates, its audit arc and its spend attribution — and the
// module stays free of any other module's internals (archcheck's principle
// P5).

import (
	"context"
	"encoding/json"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
)

// Ports are the host capabilities the engine calls out to. A nil port means
// the capability is unavailable: the engine then refuses the node that needs
// it (a code node without a sandbox, an llm node without a provider) instead
// of failing the whole daemon.
type Ports struct {
	// Publish journals one event (workflow.started, workflow.node, …).
	Publish func(event.Spec)
	// Complete runs one attributed completion through the host's Governor.
	Complete func(ctx context.Context, corr, taskType string, req llm.CompletionRequest) (*llm.CompletionResponse, error)
	// InvokeTool runs one governed tool call.
	InvokeTool func(ctx context.Context, corr, callID, toolName string, args json.RawMessage) (toolapi.Result, error)
	// RunCode runs one sandboxed script.
	RunCode func(ctx context.Context, corr, callPrefix, language, code, input string) (toolapi.Result, error)
	// ScriptRunnerReady reports whether the code sandbox exists.
	ScriptRunnerReady func() bool
	// Approve submits one operator gate and returns the decision.
	Approve func(ctx context.Context, spec approval.SubmitSpec) approval.Outcome
	// Model resolves the model a run should ask for (the live default plus
	// the run's own chain).
	Model func(ctx context.Context) string
	// DefaultModel is the daemon's live default model — what a draft follows
	// after the default is hot-swapped.
	DefaultModel func(ctx context.Context) string
	// HasProvider reports whether a provider is configured at all.
	HasProvider func() bool
	// Halted reports whether the host has stopped the kernel.
	Halted func() bool
}

// Provenance is why a run exists. The host fills it in from its own wake
// context — the module never reads another package's context keys.
type Provenance struct {
	Source            string
	Reason            string
	ScheduleID        string
	StandingID        string
	StandingName      string
	TriggerSubject    string
	ParentCorrelation string
}

// RunResult and NodeTestResult live in the contract layer
// (kernel/contract/workflowapi) — the agent-facing tool reports them, and a
// plugin may only name the contract. The module keeps their names as aliases
// (see aliases.go).

// Engine owns the workflow lifecycle for one store: the durable CRUD, the
// graph run, the single-node test and the copilot's draft.
type Engine struct {
	store *Store
	ports Ports
}

// NewEngine binds the engine to its store and the host's ports.
func NewEngine(store *Store, ports Ports) *Engine {
	return &Engine{store: store, ports: ports}
}

// Save upserts a workflow by name and journals workflow.saved with
// created/updated.
func (e *Engine) Save(corr string, w Workflow) (Workflow, bool, error) {
	saved, created, err := e.store.Save(w)
	if err != nil {
		return Workflow{}, false, err
	}
	action := "updated"
	if created {
		action = "created"
	}
	e.publish(event.Spec{
		Subject: "workflow." + saved.Name, Kind: event.KindWorkflowSaved, Actor: "workflow",
		CorrelationID: corr,
		Payload:       map[string]any{"id": saved.ID, "name": saved.Name, "action": action, "nodes": len(saved.Nodes), "edges": len(saved.Edges)},
	})
	return saved, created, nil
}

// Restore restores a checkpointed workflow while preserving its stored
// identity and enabled state, and journals workflow.restored.
func (e *Engine) Restore(corr string, w Workflow, reason string) (Workflow, bool, error) {
	restored, created, err := e.store.Restore(w)
	if err != nil {
		return Workflow{}, false, err
	}
	payload := map[string]any{"id": restored.ID, "name": restored.Name, "created": created, "nodes": len(restored.Nodes), "edges": len(restored.Edges)}
	if reason != "" {
		payload["reason"] = reason
	}
	e.publish(event.Spec{
		Subject: "workflow." + restored.Name, Kind: event.KindWorkflowRestored, Actor: "workflow",
		CorrelationID: corr,
		Payload:       payload,
	})
	return restored, created, nil
}

// SetEnabled arms/disarms a workflow's triggers and journals
// workflow.updated.
func (e *Engine) SetEnabled(corr, ref string, enabled bool) (Workflow, error) {
	w, err := e.store.SetEnabled(ref, enabled)
	if err != nil {
		return Workflow{}, err
	}
	e.publish(event.Spec{
		Subject: "workflow." + w.Name, Kind: event.KindWorkflowUpdated, Actor: "workflow",
		CorrelationID: corr,
		Payload:       map[string]any{"id": w.ID, "name": w.Name, "enabled": enabled},
	})
	return w, nil
}

// Remove deletes a workflow and journals workflow.removed when it existed.
func (e *Engine) Remove(corr, ref string) (bool, error) {
	gone, ok, err := e.store.Remove(ref)
	if err != nil {
		return false, err
	}
	if ok {
		e.publish(event.Spec{
			Subject: "workflow." + gone.Name, Kind: event.KindWorkflowRemoved, Actor: "workflow",
			CorrelationID: corr,
			Payload:       map[string]any{"id": gone.ID, "name": gone.Name},
		})
	}
	return ok, nil
}

// publish journals one event through the host's bus. A host without a bus
// loses the audit arc, never the run.
func (e *Engine) publish(spec event.Spec) {
	if e.ports.Publish == nil {
		return
	}
	e.ports.Publish(spec)
}
