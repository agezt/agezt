// SPDX-License-Identifier: MIT

package runtime_test

// The daemon's MCP surface: the GATES it keeps and the FACE it hands over.
//
// The daemon used to hold the registry's whole lifecycle — the store AND the
// operations over it, the live attachments and the bridge. It now holds the
// extensions module's face and asks the module (W3.76w). These tests pin that
// with a fake registry: if the daemon grew the lifecycle back, the fake would
// record nothing and the daemon's answer would be its own.

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// mcpStubTool is a registered tool: the run's own map.
type mcpStubTool struct{ name string }

func (s mcpStubTool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: s.name, Description: "registered"}
}

func (s mcpStubTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{Output: "registered"}, nil
}

// stubMCPStore is the registry's store port, empty on purpose: the fake
// service answers for the module and never reaches the store.
type stubMCPStore struct{ mcpapi.Registry }

// recordingMCPRegistry is a fake extensions module: it records what the
// caller asks and answers with a row of its own.
//
// The embedded interface is nil on purpose: a verb the caller must not reach
// for panics instead of silently answering. The verbs it DOES answer are the
// CONTRACT's names (W3.76w) — the module answers in the vocabulary the
// contract publishes.
type recordingMCPRegistry struct {
	extapi.MCPRegistryService
	calls  []string
	merged int
}

func (r *recordingMCPRegistry) AddMCPServer(corr string, srv mcpapi.Server) (mcpapi.Server, error) {
	r.calls = append(r.calls, "add|"+corr+"|"+srv.Name)
	return mcpapi.Server{Name: srv.Name, ID: "owned"}, nil
}

func (r *recordingMCPRegistry) SetMCPServerEnabled(corr, ref string, enabled bool) (mcpapi.Server, error) {
	r.calls = append(r.calls, "set-enabled|"+corr+"|"+ref)
	return mcpapi.Server{Name: ref, Enabled: enabled}, nil
}

func (r *recordingMCPRegistry) AttachMCPServer(_ context.Context, corr, ref string) (mcpapi.Server, []string, error) {
	r.calls = append(r.calls, "attach|"+corr+"|"+ref)
	return mcpapi.Server{Name: ref}, []string{"mcp_" + ref + "_greet"}, nil
}

func (r *recordingMCPRegistry) DetachMCPServer(corr, ref string) error {
	r.calls = append(r.calls, "detach|"+corr+"|"+ref)
	return nil
}

func (r *recordingMCPRegistry) RemoveMCPServer(corr, ref string) (bool, error) {
	r.calls = append(r.calls, "remove|"+corr+"|"+ref)
	return true, nil
}

func (r *recordingMCPRegistry) AttachEnabledMCPServers(context.Context) ([]string, map[string]error) {
	r.calls = append(r.calls, "attach-enabled")
	return []string{"owned"}, map[string]error{}
}

func (r *recordingMCPRegistry) MCPAttached() map[string]int {
	r.calls = append(r.calls, "attached")
	return map[string]int{"owned": 2}
}

func (r *recordingMCPRegistry) MergeTools(tools map[string]toolapi.Tool) map[string]toolapi.Tool {
	r.calls = append(r.calls, "merge-tools")
	r.merged++
	out := make(map[string]toolapi.Tool, len(tools)+1)
	for name, t := range tools {
		out[name] = t
	}
	out["mcp_owned_greet"] = mcpStubTool{"mcp_owned_greet"}
	return out
}

func (r *recordingMCPRegistry) Close() {
	r.calls = append(r.calls, "close")
}

// TestTheDaemonHoldsNoRegistryFaceWithoutTheModule: a daemon with no
// registry module holds NO face.
//
// The registry's operations are the MODULE's (W3.76w): the daemon used to hold
// seven forwarders over them — a second spelling of every verb the contract's
// runner port already names — and it holds none now. A daemon with no module
// therefore holds no face at all: there is nothing left to refuse with, and
// the layers above (the application, the agent-facing tool) refuse in its
// place.
//
// The daemon's own READ gates still answer: a host that handed over no
// registry has no MCP servers.
func TestTheDaemonHoldsNoRegistryFaceWithoutTheModule(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if k.MCPRegistryService() != nil {
		t.Fatal("a daemon with no registry module must hold no registry face")
	}
	if rows := k.MCPReader().List(); len(rows) != 0 {
		t.Fatalf("a daemon with no registry must answer no servers, got %v", rows)
	}
	if n := k.MCPRegistrationCount(); n != 0 {
		t.Fatalf("a daemon with no registry must answer zero, got %d", n)
	}
}

// TestTheRegistryFaceAnswersEveryRegistryVerb: the face the daemon hands over
// is the MODULE's, and it answers every verb of the registry's lifecycle. The
// fake records what was asked.
//
// The fake is handed over the way the composition root hands it over — over
// runtime.Config.MCPBuilder (W3.76k: the daemon's SetMCPRegistry setter had
// no caller outside this test and is gone; the builder is the production
// path).
func TestTheRegistryFaceAnswersEveryRegistryVerb(t *testing.T) {
	fake := &recordingMCPRegistry{}
	cfg := runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}
	cfg.MCPStore = stubMCPStore{}
	cfg.MCPBuilder = func(extapi.MCPRegistryDependencies) extapi.MCPRegistryService { return fake }
	k, err := runtime.Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, err := k.MCPRegistryService().AddMCPServer("c1", mcpapi.Server{Name: "owned", Command: "never-executed"}); err != nil {
		t.Fatalf("AddMCPServer: %v", err)
	}
	if _, err := k.MCPRegistryService().SetMCPServerEnabled("c2", "owned", true); err != nil {
		t.Fatalf("SetMCPServerEnabled: %v", err)
	}
	if _, _, err := k.MCPRegistryService().AttachMCPServer(context.Background(), "c3", "owned"); err != nil {
		t.Fatalf("AttachMCPServer: %v", err)
	}
	if err := k.MCPRegistryService().DetachMCPServer("c4", "owned"); err != nil {
		t.Fatalf("DetachMCPServer: %v", err)
	}
	if _, err := k.MCPRegistryService().RemoveMCPServer("c5", "owned"); err != nil {
		t.Fatalf("RemoveMCPServer: %v", err)
	}
	if _, failures := k.MCPRegistryService().AttachEnabledMCPServers(context.Background()); len(failures) != 0 {
		t.Fatalf("AttachEnabledMCPServers: %v", failures)
	}
	if got := k.MCPRegistryService().MCPAttached(); len(got) != 1 || got["owned"] != 2 {
		t.Fatalf("the attached surface does not answer the module's: %v", got)
	}

	want := []string{
		"add|c1|owned",
		"set-enabled|c2|owned",
		"attach|c3|owned",
		"detach|c4|owned",
		"remove|c5|owned",
		"attach-enabled",
		"attached",
	}
	if !reflect.DeepEqual(fake.calls, want) {
		t.Fatalf("the daemon asked the module for %v, want %v", fake.calls, want)
	}
}

// TestTheDaemonKeepsOnlyTheRegistryGates: the host's public API keeps the
// registry's FILE and its READ GATES, and hands over the module's FACE.
//
// The daemon used to keep the whole MCP vocabulary — the row surface the
// agent-facing tool reads (MCPServers / MCPServer) and the lifecycle it
// drives (AddMCPServer, SetMCPServerEnabled, AttachMCPServer,
// DetachMCPServer, RemoveMCPServer, AttachEnabledMCPServers, MCPAttached).
// Those are the MODULE's verbs now (W3.76w): the module answers in the
// contract's names and the daemon spells none of them.
//
// The two readers the daemon kept for itself (MCPRegistrations /
// MCPRegistration) went with W3.76k: they had no caller outside the daemon,
// and the row surface above already answers the same rows.
func TestTheDaemonKeepsOnlyTheRegistryGates(t *testing.T) {
	want := []string{"MCPStore", "MCPRegistrationCount", "MCPReader", "MCPRegistryService"}
	typ := reflect.TypeOf(&runtime.Kernel{})
	for _, name := range want {
		if _, ok := typ.MethodByName(name); !ok {
			t.Fatalf("the daemon lost the verb %s", name)
		}
	}
	for _, gone := range []string{
		"MCPRegistrations", "MCPRegistration",
		"MCPServers", "MCPServer",
		"AddMCPServer", "SetMCPServerEnabled", "AttachMCPServer",
		"DetachMCPServer", "RemoveMCPServer", "AttachEnabledMCPServers",
		"MCPAttached",
	} {
		if _, ok := typ.MethodByName(gone); ok {
			t.Errorf("*Kernel has a %s method again — the registry's verbs are the module's, and one operation has one name (W3.76w)", gone)
		}
	}
}
