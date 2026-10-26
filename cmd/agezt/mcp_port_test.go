// SPDX-License-Identifier: MIT

package main

// Provenance: the daemon's contract test for the MCP REGISTRY (W3.58) — that
//
//	the composition root opens the registry's FILE, hands the kernel the
//	contract's port, and injects the DIALERS (the module's clients, which the
//	kernel may not name).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/modules/extensions/mcp"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestOpenAppKernelWiresTheMCPRegistry: the daemon end — the registry is a
// FILE on disk under the base dir, and the port the kernel holds is the
// injected one.
//
// The file is the point: a server registered through the kernel has to land
// in the file the NEXT start reads, or the agent would forget its servers on
// every restart.
func TestOpenAppKernelWiresTheMCPRegistry(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("openAppKernel: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if k.MCPStore() == nil {
		t.Fatal("the daemon must hand the kernel an MCP registry port")
	}
	if fi, err := os.Stat(filepath.Join(dir, "mcp")); err != nil || !fi.IsDir() {
		t.Fatalf("the daemon must open the registry's directory: %v", err)
	}
	if _, err := k.AddMCPServer("corr", mcpapi.Server{Name: "owned", Command: "never-executed"}); err != nil {
		t.Fatalf("AddMCPServer: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mcp", "servers.json")); err != nil {
		t.Fatalf("the daemon's registry must be a file on disk: %v", err)
	}
	if n := k.MCPRegistrationCount(); n != 1 {
		t.Fatalf("the daemon's registry must hold the written server, got %d", n)
	}
}

// TestTheMCPRegistryIsTheInjectedOne: the daemon uses the INJECTED registry,
// not one of its own. A daemon that opened its own registry over the injected
// one would silently ignore what the host handed over.
func TestTheMCPRegistryIsTheInjectedOne(t *testing.T) {
	dir := t.TempDir()
	store, err := mcp.OpenStore(filepath.Join(dir, "injected"))
	if err != nil {
		t.Fatalf("open the MCP registry: %v", err)
	}
	k, err := openAppKernel(kernelruntime.Config{
		BaseDir:   dir,
		Provider:  mock.New(),
		MCPStore:  store,
		MCPDialer: func(context.Context, string, []string, map[string]string) (mcpapi.Conn, error) { return nil, nil },
	})
	if err != nil {
		t.Fatalf("openAppKernel: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if _, err := k.AddMCPServer("corr", mcpapi.Server{Name: "owned", Command: "never-executed"}); err != nil {
		t.Fatalf("AddMCPServer: %v", err)
	}
	if store.Count() != 1 {
		t.Fatalf("the daemon must use the injected registry, got %d servers in it", store.Count())
	}
	// And the daemon's OWN registry directory was never created: the injected
	// store replaced it, not accompanied it.
	if _, err := os.Stat(filepath.Join(dir, "mcp")); err == nil {
		t.Fatal("the daemon must not open a second registry over the injected one")
	}
}

// TestTheDaemonWiresTheDialers: the dialers are the HOST's seams — the
// clients live in the module and the kernel may not name them, so the daemon
// injects them. Without them every attach would refuse with "no dialer
// configured"; with them the attach reaches the spawn.
func TestTheDaemonWiresTheDialers(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("openAppKernel: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if _, err := k.AddMCPServer("corr", mcpapi.Server{Name: "owned", Command: "never-executed-binary"}); err != nil {
		t.Fatalf("AddMCPServer: %v", err)
	}
	_, _, err = k.AttachMCPServer(context.Background(), "corr", "owned")
	if err == nil {
		t.Fatal("attaching a server that does not exist must fail")
	}
	if strings.Contains(err.Error(), "no stdio MCP dialer is configured") {
		t.Fatalf("the daemon must wire the stdio dialer, got %v", err)
	}
	if !strings.Contains(err.Error(), "never-executed-binary") {
		t.Fatalf("the attach must reach the spawn, got %v", err)
	}
}
