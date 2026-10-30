// SPDX-License-Identifier: MIT

package runtime_test

// Provenance: the kernel's own test for the MCP REGISTRY PORT (W3.58). The
//
//	registry's FILE is the daemon's business: the kernel holds the contract's
//	port and never opens a registry of its own. These tests pin that split —
//	a kernel with no registry answers "no servers" and refuses the writes, and
//	a kernel NEVER creates a registry file behind the host's back.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// recordingRegistry is a contract-port registry that remembers what the
// kernel wrote through it.
type recordingRegistry struct {
	servers []mcpapi.Server
}

func (r *recordingRegistry) Add(srv mcpapi.Server) (mcpapi.Server, error) {
	srv.ID = "owned"
	r.servers = append(r.servers, srv)
	return srv, nil
}
func (r *recordingRegistry) SetEnabled(ref string, enabled bool) (mcpapi.Server, error) {
	for i, srv := range r.servers {
		if srv.Name == ref {
			r.servers[i].Enabled = enabled
			return srv, nil
		}
	}
	return mcpapi.Server{}, mcpapi.ErrNotFound
}
func (r *recordingRegistry) Remove(ref string) (mcpapi.Server, bool, error) {
	for i, srv := range r.servers {
		if srv.Name == ref {
			r.servers = append(r.servers[:i], r.servers[i+1:]...)
			return srv, true, nil
		}
	}
	return mcpapi.Server{}, false, nil
}
func (r *recordingRegistry) Get(ref string) (mcpapi.Server, bool) {
	for _, srv := range r.servers {
		if srv.ID == ref || srv.Name == ref {
			return srv, true
		}
	}
	return mcpapi.Server{}, false
}
func (r *recordingRegistry) List() []mcpapi.Server { return append([]mcpapi.Server(nil), r.servers...) }
func (r *recordingRegistry) Count() int            { return len(r.servers) }

// TestTheKernelHoldsTheMCPRegistryContractsPort: the registry reaches the
// kernel through the CONTRACT's port — not through the module's concrete type.
// A kernel that named the module's type could not be given a fake, and the
// layers above it could not speak to it without naming the module.
//
// The accessors must answer the contract's NAMED types. An anonymous
// interface with the same verbs would still compile — and would be a SECOND
// shape of the same port, drifting from the contract on its own.
func TestTheKernelHoldsTheMCPRegistryContractsPort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	store, ok := reflect.TypeOf(k).MethodByName("MCPStore")
	if !ok {
		t.Fatal("the kernel has no MCPStore accessor")
	}
	if got, want := store.Type.Out(0), reflect.TypeOf((*mcpapi.Registry)(nil)).Elem(); got != want {
		t.Fatalf("the registry accessor answers %v, want the contract's registry port %v", got, want)
	}
	reader, ok := reflect.TypeOf(k).MethodByName("MCPReader")
	if !ok {
		t.Fatal("the kernel has no MCPReader accessor")
	}
	if got, want := reader.Type.Out(0), reflect.TypeOf((*mcpapi.RegistrationReader)(nil)).Elem(); got != want {
		t.Fatalf("the reader accessor answers %v, want the contract's reader port %v", got, want)
	}
}

// TestKernelNeverOpensARegistryOfItsOwn: the registry's file belongs to the
// daemon. A kernel that opened one would leave a second registry behind the
// host's back — and the host's own registry would be the one nobody reads.
func TestKernelNeverOpensARegistryOfItsOwn(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if k.MCPStore() != nil {
		t.Fatal("the kernel opened a registry of its own")
	}
	if _, err := os.Stat(filepath.Join(dir, "mcp")); err == nil {
		t.Fatal("the kernel created a registry directory behind the host's back")
	}
	if _, err := os.Stat(filepath.Join(dir, "mcp", "servers.json")); err == nil {
		t.Fatal("the kernel created a registry file behind the host's back")
	}
}

// TestKernelWithoutARegistryAnswersNoServersAndRefusesWrites: a host that
// handed over no registry gets a daemon with no MCP servers — the reads say
// so, and the writes REFUSE with the kernel's own error rather than calling
// into nothing.
func TestKernelWithoutARegistryAnswersNoServersAndRefusesWrites(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if rows := k.MCPRegistrations(); len(rows) != 0 {
		t.Fatalf("a kernel with no registry must answer no servers, got %v", rows)
	}
	if n := k.MCPRegistrationCount(); n != 0 {
		t.Fatalf("a kernel with no registry must answer zero, got %d", n)
	}
	if _, found := k.MCPRegistration("ghost"); found {
		t.Fatal("a kernel with no registry invented a row")
	}
	if rows := k.MCPReader().List(); len(rows) != 0 {
		t.Fatalf("the reader must answer no servers, got %v", rows)
	}
	if _, _, err := k.AttachMCPServer(context.Background(), "corr", "ghost"); !errors.Is(err, runtime.ErrNoMCPRegistry) {
		t.Fatalf("attach without a registry must refuse, got %v", err)
	}
	if _, err := k.AddMCPServer("corr", mcpapi.Server{Name: "owned", Command: "never-executed"}); !errors.Is(err, runtime.ErrNoMCPRegistry) {
		t.Fatalf("add without a registry must refuse, got %v", err)
	}
	if _, err := k.SetMCPServerEnabled("corr", "owned", true); !errors.Is(err, runtime.ErrNoMCPRegistry) {
		t.Fatalf("set_enabled without a registry must refuse, got %v", err)
	}
	if _, err := k.RemoveMCPServer("corr", "owned"); !errors.Is(err, runtime.ErrNoMCPRegistry) {
		t.Fatalf("remove without a registry must refuse, got %v", err)
	}
}

// TestKernelHoldsTheRegistryItWasGiven: the port the kernel holds is the one
// the host handed over — the kernel does not open a second one over it.
func TestKernelHoldsTheRegistryItWasGiven(t *testing.T) {
	registry := &recordingRegistry{}
	k, err := runtime.Open(runtime.Config{
		BaseDir:    t.TempDir(),
		Provider:   mock.New(),
		MCPStore:   registry,
		MCPBuilder: mcpServiceBuilder(),
		MCPDialer:  func(context.Context, string, []string, map[string]string) (mcpapi.Conn, error) { return nil, nil },
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, err := k.AddMCPServer("corr", mcpapi.Server{Name: "owned", Command: "never-executed"}); err != nil {
		t.Fatalf("AddMCPServer: %v", err)
	}
	if registry.Count() != 1 {
		t.Fatalf("the kernel wrote to a registry of its own, not the injected one: %d", registry.Count())
	}
	if rows := k.MCPRegistrations(); len(rows) != 1 || rows[0].Name != "owned" {
		t.Fatalf("the kernel's reads do not see the injected registry: %v", rows)
	}
	if n := k.MCPRegistrationCount(); n != 1 {
		t.Fatalf("the kernel's count does not see the injected registry: %d", n)
	}
}

// TestAttachWithoutADialerRefusesInsteadOfPanicking: the dialers are the
// HOST's seams — the kernel may not name the module's client, so the daemon
// injects them. A kernel with no dialer must refuse the attach with a
// readable error; a nil func value would panic instead.
func TestAttachWithoutADialerRefusesInsteadOfPanicking(t *testing.T) {
	k, err := runtime.Open(withMCP(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, err := k.AddMCPServer("corr", mcpapi.Server{Name: "stdio", Command: "never-executed"}); err != nil {
		t.Fatalf("AddMCPServer: %v", err)
	}
	if _, err := k.AddMCPServer("corr", mcpapi.Server{Name: "remote", URL: "https://mcp.example.com/v1"}); err != nil {
		t.Fatalf("AddMCPServer: %v", err)
	}
	if _, _, err := k.AttachMCPServer(context.Background(), "corr", "stdio"); err == nil ||
		!strings.Contains(err.Error(), "no stdio MCP dialer is configured") {
		t.Fatalf("the stdio attach must refuse with the dialer's absence, got %v", err)
	}
	if _, _, err := k.AttachMCPServer(context.Background(), "corr", "remote"); err == nil ||
		!strings.Contains(err.Error(), "no HTTP MCP dialer is configured") {
		t.Fatalf("the http attach must refuse with the dialer's absence, got %v", err)
	}
}
