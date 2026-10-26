// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestMCPSpecWiresTheLiveKernel: the MCP tool is a plugin, so it may not
// name the registry — it asks the kernel for the ROWS. The wiring has to be
// real: a server registered in the kernel's own registry must be listed by
// the tool, and a server the tool adds must land in that same registry.
func TestMCPSpecWiresTheLiveKernel(t *testing.T) {
	k, err := runtime.Open(withMCP(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	spec := specMCP()
	built, err := spec.Build(toolregapi.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolregapi.KernelDeps{MCP: k}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// A server registered in the kernel's own registry.
	if _, err := k.AddMCPServer("corr-1", mcpapi.Server{
		Name: "github", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"},
	}); err != nil {
		t.Fatalf("AddMCPServer: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the mcp tool errored: %s", res.Output)
	}
	if !strings.Contains(res.Output, "github") {
		t.Fatalf("the tool did not list the live server: %s", res.Output)
	}

	// A server the TOOL adds must land in the kernel's own registry.
	if _, err := built.Tool.Invoke(context.Background(), json.RawMessage(
		`{"op":"add","name":"fetch","command":"uvx","args":["mcp-server-fetch"]}`)); err != nil {
		t.Fatalf("Invoke add: %v", err)
	}
	srv, found := k.MCPServer("fetch")
	if !found {
		t.Fatalf("the server the tool added is not in the kernel's registry: %+v", k.MCPServers())
	}
	if srv.Command != "uvx" {
		t.Fatalf("the stored server = %+v", srv)
	}
}
