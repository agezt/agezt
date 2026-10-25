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
	"github.com/agezt/agezt/kernel/contract/workflowapi"
	"github.com/agezt/agezt/kernel/event"
)

// Ports are the host capabilities the engine calls out to. A nil port means
// the capability is unavailable: the engine then refuses the node that needs
// it (a code node without a sandbox, an llm node without a provider) instead
// of failing the whole daemon.
//
// The struct lives in the contract layer (kernel/contract/workflowapi) — it is
// the HOST's own surface handed to the engine, and a module's ports are the
// contract's. The name stays here as an alias, so every existing caller is
// unchanged.

// Provenance is why a run exists. The host fills it in from its own wake
// context — the module never reads another package's context keys.
//
// The row lives in the contract layer (kernel/contract/workflowapi). The name
// stays here as an alias.

// RunResult and NodeTestResult live in the contract layer
// (kernel/contract/workflowapi) — the agent-facing tool reports them, and a
// plugin may only name the contract. The module keeps their names as aliases
// (see aliases.go).

// Engine owns the workflow lifecycle for one store: the durable CRUD, the
// graph run, the single-node test and the copilot's draft.
//
// The store is the CONTRACT's host port (workflowapi.HostStore), not this
// module's concrete type: the engine is built by the composition root, and
// the root builds it over whatever store it holds.
type Engine struct {
	store workflowapi.HostStore
	ports Ports
}

// NewEngine binds the engine to its store and the host's ports.
func NewEngine(store workflowapi.HostStore, ports Ports) *Engine {
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
