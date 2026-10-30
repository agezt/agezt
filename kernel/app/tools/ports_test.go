// SPDX-License-Identifier: MIT

package tools

import (
	"context"
	"sort"
	"testing"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
	"github.com/agezt/agezt/kernel/modules/extensions/mcp"
	"github.com/agezt/agezt/kernel/modules/extensions/toolforge"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The tools application's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// This family owns the tool REGISTRY, and the registry is read by other
// families too, so the test pins what that wiring buys: the tools the kernel
// holds are the tools the application reads, counts and answers about.
func TestTheToolsPortsReachTheKernelTools(t *testing.T) {
	shell := &inventoryTool{def: toolapi.ToolDef{Name: "shell", Description: " runs "}}
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
		Tools:    map[string]toolapi.Tool{"shell": shell},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)

	// The ports are FILLED: a port that is nil is not a narrow view — it is
	// no view at all, and an application that cannot read the tools cannot do
	// its job at all.
	if ports.Registry == nil {
		t.Fatal("the registry port is nil — the application cannot read the tools")
	}
	if ports.MCP == nil {
		t.Error("the MCP port is nil — the application cannot read the MCP servers")
	}
	if ports.Attachment == nil {
		t.Error("the attachment port is nil — the application cannot read the attachments")
	}
	if ports.MCPLifecycle == nil {
		t.Error("the MCP lifecycle port is nil — the application cannot drive the MCP servers")
	}
	if ports.ForgeWriter == nil {
		t.Error("the forge writer port is nil — the application cannot drive the forge")
	}

	// The registry port sees the kernel's tools.
	registered := ports.Registered()
	if len(registered) != 1 {
		t.Fatalf("Registered = %+v, want the kernel's one tool", registered)
	}
	if _, ok := registered["shell"]; !ok {
		t.Fatalf("Registered = %+v, want the kernel's shell tool", registered)
	}
	if ports.Count() != 1 {
		t.Errorf("Count = %d, want the kernel's one tool", ports.Count())
	}
	if !ports.Has("shell") {
		t.Error("Has(shell) = false, want the kernel's tool")
	}
	if ports.Has("absent") {
		t.Error("Has(absent) = true, want a tool the kernel does not hold")
	}

	// The inventory service reads the same registry THROUGH THE PORT.
	out, err := NewInventory(ports.Registry).List(context.Background(), InventoryInput{})
	if err != nil || out.Count != 1 || out.Tools[0].Name != "shell" {
		t.Fatalf("the inventory = %+v %v, want the kernel's tool", out, err)
	}
}

// The remote-run question is the tools family's: only the application knows
// which tool the node family means when it asks for a remote run.
func TestTheToolsPortsAnswerTheRemoteRunQuestion(t *testing.T) {
	remote := &inventoryTool{def: toolapi.ToolDef{Name: "remote_run"}}
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
		Tools:    map[string]toolapi.Tool{"remote_run": remote},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	present, registered := ports.RemoteRun()
	if !present || !registered {
		t.Errorf("RemoteRun = %v %v, want the registry's remote_run tool", present, registered)
	}

	// A registry entry that is not a tool is present and not registered.
	empty := Ports{Registry: stubRegistry{registered: map[string]toolapi.Tool{"remote_run": nil}}}
	present, registered = empty.RemoteRun()
	if !present || registered {
		t.Errorf("RemoteRun = %v %v, want present but not registered", present, registered)
	}
}

// A daemon with NO registry holds a nil port: the application sees no tools
// rather than calling into nothing.
func TestTheToolsPortsSurviveADaemonWithNoRegistry(t *testing.T) {
	ports := Ports{}
	if registered := ports.Registered(); registered != nil {
		t.Errorf("Registered = %+v, want none from a daemon with no registry", registered)
	}
	if ports.Count() != 0 {
		t.Errorf("Count = %d, want none from a daemon with no registry", ports.Count())
	}
	if ports.Has("shell") {
		t.Error("Has(shell) = true, want none from a daemon with no registry")
	}
	if present, registered := ports.RemoteRun(); present || registered {
		t.Errorf("RemoteRun = %v %v, want none from a daemon with no registry", present, registered)
	}
	// The inventory reads nothing rather than calling into nothing.
	out, err := NewInventory(nil).List(context.Background(), InventoryInput{})
	if err != nil || out.Count != 0 {
		t.Errorf("the inventory = %+v %v, want no tools from a daemon with no registry", out, err)
	}
}

// A daemon with NO forge holds a nil forge port, and the forge catalog
// refuses with a plain answer instead of reading into nothing.
func TestTheToolsPortsRefuseADaemonWithNoForge(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	if ports.ScriptForge != nil {
		t.Skip("this daemon holds a forge — the refusal is not the question here")
	}
	if _, err := NewForgeCatalog(ports.ScriptForge).List(context.Background(), ForgeListInput{}); err == nil {
		t.Error("the forge catalog listed a daemon with no forge, want the plain refusal")
	}
}

// The MCP ports reach the kernel's registry: the lifecycle port writes and
// the catalog port reads the same server back.
func TestTheToolsPortsReachTheKernelMCPServers(t *testing.T) {
	registry := newMCPRegistry()
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
		MCPStore: registry,
		MCPBuilder: func(deps extapi.MCPRegistryDependencies) extapi.MCPRegistryService {
			return mcp.NewService(deps)
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)

	added, err := NewMCPLifecycle(ports.MCPLifecycle, ports.Attachment).Add(context.Background(), MCPAddInput{Server: mcpapi.Server{Name: "owned", Command: "never-executed"}})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if added.Server.Name != "owned" {
		t.Fatalf("Add = %+v, want the registered server", added)
	}
	if registry.Count() != 1 {
		t.Fatalf("the lifecycle port wrote %d servers, want the kernel's registry to hold one", registry.Count())
	}

	out, err := NewMCPCatalog(ports.MCP, ports.Attachment).List(context.Background(), MCPListInput{})
	if err != nil || out.Count != 1 || out.Servers[0].Name != "owned" {
		t.Fatalf("the MCP catalog = %+v %v, want the server the lifecycle port wrote", out, err)
	}
}

// The forge ports reach the kernel's forge: the lifecycle port writes and the
// catalog port reads the same script tool back.
func TestTheToolsPortsReachTheKernelForge(t *testing.T) {
	store, err := toolforge.Open(t.TempDir())
	if err != nil {
		t.Fatalf("toolforge.Open: %v", err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), ToolForge: store})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	if ports.ScriptForge == nil {
		t.Fatal("the forge port is nil — the application cannot read the forge")
	}
	if ports.ForgeWriter == nil {
		t.Fatal("the forge writer port is nil — the application cannot drive the forge")
	}

	drafted, err := ports.ForgeWriter.DraftScriptTool("", toolforge.ScriptTool{Name: "echo", Description: "echoes", Language: "python", Code: "print(1)"})
	if err != nil {
		t.Fatalf("DraftScriptTool: %v", err)
	}
	if drafted.ID == "" {
		t.Fatalf("the lifecycle port drafted %+v, want a stored tool", drafted)
	}

	found, ok := ports.ScriptForge.Get(drafted.ID)
	if !ok || found.Name != "echo" {
		t.Fatalf("the forge port reads %+v %v, want the drafted tool", found, ok)
	}
	out, err := NewForgeCatalog(ports.ScriptForge).List(context.Background(), ForgeListInput{})
	if err != nil || out.Count != 1 || out.Tools[0].Name != "echo" {
		t.Fatalf("the forge catalog = %+v %v, want the drafted tool", out, err)
	}
}

// stubRegistry is a registry that holds what it is given.
type stubRegistry struct{ registered map[string]toolapi.Tool }

func (s stubRegistry) Registered() map[string]toolapi.Tool { return s.registered }

// mcpRegistry is an in-memory MCP registry: the port the kernel drives.
type mcpRegistry struct{ servers map[string]mcpapi.Server }

func newMCPRegistry() *mcpRegistry { return &mcpRegistry{servers: map[string]mcpapi.Server{}} }

func (r *mcpRegistry) Add(srv mcpapi.Server) (mcpapi.Server, error) {
	if srv.ID == "" {
		srv.ID = srv.Name
	}
	srv.Enabled = true
	r.servers[srv.ID] = srv
	return srv, nil
}

func (r *mcpRegistry) SetEnabled(ref string, enabled bool) (mcpapi.Server, error) {
	srv, ok := r.servers[ref]
	if !ok {
		return mcpapi.Server{}, mcpapi.ErrNotFound
	}
	srv.Enabled = enabled
	r.servers[ref] = srv
	return srv, nil
}

func (r *mcpRegistry) Remove(ref string) (mcpapi.Server, bool, error) {
	srv, ok := r.servers[ref]
	if !ok {
		return mcpapi.Server{}, false, nil
	}
	delete(r.servers, ref)
	return srv, true, nil
}

func (r *mcpRegistry) Get(ref string) (mcpapi.Server, bool) {
	srv, ok := r.servers[ref]
	return srv, ok
}

func (r *mcpRegistry) List() []mcpapi.Server {
	out := make([]mcpapi.Server, 0, len(r.servers))
	for _, srv := range r.servers {
		out = append(out, srv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *mcpRegistry) Count() int { return len(r.servers) }
