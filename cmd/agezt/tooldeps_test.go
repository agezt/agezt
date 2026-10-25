// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/datalakeapi"
	"github.com/agezt/agezt/kernel/contract/remoteapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/contract/workflowapi"
	"github.com/agezt/agezt/kernel/modules/board/board"
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
	k, err := runtime.Open(withArtifacts(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
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
	k, err := runtime.Open(withForge(t, withArtifacts(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})))
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
//
// The kernel holds the port itself now (W3.45): the daemon opened the store
// and injected the contract's port, so there is no wrapper left to test. The
// rule is that the kernel's field IS an interface, and a host that injects
// nothing leaves it a nil interface — which the tool registry receives as a
// nil port and reports as "the forge is unavailable".
func TestForgeStoreStaysANilPortWithoutAStore(t *testing.T) {
	if got := forgeStore(nil); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	if got := k.ToolForge(); got != nil {
		t.Fatalf("a kernel without an injected forge must hold a nil port, got %v", got)
	}
	if got := forgeStore(k); got != nil {
		t.Fatalf("a missing forge must yield a nil port, got %v", got)
	}
}

// TestToolKernelDepsSuppliesTheLakePort: the db tool drives the data lake
// through the contract's port. The port is supplied HERE — at the composition
// root — so a daemon that dropped it would leave the tool unable to reach
// the lake on every live box, while every test that builds the tool by hand
// stayed green. This test is the only one that sees that wiring.
//
// The kernel is opened THROUGH the composition root (W3.42): the daemon opens
// the lake and seeds its built-in collections, then hands the kernel the
// contract's port. A kernel opened without the root has no lake at all.
func TestToolKernelDepsSuppliesTheLakePort(t *testing.T) {
	k, err := openAppKernel(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
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
//
// Since W3.42 the kernel holds the port itself, so there is no wrapper left
// to wrap a nil lake: the kernel's port IS the port the tool gets, and a
// kernel that was given no lake holds a nil port.
func TestLakeStoreStaysANilPortWithoutALake(t *testing.T) {
	if got := lakeStore(nil); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
	k, err := runtime.Open(withArtifacts(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	if got := lakeStore(k); got != nil {
		t.Fatalf("a kernel with no lake must yield a nil port, got %v", got)
	}
}

// TestToolKernelDepsSuppliesTheWorkflowPort: the workflow tool reads the
// workflow library through the contract's port. The port is supplied HERE —
// at the composition root — so a daemon that dropped it would leave the tool
// answering "not available" on every live box, while every test that builds
// the tool by hand stayed green. This test is the only one that sees that
// wiring, so it asserts it directly.
func TestToolKernelDepsSuppliesTheWorkflowPort(t *testing.T) {
	k, err := runtime.Open(withArtifacts(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
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
	k, err := runtime.Open(withWorkboard(t, withArtifacts(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})))
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

// TestToolKernelDepsSuppliesTheConfigPort: the daemon hands the config tool
// its live-daemon port — the agent-scoped write path and the live-apply
// reload. The port is supplied HERE — at the composition root — so a daemon
// that dropped it would leave the tool unable to write an agent's override
// on every live box, while every test that builds the tool by hand stayed
// green. This test is the only one that sees that wiring, so it asserts it
// directly.
func TestToolKernelDepsSuppliesTheConfigPort(t *testing.T) {
	k, err := runtime.Open(withArtifacts(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.Config == nil {
		t.Fatal("the daemon must supply the config port")
	}

	// The port must answer the LIVE roster — a profile added to the kernel
	// is the profile the port edits.
	if _, err := k.AddProfile(agentapi.Profile{Slug: "researcher"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	got, found, err := deps.Config.UpdateProfile("researcher", func(p *agentapi.Profile) {
		p.ConfigOverrides = map[string]string{"AGEZT_MODEL": "agent-model"}
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if !found {
		t.Fatal("the port must reach the live roster")
	}
	if got.ConfigOverrides["AGEZT_MODEL"] != "agent-model" {
		t.Fatalf("the port did not edit the live profile: %+v", got.ConfigOverrides)
	}
}

// TestConfigSourceStaysANilPortWithoutAKernel: a daemon with no kernel must
// hand over a NIL port, not a port wrapping a nil implementation.
func TestConfigSourceStaysANilPortWithoutAKernel(t *testing.T) {
	if got := configSource(nil); got != nil {
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

// TestToolKernelDepsSuppliesTheArtifactPort: the artifact index reaches the
// tools through the contract's port. The port is supplied HERE — at the
// composition root — so a daemon that dropped it would leave the artifacts,
// fetch, browser-action and code_exec tools unable to read or write the
// index on every live box, while every test that builds the tool by hand
// stayed green. This test is the only one that sees that wiring, so it
// asserts it directly.
func TestToolKernelDepsSuppliesTheArtifactPort(t *testing.T) {
	k, err := runtime.Open(withArtifacts(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.Artifacts == nil {
		t.Fatal("the daemon must supply the artifact port")
	}

	// The port must answer the LIVE index — a port bound to nothing would
	// still be non-nil.
	row, err := deps.Artifacts.PutEntry(remoteapi.Artifact{Kind: "image", Name: "cat.png", Mime: "image/png"}, []byte("PNGDATA"), 1)
	if err != nil {
		t.Fatalf("PutEntry: %v", err)
	}
	if row.Name != "cat.png" {
		t.Fatalf("the port must answer the live index, got %+v", row)
	}
	list := deps.Artifacts.List(remoteapi.Filter{})
	if len(list) != 1 || list[0].Name != "cat.png" {
		t.Fatalf("the port must answer the live index, got %v", list)
	}
}

// TestArtifactPortStaysANilPortWithoutAnIndex: a daemon with no index must
// hand over a NIL port, not a port wrapping a nil implementation.
//
// The kernel holds the port itself now (W3.43), so there is no wrapper left
// to test: the rule is that the kernel's field IS an interface, and a host
// that injects nothing leaves it a nil interface — which the tool registry
// receives as a nil port and reports as "the artifact store is unavailable".
func TestArtifactPortStaysANilPortWithoutAnIndex(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if got := k.ArtifactIndex(); got != nil {
		t.Fatalf("a kernel without an injected index must hold a nil port, got %v", got)
	}
	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.Artifacts != nil {
		t.Fatalf("a missing index must yield a nil port, got %v", deps.Artifacts)
	}
}

// TestBoardPortStaysANilPortWithoutAStore: the board store is opened late,
// and the daemon leaves it nil when it could not be opened. The same
// nil-port rule applies: a nil *board.Store handed to an interface parameter
// is a NON-NIL interface, so the board tool would bind a store that is not
// there and panic on the first post.
func TestBoardPortStaysANilPortWithoutAStore(t *testing.T) {
	if got := boardPort(nil); got != nil {
		t.Fatalf("a missing store must yield a nil port, got %v", got)
	}
}

// TestBoardPortAnswersTheLiveStore: the board port is not an empty shell —
// it is the daemon's own store, so a message posted through it is the
// message the daemon's board holds.
func TestBoardPortAnswersTheLiveStore(t *testing.T) {
	st, err := board.Open(t.TempDir())
	if err != nil {
		t.Fatalf("board.Open: %v", err)
	}
	port := boardPort(st)
	if port == nil {
		t.Fatal("a live store must yield a live port")
	}
	if _, err := port.Post("help", "tester", "please look", 1); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if msgs := st.Read("help", 10); len(msgs) != 1 || msgs[0].Text != "please look" {
		t.Fatalf("the port must answer the live store, got %+v", msgs)
	}
}

// TestToolKernelDepsSuppliesTheReasoningPort: the council, conductor and
// research tools drive the daemon's reasoning surface through the contract's
// port (W3.39b). The port is supplied HERE — at the composition root — so a
// daemon that dropped it would leave three tools answering "unavailable" on
// every live box, while every test that built the tool by hand stayed
// green. This test is the only one that sees that wiring, so it asserts it
// directly: the port must be present AND it must be the live kernel's
// reasoning surface.
func TestToolKernelDepsSuppliesTheReasoningPort(t *testing.T) {
	k, err := runtime.Open(withArtifacts(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.Reasoning == nil {
		t.Fatal("the daemon must supply the reasoning port")
	}
	if deps.Reasoning != k {
		t.Fatal("the reasoning port must be the live kernel's reasoning surface")
	}
}

// TestReasoningPortStaysANilPortWithoutAKernel: a daemon with no kernel must
// hand over a NIL port, not a port wrapping a nil implementation — the
// check is on the concrete type, because a nil *runtime.Kernel handed to an
// interface parameter is already a non-nil interface, and the three tools
// would bind a runner that is not there.
func TestReasoningPortStaysANilPortWithoutAKernel(t *testing.T) {
	if got := reasoningPort(nil); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
}

// TestToolKernelDepsSuppliesTheWorkflowAndWorkboardPorts: the workflow and
// workboard tools drive the daemon's library and board through the
// contract's ports (W3.39c). The ports are supplied HERE — at the
// composition root — so a daemon that dropped them would leave two tools
// answering "not available" on every live box, while every test that built
// the tool by hand stayed green. This test is the only one that sees that
// wiring, so it asserts it directly: the ports must be present AND they
// must be the live kernel's.
func TestToolKernelDepsSuppliesTheWorkflowAndWorkboardPorts(t *testing.T) {
	k, err := runtime.Open(withArtifacts(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.WorkflowRunner == nil {
		t.Fatal("the daemon must supply the workflow runner port")
	}
	if deps.WorkflowRunner != k {
		t.Fatal("the workflow runner port must be the live kernel's")
	}
	if deps.WorkboardRunner == nil {
		t.Fatal("the daemon must supply the workboard runner port")
	}
	if deps.WorkboardRunner != k {
		t.Fatal("the workboard runner port must be the live kernel's")
	}
}

// TestTheWorkflowAndWorkboardPortsStayNilWithoutAKernel: the same nil-port
// rule as the other ports — a nil kernel handed to an interface parameter is
// a non-nil interface, so the tools would bind a runner that is not there.
func TestTheWorkflowAndWorkboardPortsStayNilWithoutAKernel(t *testing.T) {
	if got := workflowPort(nil); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
	if got := workboardPort(nil); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
}

// TestToolKernelDepsSuppliesTheMCPAndStandingPorts: the MCP and the
// standing-orders tools drive the daemon's registry and orders through the
// contract's ports (W3.39d). The ports are supplied HERE — at the
// composition root — so a daemon that dropped them would leave two tools
// answering "not available" on every live box, while every test that built
// the tool by hand stayed green. This test is the only one that sees that
// wiring, so it asserts it directly.
func TestToolKernelDepsSuppliesTheMCPAndStandingPorts(t *testing.T) {
	k, err := runtime.Open(withArtifacts(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.MCP == nil {
		t.Fatal("the daemon must supply the mcp port")
	}
	if deps.MCP != k {
		t.Fatal("the mcp port must be the live kernel's")
	}
	if deps.Standing == nil {
		t.Fatal("the daemon must supply the standing port")
	}
	if deps.Standing != k {
		t.Fatal("the standing port must be the live kernel's")
	}
	if deps.AgentLookup == nil {
		t.Fatal("the daemon must supply the agent lookup")
	}
	if _, err := k.AddProfile(agentapi.Profile{Slug: "scout"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	profile, ok := deps.AgentLookup("scout")
	if !ok || profile.Slug != "scout" {
		t.Fatalf("the agent lookup must answer the live roster, got %+v (found=%v)", profile, ok)
	}
}

// TestTheMCPAndStandingPortsStayNilWithoutAKernel: the same nil-port rule —
// a nil kernel handed to an interface parameter is a non-nil interface, so
// the tools would bind a runner that is not there.
func TestTheMCPAndStandingPortsStayNilWithoutAKernel(t *testing.T) {
	if got := mcpPort(nil); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
	if got := standingPort(nil); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
	if got := agentLookup(nil); got != nil {
		t.Fatalf("a missing kernel must yield no lookup, got %v", got != nil)
	}
}

// TestToolKernelDepsSuppliesTheScheduleAndSkillPorts: the schedule and the
// skill tools drive the daemon's cadence store and skills forge through the
// contract's ports (W3.39e). The ports are supplied HERE — at the
// composition root — so a daemon that dropped them would leave two tools
// answering "not available" on every live box, while every test that built
// the tool by hand stayed green. This test is the only one that sees that
// wiring, so it asserts it directly: the ports must be present AND they
// must be the live ones.
func TestToolKernelDepsSuppliesTheScheduleAndSkillPorts(t *testing.T) {
	k, err := runtime.Open(withArtifacts(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.Schedules == nil {
		t.Fatal("the daemon must supply the cadence store port")
	}
	if deps.Schedules != k.Schedules() {
		t.Fatal("the cadence port must be the live store")
	}
	if deps.SkillForge == nil {
		t.Fatal("the daemon must supply the skills forge port")
	}
	if deps.SkillForge != k.Forge() {
		t.Fatal("the forge port must be the live forge")
	}
	if deps.SkillBundles == nil {
		t.Fatal("the daemon must supply the bundle store port")
	}
	if deps.SkillBundles != k.Forge().Bundles() {
		t.Fatal("the bundle port must be the live bundle store")
	}
}

// TestTheScheduleAndSkillPortsStayNilWithoutAKernel: the same nil-port rule
// as the other ports — a nil kernel handed to an interface parameter is a
// non-nil interface, so the tools would bind a store that is not there.
func TestTheScheduleAndSkillPortsStayNilWithoutAKernel(t *testing.T) {
	if got := schedulesPort(nil); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
	if got := skillForgePort(nil); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
	if got := skillBundlesPort(nil); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
}

// TestTheBundlePortStaysNilWithoutBundles: the bundle store is OPTIONAL —
// the forge leaves skills body-only when none is wired — so a daemon without
// one must hand over a NIL port.
//
// The check is on the concrete type on purpose: a nil *skill.BundleStore
// handed to an interface parameter is already a non-nil interface, so the
// skill tool would bind a bundle store that is not there and panic on the
// first resource read instead of reporting that bundles are unavailable.
func TestTheBundlePortStaysNilWithoutBundles(t *testing.T) {
	k, err := runtime.Open(withArtifacts(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if k.Forge().Bundles() == nil {
		t.Fatal("the daemon's own forge must have a bundle store to unwire")
	}
	k.Forge().SetBundles(nil)

	if got := skillBundlesPort(k); got != nil {
		t.Fatalf("a forge without bundles must yield a nil port, got %v", got != nil)
	}
}
