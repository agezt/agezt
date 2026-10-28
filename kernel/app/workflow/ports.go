// SPDX-License-Identifier: MIT

package workflow

import (
	"context"

	graphs "github.com/agezt/agezt/kernel/modules/workflows/workflow"
	"github.com/agezt/agezt/kernel/runtime"
)

// The workflow application's ports, filled from the kernel handle.
//
// The APPLICATION owns this: it knows which capabilities the workflows need
// and how each of them answers. The host hands over the handle and nothing
// else — it does not decide what the workflows are read from, and it cannot
// change the workflow wiring without editing the application.
//
// The same shape already exists in app/council, app/roster, app/standing and
// app/schedule: the application takes the kernel and builds its own ports.
//
// This family is the first one where the handle itself reaches the
// application — the workflow services need to SAVE, RESTORE, ENABLE, REMOVE,
// DRAFT, REFINE, RUN and TEST workflows, and the kernel is what does those.
// The port is the NARROW view of the handle that lets the application reach
// them and nothing else.
//
// The rows the workflow library holds are the module's own graph rows, and
// the kernel's accessors already hand them over as those rows, so the port
// hands them over without a conversion.

// Ports are the workflow application's capabilities over the workflows.
type Ports struct {
	// Reader reads the workflow library.
	//
	// It is the CONTRACT's store port: the daemon opens the library and
	// hands the port over. It is NIL when the daemon holds no workflow
	// library — the application answers "no workflows" rather than calling
	// into nothing. The services already know this: they refuse a missing
	// reader instead of panicking.
	Reader Reader

	// Journal folds the journal for a workflow's own story.
	Journal Journal

	// Writer writes the workflows: the kernel's save, restore, enable and
	// remove verbs.
	Writer Writer

	// Designer drafts and refines a workflow: the kernel's two verbs.
	Designer Designer

	// Correlation mints a correlation for a run that arrives without one.
	Correlation func() string

	// Run runs a workflow.
	Run func(context.Context, string, string, any) (RunResult, error)

	// TestNode runs one node of a workflow.
	TestNode func(context.Context, string, graphs.Workflow, string, map[string]any, any) (NodeResult, error)

	// Entries lists the workflows.
	//
	// It is nil-safe: a daemon with no workflow library sees no workflows
	// rather than a call into nothing. The adapter used to spell that
	// nil-check itself; the application spells it once.
	Entries func() []graphs.Workflow

	// Get reads one workflow.
	//
	// It is nil-safe, like Entries.
	Get func(ref string) (graphs.Workflow, bool)
}

// PortsFromKernel fills the workflow application's ports from the kernel
// handle.
func PortsFromKernel(k *runtime.Kernel) Ports {
	store := k.Workflows()
	return Ports{
		Reader:      store,
		Journal:     k.Journal(),
		Writer:      k,
		Designer:    k,
		Correlation: k.NewCorrelation,
		Run: func(ctx context.Context, corr, ref string, payload any) (RunResult, error) {
			res, err := k.RunWorkflow(ctx, corr, ref, payload)
			return RunResult{Executed: res.Executed, Outputs: res.Outputs}, err
		},
		TestNode: func(ctx context.Context, corr string, w graphs.Workflow, node string, data map[string]any, payload any) (NodeResult, error) {
			res, err := k.TestWorkflowNode(ctx, corr, w, node, data, payload)
			return NodeResult{Output: res.Output, Port: res.Port, Attempts: res.Attempts}, err
		},
		Entries: func() []graphs.Workflow {
			if store == nil {
				return nil
			}
			return store.List()
		},
		Get: func(ref string) (graphs.Workflow, bool) {
			if store == nil {
				return graphs.Workflow{}, false
			}
			return store.Get(ref)
		},
	}
}
