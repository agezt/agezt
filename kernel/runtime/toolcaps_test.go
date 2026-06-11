// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// quietTool is a no-op tool standing in for an out-of-process plugin tool.
type quietTool struct{ name string }

func (q quietTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: q.name, Description: "test", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (quietTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	return agent.Result{Output: "ran"}, nil
}

// TestToolCapabilities_DeclaredAxisGoverns (M900): a plugin tool whose
// manifest declares a known capability is classified — and gated — under that
// axis: with http.post at L0 (deny), the declared tool is refused while an
// undeclared sibling (unknown capability, UnknownAllow on) still runs. An
// UNKNOWN declared capability is dropped, falling back to the historical
// name classification.
func TestToolCapabilities_DeclaredAxisGoverns(t *testing.T) {
	prov := mock.New(
		agent.CompletionResponse{
			Message: agent.Message{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{
				{ID: "c1", Name: "plug.post", Input: json.RawMessage(`{}`)},
				{ID: "c2", Name: "plug.read", Input: json.RawMessage(`{}`)},
				{ID: "c3", Name: "plug.weird", Input: json.RawMessage(`{}`)},
			}},
			StopReason: agent.StopToolUse,
		},
		mock.FinalText("done"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Tools: map[string]agent.Tool{
			"plug.post":  quietTool{name: "plug.post"},
			"plug.read":  quietTool{name: "plug.read"},
			"plug.weird": quietTool{name: "plug.weird"},
		},
		ToolCapabilities: map[string]string{
			"plug.post":  "http.post", // known axis → joins it (and its deny below)
			"plug.weird": "made.up",   // unknown axis → dropped, name classification
		},
		Edict: edict.New(edict.Options{
			UnknownAllow: true, // undeclared plugin tools classify by name → allowed
			Levels:       map[edict.Capability]edict.TrustLevel{edict.CapHTTPPost: edict.LevelDeny},
		}),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Run(context.Background(), "exercise the plugin tools"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Fold policy decisions by tool.
	type decision struct {
		cap   string
		allow bool
	}
	got := map[string]decision{}
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindPolicyDecision {
			return nil
		}
		var p struct {
			Tool       string `json:"tool"`
			Capability string `json:"capability"`
			Allow      bool   `json:"allow"`
		}
		if json.Unmarshal(e.Payload, &p) == nil {
			got[p.Tool] = decision{cap: p.Capability, allow: p.Allow}
		}
		return nil
	})

	if d := got["plug.post"]; d.cap != "http.post" || d.allow {
		t.Errorf("plug.post decision = %+v, want capability http.post, denied (declared axis at L0)", d)
	}
	if d := got["plug.read"]; d.cap != "plug.read" || !d.allow {
		t.Errorf("plug.read decision = %+v, want name-classified + allowed (UnknownAllow)", d)
	}
	if d := got["plug.weird"]; d.cap != "plug.weird" || !d.allow {
		t.Errorf("plug.weird decision = %+v, want unknown declaration DROPPED → name-classified + allowed", d)
	}
}
