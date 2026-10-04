// SPDX-License-Identifier: MIT

package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type admissionProbe struct{ calls atomic.Int32 }

func (*admissionProbe) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "probe", InputSchema: json.RawMessage(`{"type":"object","properties":{"n":{"type":"integer"}},"required":["n"]}`), Effect: agent.ToolEffect{Class: agent.EffectReadOnly}}
}

func TestRun_PolicyAuditFailureStopsWholeBatch(t *testing.T) {
	b, j := newTestBus(t)
	tool := &admissionProbe{}
	policyCalls := 0
	prov := mock.New(agent.CompletionResponse{Message: agent.Message{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "c1", Name: "probe", Input: json.RawMessage(`{"n":1}`)}, {ID: "c2", Name: "probe", Input: json.RawMessage(`{"n":2}`)}}}, StopReason: agent.StopToolUse}, mock.FinalText("must not run"))
	_, err := agent.Run(context.Background(), agent.LoopConfig{Provider: prov, Tools: map[string]agent.Tool{"probe": tool}, Bus: b, Actor: "actor", CorrelationID: "corr", Policy: func(context.Context, agent.ToolCall) agent.PolicyVerdict {
		policyCalls++
		if policyCalls == 2 {
			b.Close()
		}
		return agent.PolicyVerdict{Allow: true}
	}}, "run")
	if !errors.Is(err, bus.ErrClosed) || !strings.Contains(err.Error(), "publish policy.decision") || policyCalls != 2 || tool.calls.Load() != 0 {
		t.Fatalf("error=%v policy=%d backend=%d", err, policyCalls, tool.calls.Load())
	}
	policyRecords, invoked, result := 0, 0, 0
	if err := j.Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindPolicyDecision:
			policyRecords++
		case event.KindToolInvoked:
			invoked++
		case event.KindToolResult:
			result++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if policyRecords != 1 || invoked != 1 || result != 0 {
		t.Errorf("policy=%d invoked=%d result=%d", policyRecords, invoked, result)
	}
}
func (p *admissionProbe) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	p.calls.Add(1)
	return agent.Result{Output: "ok"}, nil
}

func TestRun_BatchAdmissionPrecedesEffects(t *testing.T) {
	for _, parallel := range []int{1, 4} {
		for _, denySecond := range []bool{false, true} {
			t.Run(fmt.Sprintf("parallel=%d/deny=%t", parallel, denySecond), func(t *testing.T) {
				b, j := newTestBus(t)
				tool := &admissionProbe{}
				entered, release := make(chan struct{}), make(chan struct{})
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				prov := mock.New(agent.CompletionResponse{Message: agent.Message{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "c1", Name: "probe", Input: json.RawMessage(`{"n":1}`)}, {ID: "c2", Name: "probe", Input: json.RawMessage(`{"n":2}`)}}}, StopReason: agent.StopToolUse}, mock.FinalText("done"))
				done := make(chan error, 1)
				go func() {
					_, err := agent.Run(ctx, agent.LoopConfig{Provider: prov, Tools: map[string]agent.Tool{"probe": tool}, Bus: b, Actor: "actor", CorrelationID: "corr", MaxParallelTools: parallel, Policy: func(ctx context.Context, tc agent.ToolCall) agent.PolicyVerdict {
						if tc.ID == "c2" {
							close(entered)
							select {
							case <-release:
							case <-ctx.Done():
							}
						}
						return agent.PolicyVerdict{Allow: tc.ID != "c2" || !denySecond, Capability: "introspect", Reason: "decision"}
					}}, "run")
					done <- err
				}()
				select {
				case <-entered:
				case <-time.After(3 * time.Second):
					t.Fatal("second policy was not reached")
				}
				if tool.calls.Load() != 0 {
					t.Errorf("tool effects started before batch admission: %d", tool.calls.Load())
				}
				var prefix []string
				if err := j.Range(func(e *event.Event) error {
					if e.Kind == event.KindPolicyDecision || e.Kind == event.KindToolInvoked {
						if e.Actor != "actor" || e.CorrelationID != "corr" {
							t.Errorf("caller envelope: actor=%s correlation=%s", e.Actor, e.CorrelationID)
						}
						var p struct {
							CallID string `json:"call_id"`
						}
						if err := json.Unmarshal(e.Payload, &p); err != nil {
							return err
						}
						prefix = append(prefix, string(e.Kind)+":"+p.CallID)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if fmt.Sprint(prefix) != fmt.Sprint([]string{"policy.decision:c1", "tool.invoked:c1"}) {
					t.Errorf("admission prefix=%v", prefix)
				}
				close(release)
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("run did not finish")
				}
				wantCalls := int32(2)
				if denySecond {
					wantCalls = 1
				}
				if tool.calls.Load() != wantCalls {
					t.Errorf("calls=%d want %d", tool.calls.Load(), wantCalls)
				}
				var arc []string
				if err := j.Range(func(e *event.Event) error {
					if e.Kind == event.KindPolicyDecision || e.Kind == event.KindToolInvoked || e.Kind == event.KindToolResult {
						var p struct {
							CallID string `json:"call_id"`
						}
						if err := json.Unmarshal(e.Payload, &p); err != nil {
							return err
						}
						arc = append(arc, string(e.Kind)+":"+p.CallID)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				want := []string{"policy.decision:c1", "tool.invoked:c1", "policy.decision:c2"}
				if !denySecond {
					want = append(want, "tool.invoked:c2")
				}
				want = append(want, "tool.result:c1", "tool.result:c2")
				if fmt.Sprint(arc) != fmt.Sprint(want) {
					t.Errorf("arc=%v want %v", arc, want)
				}
			})
		}
	}
}

func TestRun_MemoDoesNotBypassLaterPolicy(t *testing.T) {
	b, j := newTestBus(t)
	tool := &admissionProbe{}
	policyCalls := 0
	prov := mock.New(testToolUse("c1", "probe", map[string]any{"n": 1}), testToolUse("c2", "probe", map[string]any{"n": 1}), mock.FinalText("done"))
	_, err := agent.Run(context.Background(), agent.LoopConfig{Provider: prov, Tools: map[string]agent.Tool{"probe": tool}, Bus: b, Actor: "actor", CorrelationID: "corr", ToolMemo: agent.NewToolMemo(time.Minute, 8), Policy: func(context.Context, agent.ToolCall) agent.PolicyVerdict {
		policyCalls++
		return agent.PolicyVerdict{Allow: policyCalls == 1, Reason: "later denial", EffectClass: string(agent.EffectReadOnly)}
	}}, "run")
	if err != nil {
		t.Fatal(err)
	}
	if policyCalls != 2 || tool.calls.Load() != 1 {
		t.Fatalf("policy=%d backend=%d", policyCalls, tool.calls.Load())
	}
	found := false
	if err := j.Range(func(e *event.Event) error {
		if e.Kind != event.KindToolResult {
			return nil
		}
		var p struct {
			CallID  string `json:"call_id"`
			Error   bool
			Output  string
			MemoHit bool `json:"memo_hit"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if p.CallID == "c2" {
			found = true
			if !p.Error || p.MemoHit || p.Output != "tool call denied by policy: later denial" {
				t.Errorf("denied memo result=%s", e.Payload)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("later denial result missing")
	}
}
