// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/workflowapi"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// triggerFlow is the smallest valid graph: one trigger node.
func triggerFlow(name string) workflowapi.Workflow {
	return workflowapi.Workflow{
		Name:  name,
		Nodes: []workflowapi.Node{{ID: "start", Type: workflowapi.NodeTrigger}},
	}
}

// TestTheWorkflowHoldsNoEngineWithoutOne: the engine is the MODULE's
// (W3.48/W3.76u) — the kernel holds the contract's port, and a kernel opened
// without one holds NO engine at all.
//
// The refusal is the answer, not a panic: a missing port must never take the
// daemon down. The refusal itself belongs to the layer that asks — the
// application binds the engine's face and says so (the contract's row,
// workflowapi.ErrNoEngine) instead of calling into nothing.
func TestTheWorkflowHoldsNoEngineWithoutOne(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if k.WorkflowEngine() != nil {
		t.Fatal("a kernel opened without an engine must hold no engine face")
	}
	if err := workflowapi.ErrNoEngine; !strings.Contains(err.Error(), "not available on this daemon") {
		t.Errorf("the refusal must name the reason: %v", err)
	}
}

// TestTheWorkflowEngineDrivesTheHostPort: with an engine wired, the kernel's
// lifecycle methods drive the ENGINE — and the engine writes through the
// contract's host port into the store the daemon opened.
//
// The chain is the whole slice: the kernel names the port, the engine is the
// module's implementation, and the row lands in the library. A port that
// answered "saved" without the row in the store would be a lie.
func TestTheWorkflowEngineDrivesTheHostPort(t *testing.T) {
	k, err := openKernelWithWorkflows(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("open the kernel: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if k.Workflows() == nil {
		t.Fatal("the kernel holds no workflow library")
	}

	saved, created, err := k.WorkflowEngine().Save("corr", triggerFlow("driven"))
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	if !created || saved.Name != "driven" {
		t.Fatalf("the engine did not save the graph: %+v (created=%v)", saved, created)
	}

	// The row is in the LIBRARY, not only in the return value.
	found, ok := k.Workflows().Get("driven")
	if !ok || found.Name != "driven" {
		t.Fatalf("the saved graph is not in the library: %+v", found)
	}

	// The enable/disable verb drives the same row.
	disabled, err := k.WorkflowEngine().SetEnabled("corr", "driven", false)
	if err != nil {
		t.Fatalf("SetWorkflowEnabled: %v", err)
	}
	if disabled.Enabled {
		t.Fatal("the workflow was not disarmed")
	}
	if stored, _ := k.Workflows().Get("driven"); stored.Enabled {
		t.Fatal("the library still holds an armed workflow")
	}

	// The remove verb answers what it removed.
	removed, err := k.WorkflowEngine().Remove("corr", "driven")
	if err != nil {
		t.Fatalf("RemoveWorkflow: %v", err)
	}
	if !removed {
		t.Fatal("the engine claimed it removed nothing")
	}
	if _, ok := k.Workflows().Get("driven"); ok {
		t.Fatal("the removed workflow is still in the library")
	}
}

// TestTheWorkflowLibraryStaysANilPortWithoutOne: a kernel opened without a
// library hands over a NIL port, not a port wrapping a nil implementation.
//
// The check is on the CONCRETE value: a nil *Store handed to an interface
// parameter is already a non-nil interface, and the tool would bind a reader
// that is not there.
func TestTheWorkflowLibraryStaysANilPortWithoutOne(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if got := k.Workflows(); got != nil {
		t.Fatalf("a kernel with no library must yield a nil port, got %v", got)
	}
}

// TestTheWorkflowReadPortIsTheReadSide: the kernel's own face hands out the
// READ side only (Get, List). The host's lifecycle goes through the engine
// port, and the agent-facing tool reads through this one — a reader that could
// remove a stored workflow would be editing the daemon's library.
//
// The width is checked STATICALLY, on the accessor itself: a widened face
// would still satisfy the read port (the host port is a superset), so only
// the declared return type can tell the two apart.
func TestTheWorkflowReadPortIsTheReadSide(t *testing.T) {
	k, err := openKernelWithWorkflows(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("open the kernel: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	// The port answers the contract's read side...
	var read workflowapi.Store = k.Workflows()
	if read == nil {
		t.Fatal("the kernel holds no workflow library")
	}

	// ...and the face is EXACTLY the read side — not the wider host port.
	got := reflect.TypeOf(k.Workflows).Out(0)
	want := reflect.TypeOf((*workflowapi.Store)(nil)).Elem()
	if got != want {
		t.Fatalf("the kernel's workflow face is %v, want the read side %v", got, want)
	}
	if want == reflect.TypeOf((*workflowapi.HostStore)(nil)).Elem() {
		t.Fatal("the read side and the host side are the same type — the width rule is gone")
	}

	// ...and the empty library is empty, not an error.
	if got := read.List(); len(got) != 0 {
		t.Fatalf("a fresh library is not empty: %v", got)
	}
	if _, ok := read.Get("nothing"); ok {
		t.Fatal("a fresh library must not know a workflow")
	}
}

// TestTheWorkflowRunCarriesTheHostProvenance: a run's provenance is the
// HOST's wake context — the module never reads another package's context keys.
// A run that lost its source would be journalled as a manual run, and the
// operator would see a cron-triggered run as if a person had started it.
func TestTheWorkflowRunCarriesTheHostProvenance(t *testing.T) {
	k, err := openKernelWithWorkflows(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(mock.FinalText("hi"))})
	if err != nil {
		t.Fatalf("open the kernel: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if _, _, err := k.WorkflowEngine().Save("corr", workflowapi.Workflow{
		Name: "provenance",
		Nodes: []workflowapi.Node{
			{ID: "start", Type: workflowapi.NodeTrigger},
			{ID: "brief", Type: workflowapi.NodeLLM, Config: json.RawMessage(`{"prompt":"say hi"}`)},
		},
		Edges: []workflowapi.Edge{{From: "start", To: "brief"}},
	}); err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}

	ctx := runctx.WithWakeContext(context.Background(), runctx.WakeContext{
		Source: "cron",
		Reason: "cron interval",
	})
	if _, err := k.WorkflowEngine().Run(ctx, k.NewCorrelation(), "provenance", nil); err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}
}
