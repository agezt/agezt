// SPDX-License-Identifier: MIT

package mcp

// The MCP-TO-AGENT BRIDGE's own tests (W3.71).
//
// The bridge answers for the LIVE attachments, so it belongs with them — and
// its rules are the daemon's rules: a registered tool wins a name collision,
// an allowlist trims what a server exposes, and a lazy server collapses into
// ONE dispatcher tool instead of N schemas.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
)

// stubTool is a registered tool: the run's own map.
type stubTool struct{ name string }

func (s stubTool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: s.name, Description: "registered"}
}

func (s stubTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{Output: "registered"}, nil
}

// bridgeService builds a service with one attached server over a fake
// connection, and registers the row the bridge reads its per-server config
// from.
func bridgeService(t *testing.T, conn mcpapi.Conn, srv mcpapi.Server) *Service {
	t.Helper()
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("open the MCP registry: %v", err)
	}
	added, err := store.Add(srv)
	if err != nil {
		t.Fatalf("register the server: %v", err)
	}
	svc := NewService(extapi.MCPRegistryDependencies{Registry: store})
	svc.conns[added.Name] = conn
	return svc
}

// TestMergeToolsLeavesTheMapUntouchedWithoutAttachments: a run with no live
// attachment gets its own tool map back — the same map, not a copy.
func TestMergeToolsLeavesTheMapUntouchedWithoutAttachments(t *testing.T) {
	svc := NewService(extapi.MCPRegistryDependencies{})
	tools := map[string]toolapi.Tool{"own": stubTool{"own"}}
	got := svc.MergeTools(tools)
	if len(got) != 1 {
		t.Fatalf("the merge invented tools: %v", got)
	}
	if _, ok := got["own"]; !ok {
		t.Fatalf("the merge dropped the registered tool: %v", got)
	}
}

// TestMergeToolsExposesTheAttachedTools: an attached server's tools are
// offered under their callable names, and a call forwards to the connection.
func TestMergeToolsExposesTheAttachedTools(t *testing.T) {
	conn := &fakeConn{tools: []mcpapi.ToolDef{{Name: "greet", Description: "greets a name"}}, out: "hello"}
	svc := bridgeService(t, conn, mcpapi.Server{Name: "fake", Command: "never-executed"})

	got := svc.MergeTools(map[string]toolapi.Tool{})
	tool, ok := got["mcp_fake_greet"]
	if !ok {
		t.Fatalf("the attached tool is not offered: %v", got)
	}
	def := tool.Definition()
	if !strings.Contains(def.Description, "greets a name") || !strings.Contains(def.Description, "fake") {
		t.Fatalf("the offered tool does not describe the server: %+v", def)
	}
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"name":"ersin"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.Output != "hello" || res.IsError {
		t.Fatalf("the call did not forward to the connection: %+v", res)
	}
	if len(conn.calls) != 1 || conn.calls[0] != "greet" {
		t.Fatalf("the call did not reach the server's tool: %v", conn.calls)
	}
}

// TestMergeToolsHonoursTheAllowlist: a chatty server can be trimmed to the
// few tools a run actually needs.
func TestMergeToolsHonoursTheAllowlist(t *testing.T) {
	conn := &fakeConn{tools: []mcpapi.ToolDef{{Name: "greet"}, {Name: "shout"}}}
	svc := bridgeService(t, conn, mcpapi.Server{Name: "fake", Command: "never-executed", ToolAllow: []string{"greet"}})

	got := svc.MergeTools(map[string]toolapi.Tool{})
	if _, ok := got["mcp_fake_greet"]; !ok {
		t.Fatalf("the allowed tool is not offered: %v", got)
	}
	if _, ok := got["mcp_fake_shout"]; ok {
		t.Fatalf("the tool outside the allowlist is offered: %v", got)
	}
}

// TestMergeToolsCollapsesALazyServer: a lazy server is offered ONE
// dispatcher tool whose schema names the server's tools.
func TestMergeToolsCollapsesALazyServer(t *testing.T) {
	conn := &fakeConn{tools: []mcpapi.ToolDef{{Name: "greet", Description: "greets"}, {Name: "shout"}}, out: "done"}
	svc := bridgeService(t, conn, mcpapi.Server{Name: "lazy", Command: "never-executed", Lazy: true})

	got := svc.MergeTools(map[string]toolapi.Tool{})
	if len(got) != 1 {
		t.Fatalf("a lazy server must be offered one tool, got %v", got)
	}
	tool, ok := got["mcp_lazy"]
	if !ok {
		t.Fatalf("the dispatcher is not offered: %v", got)
	}
	def := tool.Definition()
	if !strings.Contains(def.Description, "greet") || !strings.Contains(def.Description, "shout") {
		t.Fatalf("the dispatcher does not list the server's tools: %+v", def)
	}
	if !strings.Contains(string(def.InputSchema), `"enum"`) {
		t.Fatalf("the dispatcher's schema does not name the tools: %s", def.InputSchema)
	}
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"tool":"greet","arguments":{}}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.Output != "done" || res.IsError {
		t.Fatalf("the dispatcher did not forward the call: %+v", res)
	}
	if len(conn.calls) != 1 || conn.calls[0] != "greet" {
		t.Fatalf("the dispatcher called the wrong tool: %v", conn.calls)
	}
}

// TestTheLazyDispatcherRefusesAnUnexposedTool: the dispatcher forwards only
// what the server exposes.
func TestTheLazyDispatcherRefusesAnUnexposedTool(t *testing.T) {
	conn := &fakeConn{tools: []mcpapi.ToolDef{{Name: "greet"}}}
	svc := bridgeService(t, conn, mcpapi.Server{Name: "lazy", Command: "never-executed", Lazy: true})
	got := svc.MergeTools(map[string]toolapi.Tool{})
	res, err := got["mcp_lazy"].Invoke(context.Background(), json.RawMessage(`{"tool":"shout"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not exposed") {
		t.Fatalf("the dispatcher must refuse an unexposed tool: %+v", res)
	}
	if len(conn.calls) != 0 {
		t.Fatalf("the refused tool still reached the server: %v", conn.calls)
	}
}

// TestARegisteredToolWinsTheCollision: the run's own tool is not shadowed by
// a bridged one.
func TestARegisteredToolWinsTheCollision(t *testing.T) {
	conn := &fakeConn{tools: []mcpapi.ToolDef{{Name: "greet"}}}
	svc := bridgeService(t, conn, mcpapi.Server{Name: "fake", Command: "never-executed"})
	own := stubTool{"own"}
	got := svc.MergeTools(map[string]toolapi.Tool{"mcp_fake_greet": own})
	if got["mcp_fake_greet"].Definition().Description != "registered" {
		t.Fatalf("the bridged tool shadowed the registered one: %+v", got["mcp_fake_greet"].Definition())
	}
}

// TestTheCallableNameIsSanitizedAndCapped: the tool name must be
// provider-safe and short enough for every provider's limit.
func TestTheCallableNameIsSanitizedAndCapped(t *testing.T) {
	if got := mcpToolName("srv", "a b/c"); got != "mcp_srv_a-b-c" {
		t.Fatalf("the callable name was not sanitized: %q", got)
	}
	long := strings.Repeat("x", 100)
	got := mcpToolName("srv", long)
	if len(got) != 64 {
		t.Fatalf("the callable name is not capped at 64 chars: %d (%q)", len(got), got)
	}
	if !strings.HasPrefix(got, "mcp_srv_") {
		t.Fatalf("the capped name lost its prefix: %q", got)
	}
}

// TestABridgedToolReportsTheServersError: a failing call is an error RESULT,
// not a crashed run — and a cancelled run stays cancelled.
func TestABridgedToolReportsTheServersError(t *testing.T) {
	conn := &fakeConn{tools: []mcpapi.ToolDef{{Name: "greet"}}, err: errors.New("the server exploded")}
	svc := bridgeService(t, conn, mcpapi.Server{Name: "fake", Command: "never-executed"})
	got := svc.MergeTools(map[string]toolapi.Tool{})
	res, err := got["mcp_fake_greet"].Invoke(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("a failing call must not fail the run: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "the server exploded") {
		t.Fatalf("the failure was not reported as an error result: %+v", res)
	}

	cancelled := &fakeConn{tools: []mcpapi.ToolDef{{Name: "greet"}}, err: context.Canceled}
	svc = bridgeService(t, cancelled, mcpapi.Server{Name: "fake", Command: "never-executed"})
	got = svc.MergeTools(map[string]toolapi.Tool{})
	if _, err := got["mcp_fake_greet"].Invoke(context.Background(), json.RawMessage(`{}`)); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled run must stay cancelled, got %v", err)
	}
}
