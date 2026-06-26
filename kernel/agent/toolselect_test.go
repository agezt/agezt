// SPDX-License-Identifier: MIT

package agent_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type discoveryTool struct {
	name string
	desc string
}

func (t discoveryTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: t.name, Description: t.desc, InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (t discoveryTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	return agent.Result{Output: t.name + " ok"}, nil
}

func TestLexicalToolSelector_PicksRelevantTools(t *testing.T) {
	tools := []agent.ToolDef{
		{Name: "calendar", Description: "Create and inspect calendar events", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "file", Description: "Read, write, and search files in the workspace", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "notify", Description: "Send a message to a person", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	sel := agent.LexicalToolSelector(1)
	got, err := sel(context.Background(), agent.ToolSelectionRequest{
		Intent: "read the project file and summarize it",
		Tools:  tools,
	})
	if err != nil {
		t.Fatalf("selector: %v", err)
	}
	if len(got) != 1 || got[0].Name != "file" {
		t.Fatalf("selected %v, want only file", toolDefNames(got))
	}
}

func TestLexicalToolSelector_NoMatchReturnsAll(t *testing.T) {
	tools := []agent.ToolDef{
		{Name: "calendar", Description: "Create calendar events", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "notify", Description: "Send a message", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	sel := agent.LexicalToolSelector(1)
	got, err := sel(context.Background(), agent.ToolSelectionRequest{Intent: "solve the thing", Tools: tools})
	if err != nil {
		t.Fatalf("selector: %v", err)
	}
	if len(got) != len(tools) {
		t.Fatalf("selected %v, want all tools when there is no positive match", toolDefNames(got))
	}
}

func TestDeferredLexicalToolSelector_PinsSearchAndHidesNoMatch(t *testing.T) {
	tools := []agent.ToolDef{
		{Name: "tool_search", Description: "Search deferred tool catalog", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "calendar", Description: "Create calendar events", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "notify", Description: "Send a message", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	sel := agent.DeferredLexicalToolSelector(1, []string{"tool_search"})
	got, err := sel(context.Background(), agent.ToolSelectionRequest{Intent: "solve the thing", Tools: tools})
	if err != nil {
		t.Fatalf("selector: %v", err)
	}
	if len(got) != 1 || got[0].Name != "tool_search" {
		t.Fatalf("selected %v, want only pinned tool_search", toolDefNames(got))
	}
}

func TestDeferredLexicalToolSelector_PinsSearchWithRelevantTool(t *testing.T) {
	tools := []agent.ToolDef{
		{Name: "tool_search", Description: "Search deferred tool catalog", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "calendar", Description: "Create calendar events", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "file", Description: "Read and write files", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	sel := agent.DeferredLexicalToolSelector(1, []string{"tool_search"})
	got, err := sel(context.Background(), agent.ToolSelectionRequest{Intent: "read notes.txt", Tools: tools})
	if err != nil {
		t.Fatalf("selector: %v", err)
	}
	if len(got) != 2 || got[0].Name != "tool_search" || got[1].Name != "file" {
		t.Fatalf("selected %v, want [tool_search file]", toolDefNames(got))
	}
}

func TestRun_ToolSelectorFiltersProviderRequest(t *testing.T) {
	b, j := newTestBus(t)
	var offered []agent.ToolDef
	prov := mock.New(mock.FinalText("done"))
	prov.OnRequest = func(req agent.CompletionRequest) {
		offered = append([]agent.ToolDef(nil), req.Tools...)
	}
	if _, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: prov,
		Tools: map[string]agent.Tool{
			"calendar": discoveryTool{name: "calendar", desc: "Create calendar events"},
			"file":     discoveryTool{name: "file", desc: "Read and write files"},
		},
		ToolSelector:  agent.LexicalToolSelector(1),
		Bus:           b,
		Actor:         "agent-discovery",
		CorrelationID: "corr-discovery",
	}, "read notes.txt from the workspace"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(offered) != 1 || offered[0].Name != "file" {
		t.Fatalf("provider tools = %v, want only file", toolDefNames(offered))
	}

	var sawDiscovery bool
	_ = j.Range(func(e *event.Event) error {
		if e.Kind != event.KindLLMRequest {
			return nil
		}
		var p struct {
			ToolDiscovery       bool `json:"tool_discovery"`
			ToolsBeforeDiscover int  `json:"tools_before_discovery"`
			Tools               int  `json:"tools"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		if p.ToolDiscovery && p.ToolsBeforeDiscover == 2 && p.Tools == 1 {
			sawDiscovery = true
		}
		return nil
	})
	if !sawDiscovery {
		t.Fatal("llm.request did not journal tool discovery counts")
	}
}

func toolDefNames(defs []agent.ToolDef) []string {
	out := make([]string, len(defs))
	for i, d := range defs {
		out[i] = d.Name
	}
	return out
}
