// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/contract/overseerapi"
	"github.com/agezt/agezt/kernel/toolreg"
)

// recordingSource is a fake overseer port. It records the calls the tool makes
// so the wiring can be asserted: the tool must reach the port the DAEMON
// injected, never one it builds itself.
//
// The tool used to build its own kernel-backed source inside Configure (from
// d.K and d.BaseDir). It cannot any more: the tool is a plugin, so it may
// neither reach the live kernel nor the board store — the implementation lives
// in the app layer and arrives through the contract. This fake is the proof
// that the port really crosses the boundary by injection.
type recordingSource struct {
	calls []string
}

func (r *recordingSource) note(call string) { r.calls = append(r.calls, call) }

func (r *recordingSource) IsHalted() bool {
	r.note("IsHalted")
	return false
}

func (r *recordingSource) ActiveRunIDs() []string {
	r.note("ActiveRunIDs")
	return []string{"run-1"}
}

func (r *recordingSource) Agents() []agentapi.Profile {
	r.note("Agents")
	return []agentapi.Profile{{Slug: "scout", Enabled: true}}
}

func (r *recordingSource) OpenHelp(int) []boardapi.Message {
	r.note("OpenHelp")
	return nil
}

func (r *recordingSource) AgentImpact(string) []string { return nil }
func (r *recordingSource) CancelRun(string) bool       { return false }
func (r *recordingSource) Halt(string)                 {}
func (r *recordingSource) ResumeAll(string)            {}

func (r *recordingSource) SetAgentEnabled(string, bool) (agentapi.Profile, error) {
	return agentapi.Profile{}, nil
}

func (r *recordingSource) SetAgentRetired(string, bool, string) (agentapi.Profile, error) {
	return agentapi.Profile{}, nil
}

func (r *recordingSource) EditAgent(string, agentapi.Profile) (agentapi.Profile, error) {
	return agentapi.Profile{}, nil
}

func (r *recordingSource) CreateAgent(agentapi.Profile) (agentapi.Profile, error) {
	return agentapi.Profile{}, nil
}

func (r *recordingSource) DeleteAgent(string) (bool, error) { return false, nil }

func (r *recordingSource) GetAgent(string) (agentapi.Profile, bool, error) {
	return agentapi.Profile{}, false, nil
}

func (r *recordingSource) CloneAgent(string, agentapi.Profile) (agentapi.Profile, error) {
	return agentapi.Profile{}, nil
}

func (r *recordingSource) SearchAgents(overseerapi.SearchFilter) []agentapi.Profile {
	return nil
}

func (r *recordingSource) BulkSetEnabled([]string, bool) []overseerapi.BulkResult { return nil }

func (r *recordingSource) BulkSetRetired([]string, bool, string) []overseerapi.BulkResult {
	return nil
}

func (r *recordingSource) BulkDelete([]string) []overseerapi.BulkResult { return nil }

func (r *recordingSource) WakeAgent(string, string, string) (string, error) { return "", nil }

func (r *recordingSource) RepairAgent(string, string) (agentapi.RepairResult, error) {
	return agentapi.RepairResult{}, nil
}

// TestOverseerSpecBindsTheInjectedPort: the daemon injects the app-layer
// implementation through the contract, and the tool steers THAT one.
func TestOverseerSpecBindsTheInjectedPort(t *testing.T) {
	spec := specOverseer()
	built, err := spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if built.Tool == nil {
		t.Fatal("Build returned no tool")
	}
	src := &recordingSource{}
	if err := spec.Configure(built.Tool, toolreg.KernelDeps{Overseer: src}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"status"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("status errored: %s", res.Output)
	}
	if len(src.calls) == 0 {
		t.Fatal("the tool never reached the injected port — it did not bind what the daemon supplied")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Output), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if out["halted"] != false {
		t.Fatalf("status output = %+v", out)
	}
	if out["active_runs"] != float64(1) {
		t.Fatalf("the injected port's answer did not reach the tool: %+v", out)
	}
}

// TestOverseerSpecWithoutAPortIsLoud: an unwired port must be a clear error,
// not a panic and not a silent success — the daemon that forgets to supply
// the implementation has to be told.
func TestOverseerSpecWithoutAPortIsLoud(t *testing.T) {
	spec := specOverseer()
	built, err := spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolreg.KernelDeps{}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"status"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError {
		t.Fatalf("an unwired overseer must report an error, got %s", res.Output)
	}
	if res.Output == "" {
		t.Fatal("the unwired overseer must say WHY it is unavailable")
	}
}
