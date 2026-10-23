// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// TestConfigTool_WithoutADaemonHasNoOverrides: a tool with no daemon behind
// it has no agent overrides — the same posture the agent-scoped write takes
// ("agent-scoped config needs a live kernel"). The read path must not
// dereference a missing port.
func TestConfigTool_WithoutADaemonHasNoOverrides(t *testing.T) {
	tool := New(t.TempDir())
	ctx := toolapi.WithAgent(context.Background(), "researcher")

	raw, _ := json.Marshal(map[string]any{"op": "get", "scope": "agent", "name": "AGEZT_MODEL"})
	res, err := tool.Invoke(ctx, raw)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the read must not fail without a daemon: %s", res.Output)
	}
	if !strings.Contains(res.Output, "not set for agent") {
		t.Fatalf("a daemon-less tool must report no override, got %q", res.Output)
	}
}

// TestConfigTool_AgentScopedReadNeedsThePort: with a daemon behind it, the
// agent-scoped read answers the override the run carries.
func TestConfigTool_AgentScopedReadNeedsThePort(t *testing.T) {
	tool := New(t.TempDir())
	port := newFakePort()
	port.overrides = map[string]string{"AGEZT_MODEL": "agent-model"}
	tool.SetKernel(port)

	ctx := toolapi.WithAgent(context.Background(), "researcher")
	raw, _ := json.Marshal(map[string]any{"op": "get", "scope": "agent", "name": "AGEZT_MODEL"})
	res, err := tool.Invoke(ctx, raw)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the read failed: %s", res.Output)
	}
	if res.Output != "AGEZT_MODEL=agent-model (agent override: researcher)" {
		t.Fatalf("the agent-scoped read = %q", res.Output)
	}
}

// TestConfigTool_AgentScopedWriteNeedsADaemon: the write path needs a
// daemon — without one the tool must say so, not call into nothing.
func TestConfigTool_AgentScopedWriteNeedsADaemon(t *testing.T) {
	tool := New(t.TempDir())
	ctx := toolapi.WithAgent(context.Background(), "researcher")

	raw, _ := json.Marshal(map[string]any{"op": "set", "scope": "agent", "name": "AGEZT_MODEL", "value": "agent-next"})
	res, err := tool.Invoke(ctx, raw)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !res.IsError {
		t.Fatalf("a daemon-less write must be refused, got %q", res.Output)
	}
	if !strings.Contains(res.Output, "live kernel") {
		t.Fatalf("the refusal must name what is missing, got %q", res.Output)
	}
}

// TestConfigTool_UnknownAgentIsReported: the port answers "not found" and
// the tool must say which agent is unknown — not claim it saved an override.
func TestConfigTool_UnknownAgentIsReported(t *testing.T) {
	tool := New(t.TempDir())
	port := newFakePort() // no profiles: the port answers "not found"
	tool.SetKernel(port)

	ctx := toolapi.WithAgent(context.Background(), "nobody")
	raw, _ := json.Marshal(map[string]any{"op": "set", "scope": "agent", "name": "AGEZT_MODEL", "value": "agent-next"})
	res, err := tool.Invoke(ctx, raw)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !res.IsError {
		t.Fatalf("an unknown agent must be reported, got %q", res.Output)
	}
	if !strings.Contains(res.Output, "unknown agent") {
		t.Fatalf("the refusal must name the agent, got %q", res.Output)
	}
}

// TestConfigTool_LiveApplyReloadsTheDaemon: a live-apply field rebuilds the
// provider in place — the daemon is really reloaded, and the tool says so.
func TestConfigTool_LiveApplyReloadsTheDaemon(t *testing.T) {
	t.Setenv("AGEZT_MODEL", "")
	tool := New(t.TempDir())
	port := newFakePort()
	tool.SetKernel(port)

	raw, _ := json.Marshal(map[string]any{"op": "set", "name": "AGEZT_MODEL", "value": "gpt-4o"})
	res, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the write failed: %s", res.Output)
	}
	if res.Output != "AGEZT_MODEL applied live" {
		t.Fatalf("the live-apply answer = %q", res.Output)
	}
	if port.reloads != 1 {
		t.Fatalf("the daemon was reloaded %d times, want 1", port.reloads)
	}
}

// TestConfigTool_RestartFieldDoesNotReloadTheDaemon: a restart-apply field
// takes effect on the next restart — the daemon must NOT be reloaded for it.
func TestConfigTool_RestartFieldDoesNotReloadTheDaemon(t *testing.T) {
	t.Setenv("AGEZT_WEB_ADDR", "")
	tool := New(t.TempDir())
	port := newFakePort()
	tool.SetKernel(port)

	raw, _ := json.Marshal(map[string]any{"op": "set", "name": "AGEZT_WEB_ADDR", "value": "127.0.0.1:9999"})
	res, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the write failed: %s", res.Output)
	}
	if !strings.Contains(res.Output, "restart to apply") {
		t.Fatalf("a restart-apply field must say so, got %q", res.Output)
	}
	if port.reloads != 0 {
		t.Fatalf("a restart-apply field must not reload the daemon, got %d reloads", port.reloads)
	}
}

// TestConfigTool_FailedReloadIsReported: the setting is saved, but the
// operator must hear that the live apply failed — a silent success would
// leave the daemon on the old value.
func TestConfigTool_FailedReloadIsReported(t *testing.T) {
	t.Setenv("AGEZT_MODEL", "")
	tool := New(t.TempDir())
	port := newFakePort()
	port.reloadErr = errReloadFailed
	tool.SetKernel(port)

	raw, _ := json.Marshal(map[string]any{"op": "set", "name": "AGEZT_MODEL", "value": "gpt-4o"})
	res, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !res.IsError {
		t.Fatalf("a failed reload must be reported, got %q", res.Output)
	}
	if !strings.Contains(res.Output, "live reload failed") {
		t.Fatalf("the refusal must name the failure, got %q", res.Output)
	}
}
