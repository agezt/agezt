// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/plugins/providers/mock"
)

const workflowGraph = `{"name":"greeter","description":"says hi","nodes":[{"id":"start","type":"trigger"},{"id":"greet","type":"transform","config":{"template":"hello"}}],"edges":[{"from":"start","to":"greet"}]}`

// TestTheWorkflowSpecWiresTheLibraryPort: the workflow tool is a plugin, so
// it may not name the workflows module — it asks for the CONTRACT's port and
// the daemon hands the live library over. The wiring has to be real: a
// workflow the agent saved must show up in the tool's own op=list, or the
// agent would author workflows it cannot see.
func TestTheWorkflowSpecWiresTheLibraryPort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if k.Workflows() == nil {
		t.Fatal("the kernel must own a workflow library")
	}

	spec := specWorkflow()
	built, err := spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolreg.KernelDeps{WorkflowRunner: k, Workflows: k.Workflows()}); err != nil {
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
	built, err := spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolreg.KernelDeps{WorkflowRunner: k}); err != nil {
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
