// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/app/binding"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// toolsPortTool is a registered tool: the registry the tools family reads.
type toolsPortTool struct{ name string }

func (t toolsPortTool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: t.name, InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (toolsPortTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{}, nil
}

// The tools application's ports are bound to this daemon's kernel, and the
// daemon's own services read the tools THROUGH THEM.
//
// The test dispatches the real operations: the adapter's wiring is what
// answers, so a port that answers for another kernel — or an empty one —
// shows up here.
func TestTheAdapterBindsTheToolsPortsToItsOwnKernel(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(withRoster(t, runtime.Config{
		BaseDir:  dir,
		Provider: mock.New(),
		Tools: map[string]toolapi.Tool{
			"shell":      toolsPortTool{name: "shell"},
			"remote_run": toolsPortTool{name: "remote_run"},
		},
	}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()
	if _, err := k.AddProfile(agentapi.Profile{Slug: "owned", Name: "Owned", Enabled: true}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	s := NewServer(binding.ScopeFromKernel(k, dir))
	s.token = "primary"

	// The daemon's own ports read its own kernel's registry.
	if !s.toolsPorts().Has("shell") {
		t.Error("the daemon's tools ports do not see the kernel's shell tool")
	}
	if s.toolsPorts().Count() != 2 {
		t.Errorf("the daemon's tools ports count %d tools, want the kernel's two", s.toolsPorts().Count())
	}

	// The daemon's tool inventory — the adapter's own wiring — lists the
	// kernel's tools.
	inventory := callAppHost(t, s, Request{ID: "i", Cmd: "tool_list", Token: "primary"})[0]
	if inventory.Type != RespResult {
		t.Fatalf("the tool inventory: %s", inventory.Error)
	}
	rows := inventory.Result
	listed, _ := rows["tools"].([]any)
	if len(listed) != 2 {
		t.Fatalf("the tool inventory listed %+v, want the kernel's two tools", inventory.Result)
	}

	// The config's tool count is the tools family's answer, not the adapter's.
	shown := callAppHost(t, s, Request{ID: "c", Cmd: CmdConfig, Token: "primary"})[0]
	if shown.Type != RespResult {
		t.Fatalf("the config: %s", shown.Error)
	}
	config := shown.Result
	if count, _ := config["tool_count"].(float64); count != 2 {
		t.Errorf("the config counts %v tools, want the kernel's two", config["tool_count"])
	}

	// The roster's permission picture is built from the same registry.
	permissions := callAppHost(t, s, Request{ID: "p", Cmd: "agent_permissions", Token: "primary", Args: map[string]any{"ref": "owned"}})[0]
	if permissions.Type != RespResult {
		t.Fatalf("the agent permissions: %s", permissions.Error)
	}
	picture := permissions.Result
	permitted, _ := picture["permissions"].([]any)
	if len(permitted) != 2 {
		t.Fatalf("the permission picture holds %+v, want the kernel's two tools", permissions.Result)
	}

	// The schedule's tool targets are the tools family's answer too.
	if err := s.scheduleAdmission().Runnable(cadenceapi.Entry{Target: cadenceapi.TargetTool, Tool: "shell"}); err != nil {
		t.Errorf("the schedule admission refused a registered tool: %v", err)
	}
	if err := s.scheduleAdmission().Runnable(cadenceapi.Entry{Target: cadenceapi.TargetTool, Tool: "absent"}); err == nil {
		t.Error("the schedule admission accepted a tool the registry does not hold")
	}
}

// A tenant's operations run on the tenant's own kernel, and the tenant's
// tools are the tenant's tools: the REQUEST's ports follow the routed kernel,
// while the daemon's own ports answer for the root kernel.
func TestTheRequestToolsPortsFollowTheRoutedKernel(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{"shell": toolsPortTool{name: "shell"}}})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()
	tenant, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{"tenant_tool": toolsPortTool{name: "tenant_tool"}}})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tenant.Close()

	s := NewServer(binding.ScopeFromKernel(root, t.TempDir()))

	// The daemon's own ports answer for the root kernel.
	if !s.toolsPorts().Has("shell") || s.toolsPorts().Has("tenant_tool") {
		t.Error("the daemon's tools ports do not answer for its own kernel")
	}

	// The request's ports follow the routed kernel.
	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(tenant)})
	ports := requestToolsPorts(ctx)
	if !ports.Has("tenant_tool") {
		t.Error("the request's tools ports do not see the routed kernel's tool")
	}
	if ports.Has("shell") {
		t.Error("the request's tools ports see the root kernel's tool")
	}
}
