// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/kernel/modules/workflows/workflow"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

const workflowGraph = `{"name":"greeter","description":"says hi","nodes":[{"id":"start","type":"trigger"},{"id":"greet","type":"transform","config":{"template":"hello"}}],"edges":[{"from":"start","to":"greet"}]}`

// withWorkflows opens the workflow library the composition root would open
// (W3.48) and hands the kernel the contract's host port — and the ENGINE,
// because the engine is the daemon's too: it is built over that store and over
// the host's own ports (the kernel's governed paths).
//
// This test is such a host: the library's file is the daemon's business, and
// the kernel holds the ports.
func withWorkflows(t *testing.T, cfg runtime.Config) *runtime.Kernel {
	t.Helper()
	store, err := workflow.OpenStore(filepath.Join(cfg.BaseDir, "workflows"))
	if err != nil {
		t.Fatalf("open the workflow store: %v", err)
	}
	cfg.Workflows = store
	k, err := runtime.Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	k.SetWorkflowEngine(workflow.NewEngine(store, k.WorkflowPorts()))
	return k
}

// TestTheWorkflowSpecWiresTheLibraryPort: the workflow tool is a plugin, so
// it may not name the workflows module — it asks for the CONTRACT's port and
// the daemon hands the live library over. The wiring has to be real: a
// workflow the agent saved must show up in the tool's own op=list, or the
// agent would author workflows it cannot see.
func TestTheWorkflowSpecWiresTheLibraryPort(t *testing.T) {
	k := withWorkflows(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	t.Cleanup(func() { k.Close() })

	if k.Workflows() == nil {
		t.Fatal("the kernel must own a workflow library")
	}

	spec := specWorkflow()
	built, err := spec.Build(toolregapi.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolregapi.KernelDeps{WorkflowRunner: k, Workflows: k.Workflows()}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(
		`{"op":"save","workflow":`+workflowGraph+`}`))
	if err != nil {
		t.Fatalf("Invoke save: %v", err)
	}
	if res.IsError {
		t.Fatalf("the tool errored: %s", res.Output)
	}

	// The read op must see the very workflow the kernel just stored — the
	// port really carried the live implementation, not a copy of it.
	res, err = built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke list: %v", err)
	}
	if res.IsError {
		t.Fatalf("the library port was not handed over: %s", res.Output)
	}
	if !strings.Contains(res.Output, "greeter") {
		t.Fatalf("the tool did not list the saved workflow: %s", res.Output)
	}
}

// TestTheWorkflowSpecSurvivesAMissingLibraryPort: a daemon that handed over
// only the runner must leave the read op reporting that the library is
// unavailable — not dereferencing nil.
func TestTheWorkflowSpecSurvivesAMissingLibraryPort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	spec := specWorkflow()
	built, err := spec.Build(toolregapi.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolregapi.KernelDeps{WorkflowRunner: k}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError {
		t.Fatalf("an unwired library port must report that, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "not available") {
		t.Fatalf("the unwired tool must say it is not available: %s", res.Output)
	}
}
