// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestToolSearchToolFindsCatalogMatches(t *testing.T) {
	tools := map[string]agent.Tool{
		"calendar": searchStub{name: "calendar", desc: "Create calendar events"},
		"file":     searchStub{name: "file", desc: "Read and write project files"},
		"notify":   searchStub{name: "notify", desc: "Send a notification"},
	}
	tools = withToolSearch(tools)
	ts, ok := tools[toolSearchName]
	if !ok {
		t.Fatal("tool_search was not installed")
	}
	res, err := ts.Invoke(context.Background(), json.RawMessage(`{"query":"read project files","limit":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool_search returned error: %s", res.Output)
	}
	if !strings.Contains(res.Output, `"name": "file"`) {
		t.Fatalf("tool_search output did not include file: %s", res.Output)
	}
	if strings.Contains(res.Output, `"name": "tool_search"`) {
		t.Fatalf("tool_search must not include itself in its catalog: %s", res.Output)
	}
}

func TestWithToolSearchDoesNotOverrideExistingTool(t *testing.T) {
	custom := toolSet(toolSearchName)
	out := withToolSearch(custom)
	if len(out) != 1 {
		t.Fatalf("existing tool_search should not be duplicated, got %d tools", len(out))
	}
	if _, ok := out[toolSearchName].(stubTool); !ok {
		t.Fatalf("existing tool_search should be preserved, got %T", out[toolSearchName])
	}
}

var _ agent.Tool = toolSearchTool{}

type searchStub struct {
	name string
	desc string
}

func (s searchStub) Definition() agent.ToolDef {
	return agent.ToolDef{Name: s.name, Description: s.desc, InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (s searchStub) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	return agent.Result{Output: s.name}, nil
}
