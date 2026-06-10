// SPDX-License-Identifier: MIT

package workflow

import (
	"encoding/json"
	"strings"
	"testing"
)

func graph(nodes []Node, edges []Edge) Workflow {
	return Workflow{Name: "test-flow", Nodes: nodes, Edges: edges}
}

func trigger() Node { return Node{ID: "start", Type: NodeTrigger} }

func toolNode(id string) Node {
	return Node{ID: id, Type: NodeTool, Config: json.RawMessage(`{"tool":"shell","args":{"command":"ls"}}`)}
}

func TestValidate_Accepts(t *testing.T) {
	w := graph(
		[]Node{
			trigger(),
			toolNode("fetch"),
			{ID: "check", Type: NodeCondition, Config: json.RawMessage(`{"left":"{{fetch.output}}","op":"not_empty"}`)},
			{ID: "shape", Type: NodeTransform, Config: json.RawMessage(`{"template":"got: {{fetch.output}}"}`)},
			{ID: "think", Type: NodeLLM, Config: json.RawMessage(`{"prompt":"summarize {{shape.output}}"}`)},
			{ID: "wait", Type: NodeDelay, Config: json.RawMessage(`{"seconds":1}`)},
		},
		[]Edge{
			{From: "start", To: "fetch"},
			{From: "fetch", To: "check"},
			{From: "check", To: "shape", Port: "true"},
			{From: "check", To: "wait", Port: "false"},
			{From: "shape", To: "think"},
		},
	)
	if err := Validate(w); err != nil {
		t.Fatalf("valid workflow rejected: %v", err)
	}
}

func TestValidate_Rejects(t *testing.T) {
	cases := []struct {
		label string
		w     Workflow
		want  string
	}{
		{"bad name", Workflow{Name: "Bad Name", Nodes: []Node{trigger()}}, "name"},
		{"no nodes", Workflow{Name: "x"}, "at least one"},
		{"no trigger", graph([]Node{toolNode("a")}, nil), "exactly one trigger"},
		{"two triggers", graph([]Node{trigger(), {ID: "t2", Type: NodeTrigger}}, nil), "exactly one trigger"},
		{"dup id", graph([]Node{trigger(), toolNode("a"), toolNode("a")}, nil), "duplicate"},
		{"bad node id", graph([]Node{trigger(), {ID: "Bad ID", Type: NodeTool, Config: json.RawMessage(`{"tool":"x"}`)}}, nil), "node id"},
		{"unknown type", graph([]Node{trigger(), {ID: "a", Type: "spaceship"}}, nil), "unknown type"},
		{"edge to ghost", graph([]Node{trigger()}, []Edge{{From: "start", To: "ghost"}}), "unknown node"},
		{"self edge", graph([]Node{trigger(), toolNode("a")}, []Edge{{From: "a", To: "a"}}), "self-edge"},
		{"trigger with incoming", graph([]Node{trigger(), toolNode("a")}, []Edge{{From: "a", To: "start"}}), "incoming"},
		{"condition without port", graph(
			[]Node{trigger(), {ID: "c", Type: NodeCondition, Config: json.RawMessage(`{"left":"x","op":"not_empty"}`)}, toolNode("a")},
			[]Edge{{From: "start", To: "c"}, {From: "c", To: "a"}}), "port"},
		{"port on plain node", graph([]Node{trigger(), toolNode("a")}, []Edge{{From: "start", To: "a", Port: "true"}}), "default port"},
		{"cycle", graph(
			[]Node{trigger(), toolNode("a"), toolNode("b")},
			[]Edge{{From: "start", To: "a"}, {From: "a", To: "b"}, {From: "b", To: "a"}}), "cycle"},
		{"tool without name", graph([]Node{trigger(), {ID: "a", Type: NodeTool, Config: json.RawMessage(`{}`)}}, nil), "tool name"},
		{"llm without prompt", graph([]Node{trigger(), {ID: "a", Type: NodeLLM, Config: json.RawMessage(`{}`)}}, nil), "prompt"},
		{"condition bad op", graph([]Node{trigger(), {ID: "a", Type: NodeCondition, Config: json.RawMessage(`{"left":"x","op":"vibes"}`)}}, nil), "op"},
		{"delay out of range", graph([]Node{trigger(), {ID: "a", Type: NodeDelay, Config: json.RawMessage(`{"seconds":99999}`)}}, nil), "seconds"},
	}
	for _, tc := range cases {
		err := Validate(tc.w)
		if err == nil {
			t.Errorf("%s: accepted", tc.label)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %q does not mention %q", tc.label, err, tc.want)
		}
	}
}

func TestInterpolate(t *testing.T) {
	data := map[string]any{
		"trigger": map[string]any{"payload": map[string]any{"city": "izmir", "n": float64(3)}},
		"fetch":   map[string]any{"output": map[string]any{"items": []any{map[string]any{"title": "first"}, "second"}}},
		"plain":   map[string]any{"output": "hello"},
	}
	cases := []struct{ in, want string }{
		{"city={{trigger.payload.city}}", "city=izmir"},
		{"n={{trigger.payload.n}}", "n=3"},
		{"t={{fetch.output.items.0.title}}", "t=first"},
		{"s={{ fetch.output.items.1 }}", "s=second"},
		{"whole={{plain.output}}", "whole=hello"},
		{"obj={{fetch.output}}", `obj={"items":[{"title":"first"},"second"]}`},
		{"miss=[{{ghost.output}}]", "miss=[]"}, // unknown path → empty, run survives
		{"no templates here", "no templates here"},
		{"dangling {{open", "dangling {{open"},
	}
	for _, tc := range cases {
		if got := Interpolate(tc.in, data); got != tc.want {
			t.Errorf("Interpolate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestStore_UpsertByName(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	w := graph([]Node{trigger(), toolNode("a")}, []Edge{{From: "start", To: "a"}})
	saved, created, err := s.Save(w)
	if err != nil || !created || saved.ID == "" || !saved.Enabled {
		t.Fatalf("first save = %+v/%v/%v", saved, created, err)
	}

	// Disable, then upsert a new graph under the same name: identity +
	// enabled survive, the graph is replaced wholesale.
	if _, err := s.SetEnabled(saved.Name, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	w2 := w
	w2.Nodes = []Node{trigger(), toolNode("a"), toolNode("b")}
	w2.Edges = []Edge{{From: "start", To: "a"}, {From: "a", To: "b"}}
	updated, created, err := s.Save(w2)
	if err != nil || created {
		t.Fatalf("upsert = %v/%v", created, err)
	}
	if updated.ID != saved.ID || updated.Enabled || len(updated.Nodes) != 3 {
		t.Fatalf("upsert lost identity/state: %+v", updated)
	}

	// Invalid graphs never reach disk.
	bad := w
	bad.Nodes = nil
	if _, _, err := s.Save(bad); err == nil {
		t.Fatal("invalid save accepted")
	}
	if got, _ := s.Get(saved.ID); len(got.Nodes) != 3 {
		t.Fatalf("failed save mutated the store: %+v", got)
	}
}

func TestStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	s, _ := OpenStore(dir)
	if _, _, err := s.Save(graph([]Node{trigger()}, nil)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	re, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got, found := re.Get("test-flow"); !found || got.TriggerNode() == nil {
		t.Fatalf("reload = %+v/%v", got, found)
	}
	gone, ok, err := re.Remove("test-flow")
	if err != nil || !ok || gone.Name != "test-flow" || re.Count() != 0 {
		t.Fatalf("Remove = %+v/%v/%v count=%d", gone, ok, err, re.Count())
	}
}
