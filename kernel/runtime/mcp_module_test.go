// SPDX-License-Identifier: MIT

package runtime_test

// The daemon's MCP verbs: the NAMES it keeps and the BODY it gave away.
//
// The daemon used to hold the registry's whole lifecycle — the store AND the
// operations over it, the live attachments and the bridge. It now holds the
// extensions module's face and asks the module. These tests pin that with a
// fake registry: if the daemon grew the lifecycle back, the fake would record
// nothing and the daemon's answer would be its own.

import (
	"context"
	"encoding/json"
	"errors"
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

// recordingMCPRegistry is a fake extensions module: it records what the daemon
// asks and answers with a row of its own.
//
// The embedded interface is nil on purpose: a verb the daemon must not reach
// for panics instead of silently answering.
type recordingMCPRegistry struct {
	extapi.MCPRegistryService
	calls  []string
	merged int
}

func (r *recordingMCPRegistry) Add(corr string, srv mcpapi.Server) (mcpapi.Server, error) {
	r.calls = append(r.calls, "add|"+corr+"|"+srv.Name)
	return mcpapi.Server{Name: srv.Name, ID: "owned"}, nil
}

func (r *recordingMCPRegistry) SetEnabled(corr, ref string, enabled bool) (mcpapi.Server, error) {
	r.calls = append(r.calls, "set-enabled|"+corr+"|"+ref)
	return mcpapi.Server{Name: ref, Enabled: enabled}, nil
}

func (r *recordingMCPRegistry) Attach(_ context.Context, corr, ref string) (mcpapi.Server, []string, error) {
	r.calls = append(r.calls, "attach|"+corr+"|"+ref)
	return mcpapi.Server{Name: ref}, []string{"mcp_" + ref + "_greet"}, nil
}

func (r *recordingMCPRegistry) Detach(corr, ref string) error {
	r.calls = append(r.calls, "detach|"+corr+"|"+ref)
	return nil
}

func (r *recordingMCPRegistry) Remove(corr, ref string) (bool, error) {
	r.calls = append(r.calls, "remove|"+corr+"|"+ref)
	return true, nil
}

func (r *recordingMCPRegistry) AttachEnabled(context.Context) ([]string, map[string]error) {
	r.calls = append(r.calls, "attach-enabled")
	return []string{"owned"}, map[string]error{}
}

func (r *recordingMCPRegistry) Attached() map[string]int {
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

// TestTheDaemonRefusesEveryRegistryVerbWithoutTheModule: a daemon with no
// registry module is a daemon with no MCP servers — the writes must SAY so
// rather than call into nothing.
func TestTheDaemonRefusesEveryRegistryVerbWithoutTheModule(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, err := k.AddMCPServer("corr", mcpapi.Server{Name: "owned", Command: "never-executed"}); !errors.Is(err, runtime.ErrNoMCPRegistry) {
		t.Fatalf("add without the module must refuse, got %v", err)
	}
	if _, err := k.SetMCPServerEnabled("corr", "owned", true); !errors.Is(err, runtime.ErrNoMCPRegistry) {
		t.Fatalf("set-enabled without the module must refuse, got %v", err)
	}
	if _, _, err := k.AttachMCPServer(context.Background(), "corr", "owned"); !errors.Is(err, runtime.ErrNoMCPRegistry) {
		t.Fatalf("attach without the module must refuse, got %v", err)
	}
	if err := k.DetachMCPServer("corr", "owned"); !errors.Is(err, runtime.ErrNoMCPRegistry) {
		t.Fatalf("detach without the module must refuse, got %v", err)
	}
	if _, err := k.RemoveMCPServer("corr", "owned"); !errors.Is(err, runtime.ErrNoMCPRegistry) {
		t.Fatalf("remove without the module must refuse, got %v", err)
	}
	if attached := k.MCPAttached(); len(attached) != 0 {
		t.Fatalf("a daemon with no module has no attachments, got %v", attached)
	}
	attached, failures := k.AttachEnabledMCPServers(context.Background())
	if len(attached) != 0 || len(failures) != 0 {
		t.Fatalf("a daemon with no module attaches nothing, got %v / %v", attached, failures)
	}
}

// TestTheDaemonForwardsEveryRegistryVerbToTheModule: the daemon keeps the
// NAMES; the module answers. The fake records what the daemon asked for.
func TestTheDaemonForwardsEveryRegistryVerbToTheModule(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	fake := &recordingMCPRegistry{}
	k.SetMCPRegistry(fake)

	if _, err := k.AddMCPServer("c1", mcpapi.Server{Name: "owned", Command: "never-executed"}); err != nil {
		t.Fatalf("AddMCPServer: %v", err)
	}
	if _, err := k.SetMCPServerEnabled("c2", "owned", true); err != nil {
		t.Fatalf("SetMCPServerEnabled: %v", err)
	}
	if _, _, err := k.AttachMCPServer(context.Background(), "c3", "owned"); err != nil {
		t.Fatalf("AttachMCPServer: %v", err)
	}
	if err := k.DetachMCPServer("c4", "owned"); err != nil {
		t.Fatalf("DetachMCPServer: %v", err)
	}
	if _, err := k.RemoveMCPServer("c5", "owned"); err != nil {
		t.Fatalf("RemoveMCPServer: %v", err)
	}
	if _, failures := k.AttachEnabledMCPServers(context.Background()); len(failures) != 0 {
		t.Fatalf("AttachEnabledMCPServers: %v", failures)
	}
	if got := k.MCPAttached(); len(got) != 1 || got["owned"] != 2 {
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

// TestTheDaemonKeepsTheRegistryNames: the host's public API is unchanged —
// the fourteen MCP verbs are still the daemon's.
func TestTheDaemonKeepsTheRegistryNames(t *testing.T) {
	want := []string{
		"MCPStore", "MCPRegistrations", "MCPRegistrationCount", "MCPRegistration",
		"MCPReader", "MCPServers", "MCPServer", "AddMCPServer", "SetMCPServerEnabled",
		"AttachMCPServer", "DetachMCPServer", "RemoveMCPServer", "AttachEnabledMCPServers",
		"MCPAttached",
	}
	typ := reflect.TypeOf(&runtime.Kernel{})
	for _, name := range want {
		if _, ok := typ.MethodByName(name); !ok {
			t.Fatalf("the daemon lost the verb %s", name)
		}
	}
}
