// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"path/filepath"
	"testing"

	appworkflow "github.com/agezt/agezt/kernel/app/workflow"
	graphs "github.com/agezt/agezt/kernel/modules/workflows/workflow"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the workflow application's ports — the APPLICATION
// fills them from the kernel handle. This family is the first one where the
// handle itself reaches the application, so the binding is the whole point:
// the application gets the handle NARROWED to the workflow verbs, and the
// host keeps only the handle.
//
// The test pins that binding: the workflows the kernel holds are the
// workflows the adapter's ports read, and they are the workflows the roster
// impact's workflow port sees.
func TestTheAdapterBindsTheWorkflowPortsToItsOwnKernel(t *testing.T) {
	dir := t.TempDir()
	store, err := graphs.OpenStore(filepath.Join(dir, "workflows"))
	if err != nil {
		t.Fatalf("workflow.OpenStore: %v", err)
	}
	k, err := runtime.Open(withRoster(t, runtime.Config{BaseDir: dir, Provider: mock.New(), Workflows: store}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()
	k.SetWorkflowEngine(graphs.NewEngine(store, k.WorkflowPorts()))

	saved, _, err := k.SaveWorkflow("", graphs.Workflow{
		Name:  "owned",
		Nodes: []graphs.Node{{ID: "start", Type: graphs.NodeTrigger}},
	})
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}

	s := NewServer(k, dir)

	// The adapter's own ports read the kernel's workflows.
	got, ok := s.workflowPorts().Get(saved.ID)
	if !ok || got.Name != "owned" {
		t.Fatalf("the adapter's workflow ports Get = %+v %v, want the kernel's workflow", got, ok)
	}
	entries := s.workflowPorts().Entries()
	if len(entries) != 1 || entries[0].ID != saved.ID {
		t.Fatalf("the adapter's workflow ports Entries = %+v, want the kernel's workflow", entries)
	}

	// And the roster impact's workflow port — bound from the application's
	// port, not from the handle — sees the same library.
	if workflows := s.impactPorts().Workflows(); len(workflows) != 1 || workflows[0].ID != saved.ID {
		t.Fatalf("the impact's workflow port sees %+v, want the workflow the kernel holds", workflows)
	}

	// The daemon's workflow reads service serves the same workflow.
	shown, err := s.workflowReads().Show(context.Background(), appworkflow.ShowInput{Ref: saved.ID})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if shown.Workflow.Name != "owned" {
		t.Fatalf("Show = %+v, want the workflow the kernel holds", shown)
	}
}
