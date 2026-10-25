// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/workflowapi"
	"github.com/agezt/agezt/kernel/modules/workflows/workflow"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// openWorkflowKernel opens the kernel the way the COMPOSITION ROOT does
// (W3.48): the workflow library is the daemon's file, and the engine is the
// daemon's too — built over that store and over the host's own ports (the
// kernel's governed paths) — and handed over as the contract's engine port.
//
// The helper returns the KERNEL and not the config, because the engine cannot
// be wired without the kernel's own ports.
func openWorkflowKernel(t *testing.T, cfg kernelruntime.Config) *kernelruntime.Kernel {
	t.Helper()
	store, err := workflow.OpenStore(filepath.Join(cfg.BaseDir, "workflows"))
	if err != nil {
		t.Fatalf("open the workflow store: %v", err)
	}
	cfg.Workflows = store
	k, err := kernelruntime.Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	k.SetWorkflowEngine(workflow.NewEngine(store, k.WorkflowPorts()))
	return k
}

// TestOpenAppKernelWiresTheWorkflowLibrary: the workflow library is the
// DAEMON's file (W3.48) — the kernel holds the contract's host port and the
// composition root opens the store.
//
// The daemon end: the port is live, the ENGINE is wired (a kernel without an
// engine refuses every lifecycle operation), and the library is a FILE under
// the daemon's base directory — a workflow saved through the kernel has to
// land in the file the next start reads.
func TestOpenAppKernelWiresTheWorkflowLibrary(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if k.Workflows() == nil {
		t.Fatal("the daemon did not wire the workflow library — the kernel holds no port")
	}

	// The engine must be wired too: the refusal is the answer only for a
	// kernel that was opened WITHOUT one.
	if _, _, err := k.SaveWorkflow("corr", workflowapi.Workflow{
		Name:  "wired",
		Nodes: []workflowapi.Node{{ID: "start", Type: workflowapi.NodeTrigger}},
	}); err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}

	// The library is a FILE, and the daemon owns it.
	if _, err := os.Stat(filepath.Join(dir, "workflows", "workflows.json")); err != nil {
		t.Fatalf("the daemon did not open the library's own file: %v", err)
	}
}

// TestTheWorkflowEngineIsBuiltOverTheInjectedLibrary: the daemon builds the
// engine over the store it was HANDED (W3.48) — not over a store it opened
// behind the caller's back. A caller that injects a library must have its
// library used, or the injection would be a lie.
func TestTheWorkflowEngineIsBuiltOverTheInjectedLibrary(t *testing.T) {
	dir := t.TempDir()
	store, err := workflow.OpenStore(filepath.Join(dir, "injected"))
	if err != nil {
		t.Fatalf("open the workflow store: %v", err)
	}
	k, err := openAppKernel(kernelruntime.Config{
		BaseDir:   dir,
		Provider:  mock.New(),
		Workflows: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.SaveWorkflow("corr", workflowapi.Workflow{
		Name:  "injected",
		Nodes: []workflowapi.Node{{ID: "start", Type: workflowapi.NodeTrigger}},
	}); err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}

	// The injected library holds the workflow — the daemon did not open a
	// second library of its own.
	if _, found := store.Get("injected"); !found {
		t.Fatal("the daemon saved into its own library, not the injected one")
	}
}
