// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/policyapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolphaseapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type observedLoopPhases struct {
	toolapi.Invoker
	toolphaseapi.Phases
	counts     [5]atomic.Int32
	mu         sync.Mutex
	executions map[string]string
}

func (p *observedLoopPhases) Resolve(call llm.ToolCall, lookup func(string) (toolapi.Tool, bool)) toolphaseapi.Resolution {
	p.counts[0].Add(1)
	return p.Phases.Resolve(call, lookup)
}
func (p *observedLoopPhases) Decide(ctx context.Context, call llm.ToolCall, def toolapi.ToolDef, policy policyapi.Policy, audit func(llm.ToolCall, policyapi.PolicyVerdict) error) (toolphaseapi.Decision, error) {
	p.counts[1].Add(1)
	return p.Phases.Decide(ctx, call, def, policy, audit)
}
func (p *observedLoopPhases) Announce(call llm.ToolCall, publish func(string, map[string]any) error) error {
	p.counts[2].Add(1)
	return p.Phases.Announce(call, publish)
}
func (p *observedLoopPhases) Execute(ctx context.Context, tool toolapi.Tool, input json.RawMessage, timeout time.Duration, panicError func(any) error) toolphaseapi.Execution {
	p.counts[3].Add(1)
	p.mu.Lock()
	p.executions[tool.Definition().Name] = toolapi.CorrelationFromContext(ctx)
	p.mu.Unlock()
	return p.Phases.Execute(ctx, tool, input, timeout, panicError)
}
func (p *observedLoopPhases) Settle(call llm.ToolCall, result toolapi.Result, fields map[string]any, publish func(string, map[string]any) error) error {
	p.counts[4].Add(1)
	return p.Phases.Settle(call, result, fields, publish)
}
func observedAppPhases(deps toolpipeline.Dependencies) *observedLoopPhases {
	service := apptools.NewInvoker(deps)
	return &observedLoopPhases{Invoker: service, Phases: service.(toolphaseapi.Phases), executions: map[string]string{}}
}
func (p *observedLoopPhases) totals() []int32 {
	return []int32{p.counts[0].Load(), p.counts[1].Load(), p.counts[2].Load(), p.counts[3].Load(), p.counts[4].Load()}
}

func TestKernelLoopUsesBoundPhasesAndIsolatesPolicy(t *testing.T) {
	for _, deny := range []bool{false, true} {
		t.Run(map[bool]string{false: "primary", true: "tenant-deny"}[deny], func(t *testing.T) {
			tool := &workflowAuditTool{name: "probe"}
			provider := mock.New(testToolUse("call", "probe", map[string]any{}), mock.FinalText("done"))
			var observed *observedLoopPhases
			factories := 0
			k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), TenantID: map[bool]string{false: "", true: "tenant"}[deny], Provider: provider, Tools: map[string]toolapi.Tool{"probe": tool}, NewToolInvoker: func(deps toolpipeline.Dependencies) toolapi.Invoker {
				factories++
				observed = observedAppPhases(deps)
				return observed
			}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { k.Close() })
			if deny {
				k.Edict().SetLevel(edict.CapIntrospect, edict.LevelDeny)
			}
			if got, err := k.RunWith(context.Background(), "phase-run", "use probe"); err != nil || got != "done" {
				t.Fatalf("answer=%q error=%v", got, err)
			}
			want := []int32{1, 1, 1, 1, 1}
			wantCalls := 1
			if deny {
				want = []int32{1, 1, 0, 0, 1}
				wantCalls = 0
			}
			if !reflect.DeepEqual(observed.totals(), want) || tool.calls != wantCalls || factories != 1 {
				t.Fatalf("phases=%v want=%v backend=%d factory=%d", observed.totals(), want, tool.calls, factories)
			}
			if k.BuildLoopConfig(context.Background(), "other", "").ToolPhases != observed {
				t.Fatal("loop config replaced bound service")
			}
			results := 0
			if err := k.Journal().Range(func(e *event.Event) error {
				if e.Kind == event.KindToolResult && e.CorrelationID == "phase-run" {
					results++
					var p struct {
						CallID string `json:"call_id"`
						Error  bool
					}
					if err := json.Unmarshal(e.Payload, &p); err != nil {
						return err
					}
					if p.CallID != "call" || p.Error != deny {
						t.Errorf("terminal=%s", e.Payload)
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if results != 1 {
				t.Errorf("terminal records=%d", results)
			}
		})
	}
}

func TestDelegatedLoopUsesSameBoundPhases(t *testing.T) {
	provider := mock.New(testToolUse("parent", "delegate", map[string]any{"task": "use probe"}), testToolUse("child", "probe", map[string]any{}), mock.FinalText("child done"), mock.FinalText("parent done"))
	tool := &workflowAuditTool{name: "probe"}
	var observed *observedLoopPhases
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: provider, SubAgentTool: true, SubAgentMaxDepth: 1, Tools: map[string]toolapi.Tool{"probe": tool}, NewToolInvoker: func(deps toolpipeline.Dependencies) toolapi.Invoker {
		observed = observedAppPhases(deps)
		return observed
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	if got, err := k.RunWith(context.Background(), "parent-run", "delegate the probe"); err != nil || got != "parent done" {
		t.Fatalf("answer=%q error=%v", got, err)
	}
	if !reflect.DeepEqual(observed.totals(), []int32{2, 2, 2, 2, 2}) || tool.calls != 1 {
		t.Fatalf("phases=%v backend=%d", observed.totals(), tool.calls)
	}
	observed.mu.Lock()
	defer observed.mu.Unlock()
	if observed.executions["delegate"] != "parent-run" || observed.executions["probe"] == "" || observed.executions["probe"] == "parent-run" {
		t.Fatalf("run contexts=%v", observed.executions)
	}
}

func TestOneShotFactoryKeepsCanonicalLoopCompatibility(t *testing.T) {
	provider := mock.New(testToolUse("call", "probe", map[string]any{}), mock.FinalText("done"))
	tool := &workflowAuditTool{name: "probe"}
	var calls atomic.Int32
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: provider, Tools: map[string]toolapi.Tool{"probe": tool}, NewToolInvoker: func(deps toolpipeline.Dependencies) toolapi.Invoker {
		return &observedInvoker{delegate: apptools.NewInvoker(deps), calls: &calls}
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	one := k.BuildLoopConfig(context.Background(), "one", "").ToolPhases
	two := k.BuildLoopConfig(context.Background(), "two", "").ToolPhases
	if one == nil || one != two {
		t.Fatal("compatibility phase service is not bound once per kernel")
	}
	if _, err := k.RunWith(context.Background(), "compat-run", "use probe"); err != nil {
		t.Fatal(err)
	}
	if tool.calls != 1 || calls.Load() != 0 {
		t.Fatalf("loop backend=%d one-shot port=%d", tool.calls, calls.Load())
	}
}
