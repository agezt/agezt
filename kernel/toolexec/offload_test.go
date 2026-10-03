// SPDX-License-Identifier: MIT

package toolexec_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/toolexec"
)

type offloadStore struct {
	data  []byte
	calls int
	mode  string
}

func (s *offloadStore) Put(b []byte) (string, error) {
	s.calls++
	s.data = append([]byte(nil), b...)
	if s.mode == "fail" {
		return "", errors.New("store failed")
	}
	if s.mode == "empty" {
		return "", nil
	}
	return "blob", nil
}

type fullOutputNoise struct {
	calls  int
	result toolapi.Result
}

func (n *fullOutputNoise) NotifyNoise(_ context.Context, _ llm.ToolCall, res toolapi.Result) {
	n.calls++
	n.result = res
}

func TestRunWithOptions_OffloadsOnlyAudit(t *testing.T) {
	for _, mode := range []string{"success", "reported-error", "invoke-error", "panic", "deny"} {
		t.Run(mode, func(t *testing.T) {
			full := strings.Repeat("DATA", 5000)
			cause := errors.New(full)
			calls := 0
			tool := &fakeTool{def: toolapi.ToolDef{Name: "dump"}, invoke: func(context.Context, json.RawMessage) (toolapi.Result, error) {
				calls++
				switch mode {
				case "invoke-error":
					return toolapi.Result{}, cause
				case "panic":
					panic(full)
				}
				return toolapi.Result{Output: full, IsError: mode == "reported-error"}, nil
			}}
			store, noise, events := &offloadStore{}, &fullOutputNoise{}, &mockEvents{}
			res, err := toolexec.RunWithOptions(context.Background(), "corr", "call", "dump", json.RawMessage(`{}`), mockLookup{"dump": tool}, &mockPolicy{verdict: agent.PolicyVerdict{Allow: mode != "deny", Reason: full}}, events, noise, toolexec.Options{Artifacts: store})
			want := full
			if mode == "panic" {
				want = "tool invocation panicked: " + full
			}
			if mode == "deny" {
				want = "tool call denied by policy: " + full
			}
			if mode == "success" || mode == "reported-error" {
				if err != nil || res.Output != full || res.IsError != (mode == "reported-error") {
					t.Fatalf("caller bytes=%d error=%v", len(res.Output), err)
				}
			} else if err == nil {
				t.Fatal("terminal cause lost")
			}
			if mode == "invoke-error" && !errors.Is(err, cause) {
				t.Fatal("error identity lost")
			}
			wantCalls := 1
			wantEvents := 3
			if mode == "deny" {
				wantCalls = 0
				wantEvents = 2
				if res.Output != want || noise.calls != 0 {
					t.Error("denial result/hook changed")
				}
			}
			if calls != wantCalls || len(events.published) != wantEvents {
				t.Fatalf("calls=%d events=%d", calls, len(events.published))
			}
			if mode != "deny" && (noise.calls != 1 || noise.result.Output != want) {
				t.Error("hook received preview instead of full output")
			}
			p := events.published[len(events.published)-1]
			payload := p.Payload.(map[string]any)
			if p.Kind != event.KindToolResult || p.CorrelationID != "corr" || payload["call_id"] != "call" || payload["raw_ref"] != "blob" || payload["output_bytes"] != len(want) || payload["error"] != (mode != "success") {
				t.Fatalf("terminal metadata=%v", payload)
			}
			if string(store.data) != want || store.calls != 1 || len(payload["output"].(string)) >= len(want) {
				t.Error("offload representation/store bytes incorrect")
			}
		})
	}
}

func TestRunWithOptions_InlineAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		threshold  int
		configured bool
		off        bool
	}{
		{"default", "", 0, true, true}, {"custom", "", 16, true, true}, {"boundary", "", 20000, true, false},
		{"no-store", "", 0, false, false}, {"put-error", "fail", 0, true, false}, {"empty-ref", "empty", 0, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			full := strings.Repeat("x", 20000)
			store := &offloadStore{mode: tc.mode}
			options := toolexec.Options{ArtifactThreshold: tc.threshold}
			if tc.configured {
				options.Artifacts = store
			}
			tool := &fakeTool{def: toolapi.ToolDef{Name: "dump"}, invoke: func(context.Context, json.RawMessage) (toolapi.Result, error) {
				return toolapi.Result{Output: full}, nil
			}}
			events := &mockEvents{}
			res, err := toolexec.RunWithOptions(context.Background(), "corr", "call", "dump", json.RawMessage(`{}`), mockLookup{"dump": tool}, &mockPolicy{verdict: agent.PolicyVerdict{Allow: true}}, events, &mockNoise{}, options)
			if err != nil || res.Output != full {
				t.Fatal("caller output changed")
			}
			p := events.published[2].Payload.(map[string]any)
			_, off := p["raw_ref"]
			if off != tc.off {
				t.Fatalf("offloaded=%v", off)
			}
			if !off && (p["output"] != full || p["output_bytes"] != nil) {
				t.Error("inline fallback changed")
			}
		})
	}
}

func TestRunWithOptions_ArtifactDoesNotMaskAuditCause(t *testing.T) {
	cause := errors.New(strings.Repeat("error", 3000))
	audit := errors.New("journal unavailable")
	tool := &fakeTool{def: toolapi.ToolDef{Name: "dump"}, invoke: func(context.Context, json.RawMessage) (toolapi.Result, error) { return toolapi.Result{}, cause }}
	store := &offloadStore{}
	events := &mockEvents{failKind: event.KindToolResult, failure: audit}
	_, err := toolexec.RunWithOptions(context.Background(), "corr", "call", "dump", json.RawMessage(`{}`), mockLookup{"dump": tool}, &mockPolicy{verdict: agent.PolicyVerdict{Allow: true}}, events, &mockNoise{}, toolexec.Options{Artifacts: store})
	if !errors.Is(err, cause) || !errors.Is(err, audit) || string(store.data) != cause.Error() {
		t.Errorf("causes/store changed: %v", err)
	}
}
