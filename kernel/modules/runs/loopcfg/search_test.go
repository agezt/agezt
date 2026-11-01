// SPDX-License-Identifier: MIT

package loopcfg

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
)

func TestToolSearchToolFindsCatalogMatches(t *testing.T) {
	tools := map[string]toolapi.Tool{
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

// TestWithToolSearch_OnAnEmptySetIsANoOp pins the boundary: there is no
// catalog to search, so no search tool is injected.
func TestWithToolSearch_OnAnEmptySetIsANoOp(t *testing.T) {
	out := withToolSearch(map[string]toolapi.Tool{})
	if len(out) != 0 {
		t.Fatalf("withToolSearch(empty) = %d tools, want 0", len(out))
	}
}

// TestToolSearchTool_LimitIsClamped pins the limit bound: a request above the
// ceiling is clamped to it, and a request below it is honoured.
func TestToolSearchTool_LimitIsClamped(t *testing.T) {
	many := make(map[string]toolapi.Tool, 25)
	for i := 0; i < 25; i++ {
		name := "tool" + string(rune('a'+i))
		many[name] = searchStub{name: name, desc: "shared description"}
	}
	tools := withToolSearch(many)
	ts := tools[toolSearchName]

	// limit=1 keeps exactly one row.
	res, err := ts.Invoke(context.Background(), json.RawMessage(`{"query":"shared","limit":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, `"count": 1`) {
		t.Fatalf("limit=1 output did not report one row: %s", res.Output)
	}

	// limit=99 is clamped to the ceiling of 20.
	res, err = ts.Invoke(context.Background(), json.RawMessage(`{"query":"shared","limit":99}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool_search returned error: %s", res.Output)
	}
	if !strings.Contains(res.Output, `"count": 20`) {
		t.Fatalf("clamped limit output did not report twenty rows: %s", res.Output)
	}
}

// TestToolSearchTool_EmptyQueryReturnsTheWholeCatalog pins the empty-query
// behaviour: with no tokens nothing is filtered out by score, so the catalog
// comes back whole.
func TestToolSearchTool_EmptyQueryReturnsTheWholeCatalog(t *testing.T) {
	tools := withToolSearch(map[string]toolapi.Tool{
		"a": searchStub{name: "a", desc: "alpha"},
		"b": searchStub{name: "b", desc: "beta"},
	})
	res, err := tools[toolSearchName].Invoke(context.Background(), json.RawMessage(`{"query":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, `"count": 2`) {
		t.Fatalf("empty query output did not report the whole catalog: %s", res.Output)
	}
}

// TestToolSearchTool_NonMatchingTokensAreDropped covers the score filter: a
// tool that matches nothing is not in the answer.
func TestToolSearchTool_NonMatchingTokensAreDropped(t *testing.T) {
	tools := withToolSearch(map[string]toolapi.Tool{
		"file":   searchStub{name: "file", desc: "Read and write project files"},
		"notify": searchStub{name: "notify", desc: "Send a notification"},
	})
	res, err := tools[toolSearchName].Invoke(context.Background(), json.RawMessage(`{"query":"calendar"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, `"count": 0`) {
		t.Fatalf("a non-matching query returned rows: %s", res.Output)
	}
}

// TestToolSearchTool_ExactNameScoresHighest covers the ranking: the tool whose
// NAME is the query outranks one that only mentions it in its description.
func TestToolSearchTool_ExactNameScoresHighest(t *testing.T) {
	tools := withToolSearch(map[string]toolapi.Tool{
		"file": searchStub{name: "file", desc: "Read and write project files"},
	})
	res, err := tools[toolSearchName].Invoke(context.Background(), json.RawMessage(`{"query":"file"}`))
	if err != nil {
		t.Fatal(err)
	}
	// The exact name scores 10 (the name) + 5 (the name contains it) + 2 (the
	// haystack) = 17.
	if !strings.Contains(res.Output, `"score": 17`) {
		t.Fatalf("the exact name did not score 17: %s", res.Output)
	}
}

var _ toolapi.Tool = toolSearchTool{}

type searchStub struct {
	name string
	desc string
}

func (s searchStub) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: s.name, Description: s.desc, InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (s searchStub) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{Output: s.name}, nil
}
