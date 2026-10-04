// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type codeAuditRunner struct {
	mode                        string
	calls                       atomic.Int32
	corr, language, code, input string
}

func (r *codeAuditRunner) RunScript(ctx context.Context, language, code, input string) (string, bool, error) {
	calls := r.calls.Add(1)
	r.corr, r.language, r.code, r.input = toolapi.CorrelationFromContext(ctx), language, code, input
	switch r.mode {
	case "error":
		return "", false, errors.New("sandbox unavailable")
	case "result":
		return "non-zero exit", true, nil
	case "panic":
		panic("sandbox fault")
	case "retry":
		if calls == 1 {
			return "", false, errors.New("transient sandbox error")
		}
	}
	return `{"answer":42}`, false, nil
}

func newCodeAuditKernel(t *testing.T, r *codeAuditRunner, engine *edict.Engine) *runtime.Kernel {
	t.Helper()
	p := mock.New()
	p.Responder = func(req llm.CompletionRequest) llm.CompletionResponse {
		if req.Model == "worker" {
			return mock.FinalText("```python\nprint(42)\n```")
		}
		return mock.FinalText("PASS: correct")
	}
	k, err := runtime.Open(runtime.Config{NewToolInvoker: apptools.NewInvoker, BaseDir: t.TempDir(), Provider: p, ScriptRunner: r, Edict: engine, ApprovalTimeout: 5 * time.Second,
		ToolCapabilities: map[string]string{"code_exec": "introspect"}})
	if err != nil {
		t.Fatal(err)
	}
	k.SetConductorExec(r)
	t.Cleanup(func() { k.Close() })
	return k
}

func codeAuditFlow() workflow.Workflow {
	return workflow.Workflow{Name: "code-audit", Nodes: []workflow.Node{
		{ID: "start", Type: workflow.NodeTrigger},
		{ID: "calc", Type: workflow.NodeCode, Config: json.RawMessage(`{"language":"python","code":"print(42)","input":"{\"n\": {{trigger.payload.n}}}"}`)},
	}, Edges: []workflow.Edge{{From: "start", To: "calc"}}}
}

// These exercise the actual entry points, without a registered code_exec tool.
func callCodeAudit(t *testing.T, k *runtime.Kernel, ctx context.Context, path, corr string) (failed, ran bool, output any) {
	t.Helper()
	if path == "conductor" {
		res, err := k.Conduct(ctx, corr, runtime.ConductorConfig{Task: "compute", Thinker: "thinker", Worker: "worker", Verifier: "verifier", MaxRounds: 1})
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range res.Steps {
			if s.Role == "verifier" {
				if s.Exec == nil {
					t.Fatal("code verifier fell back to LLM critique")
				}
				if err := k.Journal().Range(func(e *event.Event) error {
					if e.CorrelationID != corr || e.Kind != event.KindConductorStep {
						return nil
					}
					var p struct {
						Role  string
						Round int
						Exec  bool
					}
					if err := json.Unmarshal(e.Payload, &p); err != nil {
						return err
					}
					if p.Role == "verifier" && p.Round == s.Round && p.Exec != s.Exec.Ran {
						t.Errorf("live execution flag=%v, result Ran=%v", p.Exec, s.Exec.Ran)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				return !res.Passed, s.Exec.Ran, s.Exec.Output
			}
		}
		t.Fatal("missing verifier step")
	}
	w := codeAuditFlow()
	if path == "canvas" {
		res, err := k.TestWorkflowNode(ctx, corr, w, "calc", nil, map[string]any{"n": 42})
		return err != nil, false, res.Output
	}
	saveFlow(t, k, w)
	res, err := k.RunWorkflow(ctx, corr, w.Name, map[string]any{"n": 42})
	return err != nil, false, res.Outputs["calc"]
}

func assertCodeAudit(t *testing.T, k *runtime.Kernel, corr string, invoked, failed bool) map[string][]event.Kind {
	t.Helper()
	groups := map[string][]event.Kind{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID != corr || (e.Kind != event.KindPolicyDecision && e.Kind != event.KindToolInvoked && e.Kind != event.KindToolResult) {
			return nil
		}
		var p struct {
			Tool, Capability string
			CallID           string `json:"call_id"`
			Allow, Error     bool
			EffectClass      string `json:"effect_class"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if p.Tool != "code_exec" || p.CallID == "" {
			t.Errorf("bad tool identity: %s", e.Payload)
		}
		if e.Kind == event.KindPolicyDecision && (p.Capability != "code.exec" || p.Allow != invoked || p.EffectClass != string(toolapi.EffectIrreversible)) {
			t.Errorf("bad policy: %s", e.Payload)
		}
		if e.Kind == event.KindToolResult && p.Error != failed {
			t.Errorf("bad terminal result: %s", e.Payload)
		}
		groups[p.CallID] = append(groups[p.CallID], e.Kind)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := []event.Kind{event.KindPolicyDecision, event.KindToolInvoked, event.KindToolResult}
	if !invoked {
		want = []event.Kind{event.KindPolicyDecision, event.KindToolResult}
	}
	if len(groups) == 0 {
		t.Error("code execution has no policy/tool audit")
	}
	for id, arc := range groups {
		if !reflect.DeepEqual(arc, want) {
			t.Errorf("call %s: arc=%v want %v", id, arc, want)
		}
	}
	return groups
}

func TestCodeExecutionGoverned(t *testing.T) {
	for _, path := range []string{"conductor", "workflow", "canvas"} {
		for _, mode := range []string{"allow", "deny", "profile-deny", "ceiling", "error", "result", "panic"} {
			t.Run(path+"/"+mode, func(t *testing.T) {
				r := &codeAuditRunner{mode: mode}
				k := newCodeAuditKernel(t, r, nil)
				ctx := context.Background()
				denied := mode == "deny" || mode == "profile-deny" || mode == "ceiling"
				switch mode {
				case "deny":
					k.Edict().SetLevel(edict.CapCodeExec, edict.LevelDeny)
				case "profile-deny":
					ctx = runtime.WithAgentProfile(ctx, roster.Profile{Slug: "ops", ToolDeny: []string{"code_exec"}})
				case "ceiling":
					ctx = runtime.WithTrustCeiling(ctx, edict.LevelDeny)
				}
				failed, ran, out := callCodeAudit(t, k, ctx, path, "code-corr")
				if failed != (mode != "allow") {
					t.Errorf("failed=%v output=%v", failed, out)
				}
				wantCalls := int32(1)
				if denied {
					wantCalls = 0
				}
				if r.calls.Load() != wantCalls || (path == "conductor" && ran != !denied) {
					t.Errorf("calls=%d ran=%v denied=%v", r.calls.Load(), ran, denied)
				}
				if !denied && (r.corr != "code-corr" || r.language != "python" || strings.TrimSpace(r.code) != "print(42)") {
					t.Errorf("runner identity/input=%+v", r)
				}
				if !denied && path != "conductor" && r.input != `{"n": 42}` {
					t.Errorf("interpolated input=%q", r.input)
				}
				if !denied && path == "conductor" && r.input != "" {
					t.Errorf("conductor input changed: %q", r.input)
				}
				if mode == "allow" && path != "conductor" {
					if obj, ok := out.(map[string]any); !ok || obj["answer"] != float64(42) {
						t.Errorf("structured output=%v", out)
					}
				}
				if groups := assertCodeAudit(t, k, "code-corr", !denied, mode != "allow"); len(groups) != 1 {
					t.Errorf("audit groups=%v", groups)
				}
				if _, ok := k.LookupTool("code_exec"); ok {
					t.Error("invocation adapter leaked into the tool registry")
				}
			})
		}
	}
}

func TestCodeExecutionAuditUnavailable(t *testing.T) {
	for _, path := range []string{"conductor", "canvas"} {
		t.Run(path, func(t *testing.T) {
			r := &codeAuditRunner{}
			k := newCodeAuditKernel(t, r, nil)
			k.Bus().Close()
			failed, ran, _ := callCodeAudit(t, k, context.Background(), path, "closed-audit")
			if !failed || ran || r.calls.Load() != 0 {
				t.Errorf("failed=%v ran=%v calls=%d; unauditable code executed", failed, ran, r.calls.Load())
			}
		})
	}
}

func TestCodeExecutionRetryAudit(t *testing.T) {
	for _, path := range []string{"conductor", "workflow", "canvas"} {
		t.Run(path, func(t *testing.T) {
			r := &codeAuditRunner{mode: "retry"}
			k := newCodeAuditKernel(t, r, nil)
			ctx := context.Background()
			if path == "conductor" {
				res, err := k.Conduct(ctx, "retry-code", runtime.ConductorConfig{Task: "compute", Thinker: "thinker", Worker: "worker", Verifier: "verifier", MaxRounds: 2})
				if err != nil || !res.Passed || res.Rounds != 2 {
					t.Fatalf("retry result=%+v error=%v", res, err)
				}
			} else {
				w := codeAuditFlow()
				w.Nodes[1].Retries = 1
				if path == "canvas" {
					res, err := k.TestWorkflowNode(ctx, "retry-code", w, "calc", nil, map[string]any{"n": 42})
					if err != nil || res.Attempts != 2 {
						t.Fatalf("retry result=%+v error=%v", res, err)
					}
				} else {
					saveFlow(t, k, w)
					if _, err := k.RunWorkflow(ctx, "retry-code", w.Name, map[string]any{"n": 42}); err != nil {
						t.Fatal(err)
					}
				}
			}
			groups := map[string][]event.Kind{}
			failures := 0
			if err := k.Journal().Range(func(e *event.Event) error {
				if e.CorrelationID != "retry-code" || (e.Kind != event.KindPolicyDecision && e.Kind != event.KindToolInvoked && e.Kind != event.KindToolResult) {
					return nil
				}
				var p struct {
					CallID string `json:"call_id"`
					Error  bool
				}
				if err := json.Unmarshal(e.Payload, &p); err != nil {
					return err
				}
				groups[p.CallID] = append(groups[p.CallID], e.Kind)
				if e.Kind == event.KindToolResult && p.Error {
					failures++
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if r.calls.Load() != 2 || len(groups) != 2 || failures != 1 {
				t.Fatalf("calls=%d groups=%v failures=%d", r.calls.Load(), groups, failures)
			}
			for id, arc := range groups {
				if id == "" || !reflect.DeepEqual(arc, []event.Kind{event.KindPolicyDecision, event.KindToolInvoked, event.KindToolResult}) {
					t.Errorf("retry %q arc=%v", id, arc)
				}
			}
		})
	}
}

func TestCodeExecutionErrorPort(t *testing.T) {
	r := &codeAuditRunner{mode: "result"}
	k := newCodeAuditKernel(t, r, nil)
	w := codeAuditFlow()
	w.Nodes = append(w.Nodes, workflow.Node{ID: "rescue", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"rescued: {{calc.output.error}}"}`)})
	w.Edges = append(w.Edges, workflow.Edge{From: "calc", To: "rescue", Port: "error"})
	saveFlow(t, k, w)
	res, err := k.RunWorkflow(context.Background(), "code-error-port", w.Name, map[string]any{"n": 42})
	if err != nil || !strings.Contains(res.Outputs["rescue"].(string), "non-zero exit") {
		t.Fatalf("error branch result=%+v error=%v", res, err)
	}
	assertCodeAudit(t, k, "code-error-port", true, true)
}

func TestCodeExecutionApproval(t *testing.T) {
	for _, path := range []string{"conductor", "workflow", "canvas"} {
		for _, granted := range []bool{false, true} {
			name := "deny"
			if granted {
				name = "grant"
			}
			t.Run(path+"/"+name, func(t *testing.T) {
				r := &codeAuditRunner{}
				engine := edict.New(edict.Options{Levels: map[edict.Capability]edict.TrustLevel{edict.CapCodeExec: edict.LevelAskFirst}, AskPolicy: edict.AskPrompt})
				k := newCodeAuditKernel(t, r, engine)
				ctx, cancel := context.WithCancel(context.Background())
				t.Cleanup(cancel)
				ctx = runtime.WithAgentProfile(ctx, roster.Profile{Slug: "ops"})
				done := make(chan bool, 1)
				go func() { failed, _, _ := callCodeAudit(t, k, ctx, path, "code-approval"); done <- failed }()
				req := waitForPending(t, k.Approvals())
				if req.CorrelationID != "code-approval" || req.Actor != "ops" || req.Capability != "code.exec" {
					t.Errorf("approval=%+v", req)
				}
				if r.calls.Load() != 0 {
					t.Error("code ran before approval")
				}
				decision := approval.DecisionDeny
				if granted {
					decision = approval.DecisionGrant
				}
				if err := k.Approvals().Resolve(req.ID, decision, "test decision", "operator"); err != nil {
					t.Fatal(err)
				}
				select {
				case failed := <-done:
					if failed == granted {
						t.Errorf("failed=%v granted=%v", failed, granted)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("code did not finish after approval")
				}
				wantCalls := int32(0)
				if granted {
					wantCalls = 1
				}
				if r.calls.Load() != wantCalls {
					t.Errorf("calls=%d", r.calls.Load())
				}
				assertCodeAudit(t, k, "code-approval", granted, !granted)
			})
		}
	}
}

func TestCodeExecutionDistinctCallIDs(t *testing.T) {
	for _, path := range []string{"conductor", "workflow", "canvas"} {
		t.Run(path, func(t *testing.T) {
			r := &codeAuditRunner{}
			k := newCodeAuditKernel(t, r, nil)
			for range 2 {
				failed, _, _ := callCodeAudit(t, k, context.Background(), path, "same-corr")
				if failed {
					t.Fatal("execution failed")
				}
			}
			if groups := assertCodeAudit(t, k, "same-corr", true, false); len(groups) != 2 {
				t.Errorf("calls=%d groups=%v", r.calls.Load(), groups)
			}
		})
	}
}
