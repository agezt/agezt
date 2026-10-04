// SPDX-License-Identifier: MIT

package toolpipeline_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/policyapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/policyctx"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
	"github.com/agezt/agezt/kernel/toolexec"
)

var _ toolexec.Dependencies = toolpipeline.Dependencies{}
var _ toolpipeline.Factory = toolexec.Factory(nil)
var _ toolexec.Options = toolpipeline.Options{}

type probe struct {
	calls int
	cause error
}

func (*probe) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: "probe", InputSchema: json.RawMessage(`{"type":"object","properties":{"target":{"type":"string"}},"required":["target"]}`), Capability: toolapi.ToolCapability{Name: "introspect"}}
}
func (p *probe) Invoke(ctx context.Context, _ json.RawMessage) (toolapi.Result, error) {
	p.calls++
	if toolapi.CorrelationFromContext(ctx) != "corr" {
		return toolapi.Result{}, errors.New("lost correlation")
	}
	if def, ok := policyctx.PolicyToolDefFromContext(ctx); !ok || def.Name != "probe" {
		return toolapi.Result{}, errors.New("lost execution metadata")
	}
	return toolapi.Result{Output: strings.Repeat("x", 512)}, p.cause
}

type lookup struct{ tool *probe }

func (l lookup) LookupTool(name string) (toolapi.Tool, bool) { return l.tool, name == "probe" }

type policy struct {
	allow      bool
	calls      int
	definition toolapi.ToolDef
}

func (p *policy) CheckPolicy(ctx context.Context, _ llm.ToolCall) policyapi.PolicyVerdict {
	p.calls++
	p.definition, _ = policyctx.PolicyToolDefFromContext(ctx)
	return policyapi.PolicyVerdict{Allow: p.allow, Reason: "decision", Capability: "introspect", AffectedResources: []string{"resource"}, EpistemicConfidence: 0.75}
}

type audit struct {
	records []event.Spec
	fail    event.Kind
	cause   error
}

func (a *audit) PublishEvent(e event.Spec) error {
	if e.Kind == a.fail {
		return a.cause
	}
	a.records = append(a.records, e)
	return nil
}

type noise struct {
	calls  int
	result toolapi.Result
}

func (n *noise) NotifyNoise(_ context.Context, _ llm.ToolCall, res toolapi.Result) {
	n.calls++
	n.result = res
}

type store struct{ data []byte }

func (s *store) Put(data []byte) (string, error) {
	s.data = append([]byte(nil), data...)
	return "blob", nil
}

func TestPipelineAdmissionAndAudit(t *testing.T) {
	for _, mode := range []string{"schema", "deny", "preflight-audit", "success", "invoke-error", "joined-error"} {
		t.Run(mode, func(t *testing.T) {
			backendCause := errors.New("typed backend cause")
			auditCause := errors.New("typed audit cause")
			tool := &probe{}
			pol := &policy{allow: mode != "deny"}
			events := &audit{}
			hook := &noise{}
			artifacts := &store{}
			input := json.RawMessage(`{"target":"ok"}`)
			if mode == "schema" {
				input = json.RawMessage(`{}`)
			}
			if mode == "preflight-audit" {
				events.fail = event.KindPolicyDecision
				events.cause = auditCause
			}
			if mode == "invoke-error" || mode == "joined-error" {
				tool.cause = backendCause
			}
			if mode == "joined-error" {
				events.fail = event.KindToolResult
				events.cause = auditCause
			}
			res, err := toolpipeline.RunWithOptions(context.Background(), "corr", "call", "probe", input, lookup{tool}, pol, events, hook, toolpipeline.Options{Artifacts: artifacts, ArtifactThreshold: 16})
			switch mode {
			case "schema":
				if err == nil || !strings.Contains(err.Error(), "$.target is required") || pol.calls != 0 || tool.calls != 0 || len(events.records) != 0 || hook.calls != 0 {
					t.Fatalf("schema boundary: err=%v policy=%d tool=%d events=%d hook=%d", err, pol.calls, tool.calls, len(events.records), hook.calls)
				}
				return
			case "deny":
				if err == nil || tool.calls != 0 || len(events.records) != 2 || hook.calls != 0 {
					t.Fatalf("deny boundary: err=%v tool=%d events=%d hook=%d", err, tool.calls, len(events.records), hook.calls)
				}
			case "preflight-audit":
				if !errors.Is(err, auditCause) || tool.calls != 0 || hook.calls != 0 {
					t.Fatalf("preflight boundary: %v calls=%d hook=%d", err, tool.calls, hook.calls)
				}
				return
			case "invoke-error", "joined-error":
				if !errors.Is(err, backendCause) || hook.calls != 1 {
					t.Fatalf("backend cause/hook: %v hook=%d", err, hook.calls)
				}
				if mode == "joined-error" && !errors.Is(err, auditCause) {
					t.Fatalf("joined audit cause lost: %v", err)
				}
			default:
				if err != nil || res.Output != strings.Repeat("x", 512) || hook.result.Output != res.Output || len(artifacts.data) != 512 {
					t.Fatalf("full caller/hook/artifact: result=%+v error=%v hook=%+v artifact=%d", res, err, hook.result, len(artifacts.data))
				}
			}
			if pol.definition.Name != "probe" {
				t.Errorf("resolved metadata=%+v", pol.definition)
			}
			p := events.records[0].Payload.(map[string]any)
			if len(p) != 23 || p["call_id"] != "call" || p["epistemic_confidence"] != 0.75 {
				t.Errorf("policy=%v", p)
			}
			for _, e := range events.records {
				if e.CorrelationID != "corr" {
					t.Errorf("event correlation=%s", e.CorrelationID)
				}
			}
		})
	}
}
