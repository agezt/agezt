// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// fakeMCPConn is a canned MCP attachment: fixed tools, recorded calls.
type fakeMCPConn struct {
	tools    []mcp.ToolDef
	lastTool string
	lastArgs string
	out      string
	isErr    bool
	closed   bool
}

func (c *fakeMCPConn) Tools() []mcp.ToolDef { return c.tools }
func (c *fakeMCPConn) Call(_ context.Context, tool string, args json.RawMessage) (string, bool, error) {
	c.lastTool, c.lastArgs = tool, string(args)
	return c.out, c.isErr, nil
}
func (c *fakeMCPConn) Close() error { c.closed = true; return nil }

func openMCPKernel(t *testing.T, prov agent.Provider, conn *fakeMCPConn) (*runtime.Kernel, *int) {
	t.Helper()
	dials := 0
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		MCPDialer: func(_ context.Context, command string, args []string, _ map[string]string) (mcp.Conn, error) {
			dials++
			return conn, nil
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k, &dials
}

// TestAttach_OffersAndForwardsBridgedTool is the M796 e2e: register + attach
// a server (fake dialer), and a run is offered mcp_<server>_<tool>; the
// model's call forwards to the connection with the raw arguments, and the
// server's text comes back.
func TestAttach_OffersAndForwardsBridgedTool(t *testing.T) {
	prov := mock.New(
		mock.ToolUse("c1", "mcp_fake_greet", map[string]any{"name": "ersin"}),
		mock.FinalText("done"),
	)
	var first agent.CompletionRequest
	seen := false
	prov.OnRequest = func(r agent.CompletionRequest) {
		if !seen {
			first, seen = r, true
		}
	}
	conn := &fakeMCPConn{
		tools: []mcp.ToolDef{{Name: "greet", Description: "greets a name"}},
		out:   "hello ersin",
	}
	k, dials := openMCPKernel(t, prov, conn)

	if _, err := k.AddMCPServer("", mcp.Server{Name: "fake", Command: "python", Args: []string{"server.py"}}); err != nil {
		t.Fatalf("AddMCPServer: %v", err)
	}
	srv, names, err := k.AttachMCPServer(context.Background(), "", "fake")
	if err != nil || srv.Name != "fake" {
		t.Fatalf("Attach: %v / %+v", err, srv)
	}
	if len(names) != 1 || names[0] != "mcp_fake_greet" {
		t.Fatalf("discovered names = %v", names)
	}
	if *dials != 1 {
		t.Fatalf("dials = %d", *dials)
	}

	if _, err := k.RunWith(context.Background(), k.NewCorrelation(), "greet ersin"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	var offered bool
	for _, d := range first.Tools {
		if d.Name == "mcp_fake_greet" {
			offered = true
			if !strings.Contains(d.Description, "fake") {
				t.Errorf("description lost server origin: %q", d.Description)
			}
		}
	}
	if !offered {
		t.Fatalf("bridged tool not offered; tools = %v", toolNames(first.Tools))
	}
	if conn.lastTool != "greet" || !strings.Contains(conn.lastArgs, `"ersin"`) {
		t.Fatalf("call did not forward: %q %q", conn.lastTool, conn.lastArgs)
	}
}

// TestAttach_RemoteRoutesThroughHTTPDialer (M904, #39): a registration with a
// URL (not a command) attaches over the HTTP dialer seam, carrying the opt-in
// headers — never the stdio dialer.
func TestAttach_RemoteRoutesThroughHTTPDialer(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"))
	conn := &fakeMCPConn{tools: []mcp.ToolDef{{Name: "search"}}}

	stdioDials, httpDials := 0, 0
	var gotURL string
	var gotHeaders map[string]string
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		MCPDialer: func(_ context.Context, _ string, _ []string, _ map[string]string) (mcp.Conn, error) {
			stdioDials++
			return conn, nil
		},
		MCPHTTPDialer: func(_ context.Context, url string, headers map[string]string) (mcp.Conn, error) {
			httpDials++
			gotURL, gotHeaders = url, headers
			return conn, nil
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	reg := mcp.Server{
		Name:    "remote",
		URL:     "https://mcp.example.com/v1",
		Headers: map[string]string{"Authorization": "Bearer tok"},
	}
	if _, err := k.AddMCPServer("", reg); err != nil {
		t.Fatalf("AddMCPServer: %v", err)
	}
	_, names, err := k.AttachMCPServer(context.Background(), "", "remote")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if httpDials != 1 || stdioDials != 0 {
		t.Fatalf("dial routing wrong: http=%d stdio=%d", httpDials, stdioDials)
	}
	if gotURL != "https://mcp.example.com/v1" || gotHeaders["Authorization"] != "Bearer tok" {
		t.Fatalf("http dialer got url=%q headers=%v", gotURL, gotHeaders)
	}
	if len(names) != 1 || names[0] != "mcp_remote_search" {
		t.Fatalf("discovered names = %v", names)
	}
}

// TestAttach_ToolAllowFilters: a server with a ToolAllow allowlist exposes only
// the listed tools to a run — the others are kept out of context (M899).
func TestAttach_ToolAllowFilters(t *testing.T) {
	prov := mock.New(mock.FinalText("done"))
	var first agent.CompletionRequest
	seen := false
	prov.OnRequest = func(r agent.CompletionRequest) {
		if !seen {
			first, seen = r, true
		}
	}
	conn := &fakeMCPConn{tools: []mcp.ToolDef{{Name: "greet"}, {Name: "shout"}}}
	k, _ := openMCPKernel(t, prov, conn)
	if _, err := k.AddMCPServer("", mcp.Server{
		Name: "fake", Command: "python", Args: []string{"s.py"}, ToolAllow: []string{"greet"},
	}); err != nil {
		t.Fatalf("AddMCPServer: %v", err)
	}
	if _, _, err := k.AttachMCPServer(context.Background(), "", "fake"); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if _, err := k.RunWith(context.Background(), k.NewCorrelation(), "hi"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	var hasGreet, hasShout bool
	for _, d := range first.Tools {
		switch d.Name {
		case "mcp_fake_greet":
			hasGreet = true
		case "mcp_fake_shout":
			hasShout = true
		}
	}
	if !hasGreet {
		t.Errorf("allowed tool mcp_fake_greet not offered; tools = %v", toolNames(first.Tools))
	}
	if hasShout {
		t.Errorf("filtered tool mcp_fake_shout was offered despite the allowlist")
	}
}

// TestDetach_KillSwitch: after detach the connection is closed and the tools
// vanish from the next run; double-attach is refused while live.
func TestDetach_KillSwitch(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"))
	var req agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { req = r }
	conn := &fakeMCPConn{tools: []mcp.ToolDef{{Name: "greet"}}}
	k, _ := openMCPKernel(t, prov, conn)

	if _, err := k.AddMCPServer("", mcp.Server{Name: "fake", Command: "python"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, _, err := k.AttachMCPServer(context.Background(), "", "fake"); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if _, _, err := k.AttachMCPServer(context.Background(), "", "fake"); err == nil {
		t.Fatal("double attach accepted")
	}
	if err := k.DetachMCPServer("", "fake"); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	if !conn.closed {
		t.Fatal("detach did not close the connection")
	}
	if err := k.DetachMCPServer("", "fake"); err == nil {
		t.Fatal("detaching a non-attached server accepted")
	}
	if _, err := k.RunWith(context.Background(), k.NewCorrelation(), "hi"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if contains(toolNames(req.Tools), "mcp_fake_greet") {
		t.Fatal("detached server's tool still offered")
	}
}

// TestRemove_DetachesFirst and AttachEnabled boot path.
func TestRemoveAndAttachEnabled(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"))
	conn := &fakeMCPConn{tools: []mcp.ToolDef{{Name: "greet"}}}
	k, dials := openMCPKernel(t, prov, conn)

	if _, err := k.AddMCPServer("", mcp.Server{Name: "fake", Command: "python"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Boot path: enabled servers attach; disabled ones don't.
	attached, failures := k.AttachEnabledMCPServers(context.Background())
	if len(attached) != 1 || len(failures) != 0 || *dials != 1 {
		t.Fatalf("AttachEnabled = %v / %v / dials=%d", attached, failures, *dials)
	}
	if n := k.MCPAttached()["fake"]; n != 1 {
		t.Fatalf("MCPAttached = %v", k.MCPAttached())
	}

	// Remove detaches first, then deletes the registration.
	ok, err := k.RemoveMCPServer("", "fake")
	if err != nil || !ok {
		t.Fatalf("Remove = %v/%v", ok, err)
	}
	if !conn.closed {
		t.Fatal("remove did not detach/close first")
	}
	if k.MCPStore().Count() != 0 || len(k.MCPAttached()) != 0 {
		t.Fatal("registration or attachment survived remove")
	}
}

// TestAllowlistGatesBridgedTools: WithTools applies to MCP tools exactly like
// registered ones (merge before filter).
func TestAllowlistGatesBridgedTools(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"), mock.FinalText("ok"))
	var req agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { req = r }
	conn := &fakeMCPConn{tools: []mcp.ToolDef{{Name: "greet"}}}
	k, _ := openMCPKernel(t, prov, conn)
	if _, err := k.AddMCPServer("", mcp.Server{Name: "fake", Command: "python"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, _, err := k.AttachMCPServer(context.Background(), "", "fake"); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	ctx := runtime.WithTools(context.Background(), []string{"memory"})
	if _, err := k.RunWith(ctx, k.NewCorrelation(), "restricted"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if contains(toolNames(req.Tools), "mcp_fake_greet") {
		t.Fatal("allowlist did not gate the bridged tool")
	}
	ctx = runtime.WithTools(context.Background(), []string{"mcp_fake_greet"})
	if _, err := k.RunWith(ctx, k.NewCorrelation(), "allowed"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if !contains(toolNames(req.Tools), "mcp_fake_greet") {
		t.Fatal("allowlisted bridged tool missing")
	}
}

// TestBridgedToolNameSanitized: a server tool with hostile characters becomes
// a provider-safe, length-capped name.
func TestBridgedToolNameSanitized(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"))
	var req agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { req = r }
	long := strings.Repeat("x", 80)
	conn := &fakeMCPConn{tools: []mcp.ToolDef{{Name: "weird/tool name!" + long}}}
	k, _ := openMCPKernel(t, prov, conn)
	if _, err := k.AddMCPServer("", mcp.Server{Name: "fake", Command: "python"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, names, err := k.AttachMCPServer(context.Background(), "", "fake"); err != nil || len(names) != 1 {
		t.Fatalf("Attach: %v / %v", err, names)
	}
	if _, err := k.RunWith(context.Background(), k.NewCorrelation(), "hi"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	var got string
	for _, d := range req.Tools {
		if strings.HasPrefix(d.Name, "mcp_fake_") {
			got = d.Name
		}
	}
	if got == "" {
		t.Fatalf("sanitized tool missing; tools = %v", toolNames(req.Tools))
	}
	if len(got) > 64 || strings.ContainsAny(got, "/ !") {
		t.Fatalf("name not provider-safe: %q (len %d)", got, len(got))
	}
}
