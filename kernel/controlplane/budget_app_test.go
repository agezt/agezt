// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestBudgetBindsTheGovernor drives both typed operations through the native
// adapter, so every snapshot field the governor reports reaches the wire.
func TestBudgetBindsTheGovernor(t *testing.T) {
	reg := governor.NewRegistry()
	if err := reg.Register(&governor.ProviderInfo{Name: "mock", Provider: mock.New(llm.CompletionResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}, StopReason: llm.StopEndTurn, Usage: llm.Usage{InputTokens: 1000, OutputTokens: 500, Model: "claude-sonnet-4-6"}}), AuthMode: governor.AuthAPIKey}); err != nil {
		t.Fatal(err)
	}
	gov, err := governor.New(governor.Config{Registry: reg, DailyCeilingMicrocents: 1_000_000_000, StrictPricing: true, TaskBudgets: map[string]int64{"plan": 100_000_000, "code": 500_000_000}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gov.Complete(context.Background(), llm.CompletionRequest{Model: "claude-sonnet-4-6", TaskType: "code"}); err != nil {
		t.Fatal(err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: gov})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	result := func(cmd string, args map[string]any) string {
		t.Helper()
		resp := callAppHost(t, s, Request{ID: "b", Cmd: cmd, Token: "primary", Args: args})[0]
		if resp.Type != RespResult {
			t.Fatal(cmd, resp.Error)
		}
		raw, err := json.Marshal(resp.Result)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	date := gov.Snapshot().UTCDate
	if got := result(CmdBudget, nil); got != `{"ceiling_mc":1000000000,"per_task":[{"ceiling_mc":500000000,"spent_mc":1050000,"task_type":"code"},{"ceiling_mc":100000000,"spent_mc":0,"task_type":"plan"}],"spent_mc":1050000,"strict_pricing":true,"utc_date":"`+date+`"}` {
		t.Fatal(got)
	}
	if got := result(CmdBudgetSet, map[string]any{"ceiling_mc": "-7"}); got != `{"ceiling_mc":0,"per_task":[{"ceiling_mc":500000000,"spent_mc":1050000,"task_type":"code"},{"ceiling_mc":100000000,"spent_mc":0,"task_type":"plan"}],"spent_mc":1050000,"strict_pricing":true,"utc_date":"`+date+`"}` {
		t.Fatal("a negative ceiling is clamped to unlimited", got)
	}
	if gov.DailyCeilingMicrocents() != 0 {
		t.Fatal(gov.DailyCeilingMicrocents())
	}
	for cmd, read := range map[string]bool{CmdBudget: true, CmdBudgetSet: false} {
		if wire, exists := commandRegistry[cmd]; !exists || !wire.AppOwned || wire.ReadOnly != read || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire %+v", cmd, wire)
		}
	}
}
