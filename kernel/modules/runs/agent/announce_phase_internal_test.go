// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
)

func TestGateInvocationAnnouncementBoundary(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		deny, cached, duplicate, failSecond bool
		want                                []string
	}{
		{"allow", false, false, false, false, []string{"policy.decision:c1", "tool.invoked:c1", "policy.decision:c2", "tool.invoked:c2"}},
		{"deny", true, false, false, false, []string{"policy.decision:c1", "policy.decision:c2"}},
		{"cached", false, true, true, false, []string{"policy.decision:c1", "policy.decision:c2"}},
		{"duplicate", false, false, true, false, []string{"policy.decision:c1", "tool.invoked:c1", "policy.decision:c2"}},
		{"audit-failure", false, false, false, true, []string{"policy.decision:c1", "tool.invoked:c1", "policy.decision:c2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tool := &stubTool{def: ToolDef{Name: "probe", InputSchema: objSchema(), Effect: ToolEffect{Class: EffectReadOnly}}}
			calls := []ToolCall{{ID: "c1", Name: "probe", Input: json.RawMessage(`{}`)}, {ID: "c2", Name: "probe", Input: json.RawMessage(`{"n":2}`)}}
			if tc.duplicate {
				calls[1].Input = calls[0].Input
			}
			memo := NewToolMemo(time.Minute, 8)
			if tc.cached {
				memo.Set("probe", calls[0].Input, Result{Output: "cached"})
			}
			policyCalls, invoked := 0, 0
			s := testState(LoopConfig{Tools: map[string]Tool{"probe": tool}, ToolMemo: memo, Policy: func(context.Context, ToolCall) PolicyVerdict { policyCalls++; return PolicyVerdict{Allow: !tc.deny} }})
			cause := errors.New("invocation audit unavailable")
			var arc []string
			s.publish = func(kind event.Kind, suffix string, payload any) (*event.Event, error) {
				p := payload.(map[string]any)
				id := p["call_id"].(string)
				if kind == event.KindToolInvoked {
					invoked++
					input := calls[0].Input
					if id == "c2" {
						input = calls[1].Input
					}
					if suffix != "tool" || !reflect.DeepEqual(p, map[string]any{"tool": "probe", "call_id": id, "input": input}) {
						t.Errorf("invocation envelope: suffix=%s payload=%v", suffix, p)
					}
					if tc.failSecond && id == "c2" {
						return nil, cause
					}
				}
				arc = append(arc, string(kind)+":"+id)
				return nil, nil
			}
			jobs, err := s.gateToolCalls(context.Background(), calls, 0)
			if tc.failSecond {
				if !errors.Is(err, cause) || !strings.Contains(err.Error(), "agent: publish tool.invoked:") || jobs != nil || invoked != 2 {
					t.Fatalf("error=%v jobs=%v invoked=%d", err, jobs, invoked)
				}
			} else if err != nil || len(jobs) != 2 {
				t.Fatalf("jobs=%v error=%v", jobs, err)
			}
			if policyCalls != 2 || tool.calls != 0 || !reflect.DeepEqual(arc, tc.want) {
				t.Fatalf("policy=%d effects=%d arc=%v want=%v", policyCalls, tool.calls, arc, tc.want)
			}
		})
	}
}
