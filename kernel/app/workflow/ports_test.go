// SPDX-License-Identifier: MIT

package workflow

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	graphs "github.com/agezt/agezt/kernel/modules/workflows/workflow"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The workflow application's ports are filled by the APPLICATION from the
// kernel handle — the host never binds them itself.
//
// This family is the first one where the handle itself reaches the
// application, so the test pins what that wiring buys: the workflows the
// kernel holds are the workflows the application reads, edits and removes. A
// port bound to another library would leave the operator managing workflows
// the daemon never runs.
func TestTheWorkflowPortsReachTheKernelWorkflows(t *testing.T) {
	dir := t.TempDir()
	store, err := graphs.OpenStore(filepath.Join(dir, "workflows"))
	if err != nil {
		t.Fatalf("workflow.OpenStore: %v", err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), Workflows: store})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()
	// The kernel's workflow verbs need the engine, the way the daemon wires it.
	k.SetWorkflowEngine(graphs.NewEngine(store, k.WorkflowPorts()))

	ports := PortsFromKernel(k)

	// The ports are FILLED: this family is the first one where the handle
	// itself reaches the application, so an unfilled port is an application
	// that cannot do its job at all.
	if ports.Writer == nil {
		t.Error("the writer port is nil — the application cannot save a workflow")
	}
	if ports.Designer == nil {
		t.Error("the designer port is nil — the application cannot draft a workflow")
	}
	if ports.Correlation == nil {
		t.Fatal("the correlation port is nil — the application cannot name a run")
	}
	if corr := ports.Correlation(); corr == "" {
		t.Error("the correlation port answers empty — a run would have no name")
	}
	if ports.Run == nil || ports.TestNode == nil {
		t.Error("the run ports are nil — the application cannot run a workflow")
	}

	saved, created, err := ports.Writer.SaveWorkflow("", graphs.Workflow{
		Name:  "owned",
		Nodes: []graphs.Node{{ID: "start", Type: graphs.NodeTrigger}},
		Edges: []graphs.Edge{},
	})
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	if !created || saved.ID == "" {
		t.Fatalf("SaveWorkflow = %+v %v, want the workflow saved", saved, created)
	}

	// The ports' own listing sees the workflow.
	if entries := ports.Entries(); len(entries) != 1 || entries[0].ID != saved.ID {
		t.Fatalf("Entries = %+v, want the one workflow the port saved", entries)
	}

	// The read port sees it too.
	got, ok := ports.Get(saved.ID)
	if !ok || got.Name != "owned" {
		t.Fatalf("Get = %+v %v, want the saved workflow", got, ok)
	}

	// The reads service reads the same library.
	reads := NewReads(ports.Reader, ports.Journal, nil)
	shown, err := reads.Show(context.Background(), ShowInput{Ref: saved.ID})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if shown.Workflow.Name != "owned" {
		t.Fatalf("Show = %+v, want the saved workflow", shown)
	}

	// The lifecycle service writes through the same port.
	lifecycle := NewLifecycle(ports.Writer)
	enabled, err := lifecycle.SetEnabled(context.Background(), EnableInput{Ref: saved.ID, Enabled: false})
	if err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if enabled.Workflow.Enabled {
		t.Error("SetEnabled = enabled, want the workflow paused")
	}
}

// A daemon with NO workflow library holds a nil port: the application sees no
// workflows rather than calling into nothing.
//
// The port carries that discipline now: the adapter used to spell the
// nil-check itself, and a surface that forgot it would have called into
// nothing.
func TestTheWorkflowPortsSurviveADaemonWithNoWorkflowLibrary(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	if ports.Reader != nil {
		t.Error("the reader port is not nil — the daemon holds no workflow library")
	}
	if entries := ports.Entries(); entries != nil {
		t.Errorf("Entries = %+v, want none from a daemon with no workflow library", entries)
	}
	if _, ok := ports.Get("nothing"); ok {
		t.Error("Get(nothing) = found, want not found")
	}
}

var _ = json.RawMessage{}
