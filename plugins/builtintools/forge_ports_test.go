// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/kernel/modules/extensions/toolforge"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestTheToolForgeSpecWiresTheStorePort: the tool_forge tool is a plugin, so
// it may not name the module that keeps the script-tool store — it asks for
// the CONTRACT's port and the daemon hands the live store over. The wiring
// has to be real: a tool the agent drafted must show up in the tool's own
// op=list, or the agent would author tools it cannot see.
func TestTheToolForgeSpecWiresTheStorePort(t *testing.T) {
	// The store is the HOST's file (W3.45): the kernel holds the contract's
	// port and the host opens the store and injects it. This test is such a
	// host.
	dir := t.TempDir()
	store, err := toolforge.Open(filepath.Join(dir, "toolforge"))
	if err != nil {
		t.Fatalf("open the forge store: %v", err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), ToolForge: store})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if k.ToolForge() == nil {
		t.Fatal("the kernel must own a script-tool store")
	}

	spec := specToolForge()
	built, err := spec.Build(toolregapi.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolregapi.KernelDeps{ForgeRunner: k, Forge: k.ToolForge()}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(
		`{"op":"draft","name":"echo","description":"echoes its input","language":"python","code":"print(1)"}`))
	if err != nil {
		t.Fatalf("Invoke draft: %v", err)
	}
	if res.IsError {
		t.Fatalf("the tool errored: %s", res.Output)
	}

	// The read op must see the very tool the kernel just stored — the port
	// really carried the live implementation, not a copy of it.
	res, err = built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke list: %v", err)
	}
	if res.IsError {
		t.Fatalf("the store port was not handed over: %s", res.Output)
	}
	if !strings.Contains(res.Output, "echo") {
		t.Fatalf("the tool did not list the drafted tool: %s", res.Output)
	}
}

// TestTheToolForgeSpecSurvivesAMissingStorePort: a daemon that handed over
// only the kernel must leave the read op reporting that the store is
// unavailable — not dereferencing nil.
func TestTheToolForgeSpecSurvivesAMissingStorePort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	spec := specToolForge()
	built, err := spec.Build(toolregapi.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolregapi.KernelDeps{ForgeRunner: k}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError {
		t.Fatalf("an unwired store port must report that, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "not available") {
		t.Fatalf("the unwired tool must say it is not available: %s", res.Output)
	}
}
