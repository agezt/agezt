// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/datalakeapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/contract/workflowapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/introspecttool"
)

// TestToolKernelDepsSuppliesTheIntrospectPort: the daemon hands the
// introspect tool its live state through the contract's port. The port is
// supplied HERE — at the composition root — so a daemon that dropped it
// would leave the tool answering "not available" on every live box. This
// test is the only one that sees that wiring, so it asserts it directly.
func TestToolKernelDepsSuppliesTheIntrospectPort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
		Tools:    map[string]toolapi.Tool{"probe": nil},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.Introspect == nil {
		t.Fatal("the daemon must supply the introspect port")
	}

	// The port must answer the LIVE kernel — not an empty shell.
	ov := deps.Introspect.Overview()
	if ov.Daemon == "" {
		t.Fatalf("the port must name the daemon: %+v", ov)
	}
	if len(ov.Tools) == 0 {
		t.Fatalf("the port must list the live kernel's tools: %+v", ov)
	}
}

// TestToolKernelDepsSuppliesTheOverseerPort: the same wiring duty for the
// overseer port — the daemon supplies it, and it answers the live kernel.
func TestToolKernelDepsSuppliesTheOverseerPort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.Overseer == nil {
		t.Fatal("the daemon must supply the overseer port")
	}
	// The port must answer the live kernel — a port bound to nothing would
	// still be non-nil.
	if deps.Overseer.IsHalted() {
		t.Error("a freshly opened daemon is not halted")
	}
	if got := deps.Overseer.ActiveRunIDs(); len(got) != 0 {
		t.Errorf("a freshly opened daemon has no active runs, got %v", got)
	}
}

// TestToolKernelDepsSuppliesTheForgePort: the tool_forge tool reads the
// script-tool store through the contract's port. The port is supplied HERE —
// at the composition root — so a daemon that dropped it would leave the tool
// answering "not available" on every live box, while every test that builds
// the tool by hand stayed green. This test is the only one that sees that
// wiring, so it asserts it directly.
func TestToolKernelDepsSuppliesTheForgePort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.Forge == nil {
		t.Fatal("the daemon must supply the forge port")
	}

	// The port must answer the LIVE store — a port bound to nothing would
	// still be non-nil.
	if got := deps.Forge.List(); len(got) != 0 {
		t.Fatalf("a freshly opened daemon has no script tools, got %v", got)
	}
	if _, err := k.DraftScriptTool("corr", toolforgeapi.ScriptTool{
		Name:        "echo",
		Description: "echoes",
		Language:    "python",
		Code:        "print(1)",
	}); err != nil {
		t.Fatalf("DraftScriptTool: %v", err)
	}
	all := deps.Forge.List()
	if len(all) != 1 || all[0].Name != "echo" {
		t.Fatalf("the port must answer the live store, got %v", all)
	}
}

// TestForgeStoreStaysANilPortWithoutAStore: a daemon with no store must hand
// over a NIL port, not a port wrapping a nil implementation — otherwise the
// tool would call into nil instead of reporting that the store is missing.
func TestForgeStoreStaysANilPortWithoutAStore(t *testing.T) {
	if got := forgeStore(nil); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
	if got := forgePort(nil); got != nil {
		t.Fatalf("a missing store must yield a nil port, got %v", got)
	}
}

// TestToolKernelDepsSuppliesTheLakePort: the db tool drives the data lake
// through the contract's port. The port is supplied HERE — at the composition
// root — so a daemon that dropped it would leave the tool unable to reach
// the lake on every live box, while every test that builds the tool by hand
// stayed green. This test is the only one that sees that wiring.
func TestToolKernelDepsSuppliesTheLakePort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.Lake == nil {
		t.Fatal("the daemon must supply the lake port")
	}

	// The port must answer the LIVE lake — a port bound to nothing would
	// still be non-nil. The daemon seeds the lake's built-in collections.
	all := deps.Lake.ListCollections()
	if len(all) == 0 {
		t.Fatal("the live lake carries its built-in collections")
	}
	names := make([]string, 0, len(all))
	for _, c := range all {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	if names[0] != "bookmarks" {
		t.Fatalf("the port must answer the live lake, got %v", names)
	}
	if _, err := deps.Lake.CreateCollection(datalakeapi.Schema{Name: "agent-notes"}, "tester"); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	after := deps.Lake.ListCollections()
	if len(after) != len(all)+1 {
		t.Fatalf("the port must answer the live lake, got %d collections", len(after))
	}
}

// TestLakeStoreStaysANilPortWithoutALake: a daemon with no lake must hand
// over a NIL port, not a port wrapping a nil implementation.
func TestLakeStoreStaysANilPortWithoutALake(t *testing.T) {
	if got := lakeStore(nil); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
	if got := lakePort(nil); got != nil {
		t.Fatalf("a missing lake must yield a nil port, got %v", got)
	}
}

// TestToolKernelDepsSuppliesTheWorkflowPort: the workflow tool reads the
// workflow library through the contract's port. The port is supplied HERE —
// at the composition root — so a daemon that dropped it would leave the tool
// answering "not available" on every live box, while every test that builds
// the tool by hand stayed green. This test is the only one that sees that
// wiring, so it asserts it directly.
func TestToolKernelDepsSuppliesTheWorkflowPort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.Workflows == nil {
		t.Fatal("the daemon must supply the workflow port")
	}

	// The port must answer the LIVE library — a port bound to nothing would
	// still be non-nil.
	if got := deps.Workflows.List(); len(got) != 0 {
		t.Fatalf("a freshly opened daemon has no workflows, got %v", got)
	}
	if _, _, err := k.SaveWorkflow("corr", workflowapi.Workflow{
		Name:  "greeter",
		Nodes: []workflowapi.Node{{ID: "start", Type: workflowapi.NodeTrigger}},
	}); err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	all := deps.Workflows.List()
	if len(all) != 1 || all[0].Name != "greeter" {
		t.Fatalf("the port must answer the live library, got %v", all)
	}
}

// TestWorkflowStoreStaysANilPortWithoutALibrary: a daemon with no library
// must hand over a NIL port, not a port wrapping a nil implementation.
func TestWorkflowStoreStaysANilPortWithoutALibrary(t *testing.T) {
	if got := workflowStore(nil); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
}

// TestToolKernelDepsSuppliesTheWorkboardPort: the workboard tool reads the
// task store through the contract's port. The port is supplied HERE — at the
// composition root — so a daemon that dropped it would leave the tool
// answering "not available" on every live box, while every test that builds
// the tool by hand stayed green. This test is the only one that sees that
// wiring, so it asserts it directly.
func TestToolKernelDepsSuppliesTheWorkboardPort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.Workboard == nil {
		t.Fatal("the daemon must supply the workboard port")
	}

	// The port must answer the LIVE board — a port bound to nothing would
	// still be non-nil.
	if got := deps.Workboard.List(workboardapi.Filter{}); len(got) != 0 {
		t.Fatalf("a freshly opened daemon has no tasks, got %v", got)
	}
	if _, _, err := k.CreateWorkboardTask("corr", workboardapi.CreateSpec{Title: "wire me"}); err != nil {
		t.Fatalf("CreateWorkboardTask: %v", err)
	}
	all := deps.Workboard.List(workboardapi.Filter{})
	if len(all) != 1 || all[0].Title != "wire me" {
		t.Fatalf("the port must answer the live board, got %v", all)
	}
}

// TestWorkboardStoreStaysANilPortWithoutABoard: a daemon with no board must
// hand over a NIL port, not a port wrapping a nil implementation.
func TestWorkboardStoreStaysANilPortWithoutABoard(t *testing.T) {
	if got := workboardStore(nil); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
}

// TestTheIntrospectToolAnswersOnAConfiguredDaemon: the whole path — the
// composition root's port, the plugin's Bind, the tool's Invoke — has to
// work together, or the agent gets a daemon that cannot describe itself.
func TestTheIntrospectToolAnswersOnAConfiguredDaemon(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
		Tools:    map[string]toolapi.Tool{"probe": nil},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	tool := introspecttool.New()
	tool.Bind(deps.Introspect)

	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"overview"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the introspect tool errored on a configured daemon: %s", res.Output)
	}
	if !strings.Contains(res.Output, `"daemon"`) {
		t.Fatalf("the overview must carry the daemon row: %s", res.Output)
	}
}
