// SPDX-License-Identifier: MIT

package toolpipeline_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/policyapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolphaseapi"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
	"github.com/agezt/agezt/kernel/toolexec"
)

var _ toolphaseapi.Resolution = toolpipeline.Resolution{}
var _ toolphaseapi.Decision = toolpipeline.Decision{}
var _ toolphaseapi.Execution = toolpipeline.Execution{}

type phaseTrace struct {
	toolphaseapi.Phases
	steps []string
	fail  string
	cause error
}

func (p *phaseTrace) Resolve(call llm.ToolCall, lookup func(string) (toolapi.Tool, bool)) toolphaseapi.Resolution {
	p.steps = append(p.steps, "resolve")
	return p.Phases.Resolve(call, lookup)
}
func (p *phaseTrace) Decide(ctx context.Context, call llm.ToolCall, def toolapi.ToolDef, policy policyapi.Policy, audit func(llm.ToolCall, policyapi.PolicyVerdict) error) (toolphaseapi.Decision, error) {
	p.steps = append(p.steps, "decide")
	if p.fail == "decide" {
		return toolphaseapi.Decision{}, p.cause
	}
	return p.Phases.Decide(ctx, call, def, policy, audit)
}
func (p *phaseTrace) Announce(call llm.ToolCall, publish func(string, map[string]any) error) error {
	p.steps = append(p.steps, "announce")
	if p.fail == "announce" {
		return p.cause
	}
	return p.Phases.Announce(call, publish)
}
func (p *phaseTrace) Execute(ctx context.Context, tool toolapi.Tool, input json.RawMessage, timeout time.Duration, panicError func(any) error) toolphaseapi.Execution {
	p.steps = append(p.steps, "execute")
	return p.Phases.Execute(ctx, tool, input, timeout, panicError)
}
func (p *phaseTrace) Settle(call llm.ToolCall, result toolapi.Result, fields map[string]any, publish func(string, map[string]any) error) error {
	p.steps = append(p.steps, "settle")
	if p.fail == "settle" {
		return p.cause
	}
	return p.Phases.Settle(call, result, fields, publish)
}

func TestRunWithOptionsUsesInjectedPhasePort(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, mode := range []string{"allow", "deny", "schema", "decide", "announce", "settle"} {
			t.Run(mode+map[bool]string{false: "/platform", true: "/legacy"}[legacy], func(t *testing.T) {
				tool := &probe{}
				policy := &policy{allow: mode != "deny"}
				events, hook := &audit{}, &noise{}
				base := toolpipeline.NewInvoker(toolpipeline.Dependencies{Tools: lookup{tool}, Policy: policy, Events: events, Noise: hook})
				phases, ok := base.(toolphaseapi.Phases)
				if !ok {
					t.Fatal("constructed service has no phase port")
				}
				cause := errors.New("typed injected phase cause")
				trace := &phaseTrace{Phases: phases, fail: mode, cause: cause}
				input := json.RawMessage(`{"target":"ok"}`)
				if mode == "schema" {
					input = json.RawMessage(`{}`)
				}
				options := toolpipeline.Options{Phases: trace}
				var err error
				if legacy {
					_, err = toolexec.RunWithOptions(context.Background(), "corr", "call", "probe", input, lookup{tool}, policy, events, hook, options)
				} else {
					_, err = toolpipeline.RunWithOptions(context.Background(), "corr", "call", "probe", input, lookup{tool}, policy, events, hook, options)
				}
				want := []string{"resolve", "decide", "announce", "execute", "settle"}
				wantCalls := 1
				switch mode {
				case "schema":
					want = []string{"resolve"}
					wantCalls = 0
				case "deny":
					want = []string{"resolve", "decide", "settle"}
					wantCalls = 0
				case "decide":
					want = []string{"resolve", "decide"}
					wantCalls = 0
				case "announce":
					want = []string{"resolve", "decide", "announce"}
					wantCalls = 0
				}
				if !reflect.DeepEqual(trace.steps, want) || tool.calls != wantCalls {
					t.Fatalf("steps=%v want=%v effects=%d want=%d", trace.steps, want, tool.calls, wantCalls)
				}
				if mode == "allow" {
					if err != nil || hook.calls != 1 {
						t.Fatalf("error=%v hooks=%d", err, hook.calls)
					}
				} else if err == nil {
					t.Fatal("rejection or audit failure lost")
				}
				if mode == "decide" || mode == "announce" || mode == "settle" {
					if !errors.Is(err, cause) || hook.calls != 0 {
						t.Fatalf("phase cause/hook: %v hooks=%d", err, hook.calls)
					}
				}
			})
		}
	}
}

func TestConstructedPhasePortRetainsBoundLookup(t *testing.T) {
	one, two := &probe{}, &probe{}
	a := toolpipeline.NewInvoker(toolpipeline.Dependencies{Tools: lookup{one}}).(toolphaseapi.Phases)
	b := toolpipeline.NewInvoker(toolpipeline.Dependencies{Tools: lookup{two}}).(toolphaseapi.Phases)
	call := llm.ToolCall{Name: "probe", Input: json.RawMessage(`{"target":"ok"}`)}
	if a.Resolve(call, nil).Tool != one || b.Resolve(call, nil).Tool != two {
		t.Fatal("per-service registry binding lost")
	}
	if a.Resolve(call, lookup{two}.LookupTool).Tool != two {
		t.Fatal("caller lookup override lost")
	}
}
