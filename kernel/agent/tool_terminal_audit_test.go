// SPDX-License-Identifier: MIT

package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type terminalAuditTool struct {
	name   string
	mode   string
	cancel context.CancelFunc
	calls  atomic.Int32
	ctx    context.Context
}

func (x *terminalAuditTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: x.name, InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (x *terminalAuditTool) Invoke(ctx context.Context, _ json.RawMessage) (agent.Result, error) {
	x.calls.Add(1)
	x.ctx = ctx
	if x.mode == "panic" {
		panic("executor fault")
	}
	if x.mode == "cancel" {
		x.cancel()
		return agent.Result{}, fmt.Errorf("backend: %w", ctx.Err())
	}
	return agent.Result{Output: "finished"}, nil
}

func TestRun_ToolTerminalsAreAudited(t *testing.T) {
	for _, mode := range []string{"panic", "cancel"} {
		for _, parallel := range []int{1, 2} {
			for _, faultIndex := range []int{0, 1} {
				t.Run(fmt.Sprintf("%s/parallel-%d/fault-%d", mode, parallel, faultIndex), func(t *testing.T) {
					b, j := newTestBus(t)
					ctx, cancel := context.WithCancel(context.Background())
					t.Cleanup(cancel)
					tools := []*terminalAuditTool{{name: "first", cancel: cancel}, {name: "second", cancel: cancel}}
					tools[faultIndex].mode = mode
					var hooks atomic.Int32
					prov := mock.New(agent.CompletionResponse{Message: agent.Message{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{
						{ID: "one", Name: "first", Input: json.RawMessage(`{}`)},
						{ID: "two", Name: "second", Input: json.RawMessage(`{}`)},
					}}, StopReason: agent.StopToolUse})
					_, err := agent.Run(ctx, agent.LoopConfig{Provider: prov, Bus: b, Actor: "ops", CorrelationID: "terminal-run", Tools: map[string]agent.Tool{"first": tools[0], "second": tools[1]}, MaxParallelTools: parallel, ToolTimeout: time.Minute,
						ToolResultHook: func(context.Context, agent.ToolCall, agent.Result) { hooks.Add(1) }}, "test")
					want := agent.ErrPanic
					reason := "panic"
					if mode == "cancel" {
						want = context.Canceled
						reason = "canceled"
					}
					if !errors.Is(err, want) {
						t.Fatalf("run error=%v want %v", err, want)
					}
					if hooks.Load() != 0 || prov.CallCount() != 1 {
						t.Errorf("terminal turn triggered hooks=%d provider calls=%d", hooks.Load(), prov.CallCount())
					}
					counts := map[event.Kind]int{}
					var arc []event.Kind
					results := map[string]bool{}
					skipped := map[string]bool{}
					if err := j.Range(func(e *event.Event) error {
						counts[e.Kind]++
						if e.Kind == event.KindToolResult || e.Kind == event.KindTaskFailed {
							arc = append(arc, e.Kind)
						}
						if e.Kind == event.KindToolResult {
							var p struct {
								CallID      string `json:"call_id"`
								Error       bool
								NotExecuted bool `json:"not_executed"`
							}
							if err := json.Unmarshal(e.Payload, &p); err != nil {
								return err
							}
							if _, ok := results[p.CallID]; ok {
								t.Errorf("duplicate terminal %s", p.CallID)
							}
							results[p.CallID] = p.Error
							skipped[p.CallID] = p.NotExecuted
						}
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					if counts[event.KindToolInvoked] != 2 || counts[event.KindToolResult] != 2 {
						t.Errorf("invocations/terminals=%v results=%v", counts, results)
					}
					if len(arc) != 3 || arc[0] != event.KindToolResult || arc[1] != event.KindToolResult || arc[2] != event.KindTaskFailed {
						t.Errorf("terminal order=%v", arc)
					}
					faultID := []string{"one", "two"}[faultIndex]
					if !results[faultID] {
						t.Errorf("fault %s lacks failed terminal", faultID)
					}
					if mode == "panic" && parallel == 1 && faultIndex == 0 && tools[1].calls.Load() != 0 {
						t.Error("sequential run continued executing after panic")
					}
					if skipped["two"] != (mode == "panic" && parallel == 1 && faultIndex == 0) || skipped["one"] {
						t.Errorf("execution attribution=%v", skipped)
					}
					for _, tool := range tools {
						if tool.calls.Load() > 0 && tool.ctx.Err() == nil {
							t.Errorf("%s invocation context not released", tool.name)
						}
					}
					assertTaskFailed(t, j, reason)
				})
			}
		}
	}
}
