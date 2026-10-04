// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/agezt/agezt/plugins/providers/mock"
)

type councilAuditSearch struct {
	mode        string
	calls       atomic.Int32
	correlation string
	query       string
	limit       int
}

func TestCouncilGroundingApproval(t *testing.T) {
	for _, granted := range []bool{true, false} {
		name := "deny"
		if granted {
			name = "grant"
		}
		t.Run(name, func(t *testing.T) {
			search := &councilAuditSearch{}
			var providerCalls atomic.Int32
			provider := mock.New()
			provider.OnRequest = func(llm.CompletionRequest) { providerCalls.Add(1) }
			provider.Responder = func(llm.CompletionRequest) llm.CompletionResponse {
				return mock.FinalText("CONSENSUS: agreed.\nDISSENT: none")
			}
			engine := edict.New(edict.Options{Levels: map[edict.Capability]edict.TrustLevel{edict.CapWebSearch: edict.LevelAskFirst}, AskPolicy: edict.AskPrompt})
			k, err := runtime.Open(runtime.Config{NewToolInvoker: apptools.NewInvoker, BaseDir: t.TempDir(), Provider: provider, Tools: map[string]toolapi.Tool{"web_search": search}, CouncilWebSearch: true, Edict: engine, ApprovalTimeout: 5 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { k.Close() })
			reg := k.Approvals()
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			ctx = runtime.WithAgentProfile(ctx, roster.Profile{Slug: "ops"})
			type outcome struct {
				result runtime.CouncilResult
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				result, err := k.Council(ctx, "council-approval", "test question", []runtime.CouncilMember{{Seat: "one", Model: "mock-model"}}, 1)
				done <- outcome{result, err}
			}()
			req := waitForPending(t, reg)
			if req.CorrelationID != "council-approval" || req.Actor != "ops" || req.Capability != "web.search" {
				t.Errorf("approval=%+v", req)
			}
			if search.calls.Load() != 0 || providerCalls.Load() != 0 {
				t.Error("search/panel ran before the grounding decision")
			}
			decision := approval.DecisionDeny
			if granted {
				decision = approval.DecisionGrant
			}
			if err := reg.Resolve(req.ID, decision, "test decision", "operator"); err != nil {
				t.Fatal(err)
			}
			var out outcome
			select {
			case out = <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("Council did not finish after approval resolution")
			}
			if out.err != nil || out.result.Consensus != "agreed." || out.result.AsOf == "" || (out.result.Brief != "") != granted {
				t.Fatalf("Council outcome=%+v", out)
			}
			wantSearchCalls := int32(0)
			if granted {
				wantSearchCalls = 1
			}
			if search.calls.Load() != wantSearchCalls || providerCalls.Load() != 3 {
				t.Errorf("search=%d provider=%d", search.calls.Load(), providerCalls.Load())
			}
			counts := map[event.Kind]int{}
			if err := k.Journal().Range(func(e *event.Event) error {
				if e.CorrelationID == "council-approval" {
					counts[e.Kind]++
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if counts[event.KindPolicyDecision] != 1 || counts[event.KindToolResult] != 1 || counts[event.KindToolInvoked] != int(wantSearchCalls) || counts[event.KindApprovalRequested] != 1 {
				t.Fatalf("approval audit=%v", counts)
			}
		})
	}
}

func TestCouncilGroundingDistinctSearchIDs(t *testing.T) {
	search := &councilAuditSearch{}
	provider := mock.New()
	provider.Responder = func(llm.CompletionRequest) llm.CompletionResponse { return mock.FinalText("CONSENSUS: agreed.") }
	k, err := runtime.Open(runtime.Config{NewToolInvoker: apptools.NewInvoker, BaseDir: t.TempDir(), Provider: provider, Tools: map[string]toolapi.Tool{"web_search": search}, CouncilWebSearch: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	for range 2 {
		if _, err := k.Council(context.Background(), "shared-corr", "test question", []runtime.CouncilMember{{Seat: "one", Model: "mock-model"}}, 1); err != nil {
			t.Fatal(err)
		}
	}
	ids := map[string]int{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID != "shared-corr" || e.Kind != event.KindToolResult {
			return nil
		}
		var p struct {
			CallID string `json:"call_id"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		ids[p.CallID]++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if search.calls.Load() != 2 || len(ids) != 2 {
		t.Fatalf("searches=%d IDs=%v", search.calls.Load(), ids)
	}
}

func (*councilAuditSearch) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: "web_search", Capability: toolapi.ToolCapability{Name: "web.search"},
		InputSchema: json.RawMessage(`{"type":"object","required":["query","limit"],"properties":{"query":{"type":"string"},"limit":{"type":"integer"}}}`)}
}
func (p *councilAuditSearch) Invoke(ctx context.Context, args json.RawMessage) (toolapi.Result, error) {
	p.calls.Add(1)
	p.correlation = toolapi.CorrelationFromContext(ctx)
	var input struct {
		Query string
		Limit int
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return toolapi.Result{}, err
	}
	p.query, p.limit = input.Query, input.Limit
	switch p.mode {
	case "invoke-error":
		return toolapi.Result{}, errors.New("search failed")
	case "error-result":
		return toolapi.Result{Output: `{"results":[{"title":"isolated evidence","url":"https://example.invalid/","snippet":"must not ground a failed search"}]}`, IsError: true}, nil
	case "malformed":
		return toolapi.Result{Output: "not json"}, nil
	case "panic":
		panic("search panic")
	}
	return toolapi.Result{Output: `{"results":[{"title":"isolated evidence","url":"https://example.invalid/","snippet":"test finding"}]}`}, nil
}

func TestCouncilGroundingAudit(t *testing.T) {
	for _, mode := range []string{"allow", "deny", "profile-deny", "ceiling", "invoke-error", "error-result", "malformed", "panic", "off", "missing", "nil-tool"} {
		t.Run(mode, func(t *testing.T) {
			search := &councilAuditSearch{mode: mode}
			var prompts []string
			provider := mock.New()
			provider.OnRequest = func(req llm.CompletionRequest) {
				for _, msg := range req.Messages {
					prompts = append(prompts, msg.Content)
				}
			}
			provider.Responder = func(llm.CompletionRequest) llm.CompletionResponse {
				return mock.FinalText("CONSENSUS: agreed.\nDISSENT: none")
			}
			tools := map[string]toolapi.Tool{"web_search": search}
			if mode == "missing" {
				tools = nil
			}
			if mode == "nil-tool" {
				tools["web_search"] = nil
			}
			k, err := runtime.Open(runtime.Config{NewToolInvoker: apptools.NewInvoker, BaseDir: t.TempDir(), Provider: provider, Tools: tools, CouncilWebSearch: mode != "off"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { k.Close() })
			ctx := context.Background()
			denied := mode == "deny" || mode == "profile-deny" || mode == "ceiling"
			if mode == "deny" {
				k.Edict().SetLevel(edict.CapWebSearch, edict.LevelDeny)
			}
			if mode == "profile-deny" {
				ctx = runtime.WithAgentProfile(ctx, roster.Profile{Slug: "ops", ToolDeny: []string{"web_search"}})
			}
			if mode == "ceiling" {
				ctx = runtime.WithTrustCeiling(ctx, edict.LevelDeny)
			}
			const corr = "council-audit"
			result, err := k.Council(ctx, corr, strings.Repeat("ğ", 360), []runtime.CouncilMember{{Seat: "one", Model: "mock-model"}}, 1)
			if err != nil || result.Consensus != "agreed." || result.AsOf == "" {
				t.Fatalf("Council result=%+v error=%v", result, err)
			}
			wantBrief := mode == "allow"
			if (result.Brief != "") != wantBrief {
				t.Errorf("brief=%q mode=%s", result.Brief, mode)
			}
			for _, prompt := range prompts {
				if !strings.Contains(prompt, result.AsOf) {
					t.Error("member/chair lost date grounding")
				}
				if strings.Contains(prompt, "isolated evidence") != wantBrief {
					t.Error("member/chair brief did not match the admitted search")
				}
			}
			skipped := mode == "off" || mode == "missing" || mode == "nil-tool"
			wantCalls := int32(1)
			if denied || skipped {
				wantCalls = 0
			}
			if search.calls.Load() != wantCalls {
				t.Errorf("search calls=%d want %d", search.calls.Load(), wantCalls)
			}
			if wantCalls != 0 && (search.correlation != corr || len([]rune(search.query)) != 300 || search.limit != 6) {
				t.Errorf("search request: correlation=%q query runes=%d limit=%d", search.correlation, len([]rune(search.query)), search.limit)
			}
			var arc []event.Kind
			var callID string
			var briefs int
			if err := k.Journal().Range(func(e *event.Event) error {
				if e.CorrelationID != corr {
					return nil
				}
				if e.Kind == event.KindCouncilBrief {
					briefs++
				}
				if e.Kind != event.KindPolicyDecision && e.Kind != event.KindToolInvoked && e.Kind != event.KindToolResult {
					return nil
				}
				var p struct {
					Tool       string
					CallID     string `json:"call_id"`
					Capability string
					Allow      bool
					Error      bool
				}
				if err := json.Unmarshal(e.Payload, &p); err != nil {
					return err
				}
				if callID == "" {
					callID = p.CallID
				}
				if p.Tool != "web_search" || p.CallID != callID || !strings.HasPrefix(callID, "council-search-") {
					t.Errorf("audit identity=%+v", p)
				}
				if e.Kind == event.KindPolicyDecision && (p.Capability != "web.search" || p.Allow == denied) {
					t.Errorf("decision=%+v", p)
				}
				if e.Kind == event.KindToolResult && p.Error != (denied || mode == "invoke-error" || mode == "error-result" || mode == "panic") {
					t.Errorf("terminal result=%+v", p)
				}
				arc = append(arc, e.Kind)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			wantArc := []event.Kind{event.KindPolicyDecision, event.KindToolInvoked, event.KindToolResult}
			if denied {
				wantArc = []event.Kind{event.KindPolicyDecision, event.KindToolResult}
			}
			if skipped {
				wantArc = nil
			}
			if len(arc) != len(wantArc) {
				t.Fatalf("audit arc=%v want %v", arc, wantArc)
			}
			for i := range wantArc {
				if arc[i] != wantArc[i] {
					t.Errorf("audit arc=%v want %v", arc, wantArc)
					break
				}
			}
			wantBriefs := 0
			if wantBrief {
				wantBriefs = 1
			}
			if briefs != wantBriefs {
				t.Errorf("brief events=%d want %d", briefs, wantBriefs)
			}
		})
	}
}
