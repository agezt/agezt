// SPDX-License-Identifier: MIT

package workflowapi

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestTheWorkflowWireShape: a workflow is PERSISTED (`workflows.json`) and
// read back by key — the CLI, the console canvas and the Web UI all read it.
// So the row's JSON keys are a contract with every workflow file already on
// disk, and a renamed key would silently drop that field.
func TestTheWorkflowWireShape(t *testing.T) {
	w := Workflow{
		ID:          "w1",
		Name:        "greeter",
		Enabled:     true,
		Description: "says hi",
		Nodes: []Node{{
			ID: "start", Type: NodeTrigger,
			Config: json.RawMessage(`{"kind":"cron","daily_at":"09:00"}`),
			X:      1.5, Y: 2.5,
			TimeoutSec: 30, Retries: 2, RetryDelaySec: 5,
		}},
		Edges:     []Edge{{From: "start", To: "greet", Port: "true"}},
		CreatedMS: 10,
		UpdatedMS: 20,
	}
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"id", "name", "enabled", "description", "nodes", "edges", "created_ms", "updated_ms"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("the wire must carry %q: %s", key, raw)
		}
	}
	nodes, _ := got["nodes"].([]any)
	if len(nodes) != 1 {
		t.Fatalf("the wire must carry the nodes: %s", raw)
	}
	node, _ := nodes[0].(map[string]any)
	for _, key := range []string{"id", "type", "config", "x", "y", "timeout_sec", "retries", "retry_delay_sec"} {
		if _, ok := node[key]; !ok {
			t.Fatalf("the node wire must carry %q: %s", key, raw)
		}
	}
	edges, _ := got["edges"].([]any)
	if len(edges) != 1 {
		t.Fatalf("the wire must carry the edges: %s", raw)
	}
	edge, _ := edges[0].(map[string]any)
	for _, key := range []string{"from", "to", "port"} {
		if _, ok := edge[key]; !ok {
			t.Fatalf("the edge wire must carry %q: %s", key, raw)
		}
	}
}

// TestTheOptionalFieldsFallOutOfTheWire: an empty optional field must not
// appear — a stored workflow with `"description": ""` would be a different
// file than one without the key.
func TestTheOptionalFieldsFallOutOfTheWire(t *testing.T) {
	raw, err := json.Marshal(Workflow{Name: "x", Nodes: []Node{{ID: "a", Type: NodeTrigger}}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"description", "edges"} {
		if _, ok := got[key]; ok {
			t.Fatalf("an empty %q must fall out of the wire: %s", key, raw)
		}
	}
	for _, key := range []string{"id", "name", "enabled", "nodes", "created_ms", "updated_ms"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("the wire must always carry %q: %s", key, raw)
		}
	}
	node := got["nodes"].([]any)[0].(map[string]any)
	for _, key := range []string{"label", "config", "x", "y", "timeout_sec", "retries", "retry_delay_sec"} {
		if _, ok := node[key]; ok {
			t.Fatalf("an empty node %q must fall out of the wire: %s", key, raw)
		}
	}
}

// TestTheNodeTypesAreTheWire: the node type is what a stored graph says it
// is — a renamed type would orphan every workflow that uses it.
func TestTheNodeTypesAreTheWire(t *testing.T) {
	for _, c := range []struct {
		got  string
		want string
	}{
		{NodeTrigger, "trigger"},
		{NodeTool, "tool"},
		{NodeLLM, "llm"},
		{NodeCondition, "condition"},
		{NodeTransform, "transform"},
		{NodeDelay, "delay"},
		{NodeHTTP, "http"},
		{NodeCode, "code"},
		{NodeMap, "map"},
		{NodeFilter, "filter"},
		{NodeSwitch, "switch"},
		{NodeMerge, "merge"},
		{NodeApproval, "approval"},
		{NodeSubflow, "subworkflow"},
		{NodePipeline, "pipeline"},
	} {
		if c.got != c.want {
			t.Errorf("node type = %q, want %q", c.got, c.want)
		}
	}
}

// TestTheCapsAreTheValues: the limits the rule enforces are numbers the
// daemon promises. A test that measures a cap against the constant itself
// would pass any value at all, so the numbers are pinned here.
func TestTheCapsAreTheValues(t *testing.T) {
	if maxNodes != 100 {
		t.Errorf("maxNodes = %d, want 100", maxNodes)
	}
	if maxEdges != 300 {
		t.Errorf("maxEdges = %d, want 300", maxEdges)
	}
	if maxConfigBytes != 32*1024 {
		t.Errorf("maxConfigBytes = %d, want %d", maxConfigBytes, 32*1024)
	}
	if maxDelaySeconds != 600 {
		t.Errorf("maxDelaySeconds = %d, want 600", maxDelaySeconds)
	}
	if minIntervalSec != 30 {
		t.Errorf("minIntervalSec = %d, want 30", minIntervalSec)
	}
	if minWebhookSecretLen != 12 {
		t.Errorf("minWebhookSecretLen = %d, want 12", minWebhookSecretLen)
	}
	if maxSwitchCases != 16 {
		t.Errorf("maxSwitchCases = %d, want 16", maxSwitchCases)
	}
	if maxPipelineSteps != 16 {
		t.Errorf("maxPipelineSteps = %d, want 16", maxPipelineSteps)
	}
}

// TestValidateJudgesTheCapsAtTheirBoundary: a cap is a limit, so the
// boundary is the contract — a cap that is only measured at cap+1 would also
// pass an implementation that had no cap at all.
func TestValidateJudgesTheCapsAtTheirBoundary(t *testing.T) {
	// Nodes: at the cap the graph is accepted, one more is refused.
	atCap := graph(append([]Node{trigger()}, manyNodes(99)...), nil)
	if err := Validate(atCap); err != nil {
		t.Fatalf("a graph of exactly %d nodes must pass: %v", maxNodes, err)
	}
	overCap := graph(append([]Node{trigger()}, manyNodes(100)...), nil)
	if err := Validate(overCap); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("a graph over the node cap must be refused, got %v", err)
	}

	// Edges: a DAG over the tool nodes — every edge points forward, so the
	// graph stays acyclic while it carries as many edges as it needs.
	atCap = graph(append([]Node{trigger()}, manyNodes(99)...), dagEdges(300))
	if err := Validate(atCap); err != nil {
		t.Fatalf("a graph of exactly %d edges must pass: %v", maxEdges, err)
	}
	overCap = graph(append([]Node{trigger()}, manyNodes(99)...), dagEdges(301))
	if err := Validate(overCap); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("a graph over the edge cap must be refused, got %v", err)
	}

	// A node's config: at the cap it passes, one byte more does not.
	const prefix = `{"tool":"shell","args":{"command":"`
	const suffix = `"}}`
	atCap = graph([]Node{trigger(), {ID: "a", Type: NodeTool, Config: json.RawMessage(prefix + strings.Repeat("x", maxConfigBytes-len(prefix)-len(suffix)) + suffix)}}, nil)
	if len(atCap.Nodes[1].Config) != maxConfigBytes {
		t.Fatalf("the fixture must be at the cap, got %d", len(atCap.Nodes[1].Config))
	}
	if err := Validate(atCap); err != nil {
		t.Fatalf("a config of exactly the cap must pass: %v", err)
	}
	overCap = graph([]Node{trigger(), {ID: "a", Type: NodeTool, Config: json.RawMessage(prefix + strings.Repeat("x", maxConfigBytes-len(prefix)-len(suffix)+1) + suffix)}}, nil)
	if err := Validate(overCap); err == nil || !strings.Contains(err.Error(), "config exceeds") {
		t.Fatalf("a config over the cap must be refused, got %v", err)
	}
}

// TestValidateJudgesTheNameLengthAtItsBoundary: the name and node-id
// patterns' length limits are limits — a name of exactly 64 characters must
// pass, one more must not.
func TestValidateJudgesTheNameLengthAtItsBoundary(t *testing.T) {
	atLimit := graph([]Node{trigger()}, nil)
	atLimit.Name = strings.Repeat("a", 64)
	if err := Validate(atLimit); err != nil {
		t.Fatalf("a name of exactly 64 characters must pass: %v", err)
	}
	overLimit := atLimit
	overLimit.Name = strings.Repeat("a", 65)
	if err := Validate(overLimit); err == nil || !strings.Contains(err.Error(), "name must match") {
		t.Fatalf("a name of 65 characters must be refused, got %v", err)
	}

	atLimit = graph([]Node{trigger(), {ID: strings.Repeat("a", 64), Type: NodeTool, Config: json.RawMessage(`{"tool":"x"}`)}}, nil)
	if err := Validate(atLimit); err != nil {
		t.Fatalf("a node id of exactly 64 characters must pass: %v", err)
	}
	overLimit = graph([]Node{trigger(), {ID: strings.Repeat("a", 65), Type: NodeTool, Config: json.RawMessage(`{"tool":"x"}`)}}, nil)
	if err := Validate(overLimit); err == nil || !strings.Contains(err.Error(), "node id") {
		t.Fatalf("a node id of 65 characters must be refused, got %v", err)
	}
}

// manyNodes builds n distinct tool nodes (each with a valid config).
func manyNodes(n int) []Node {
	out := make([]Node, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Node{
			ID:     nodeID(i),
			Type:   NodeTool,
			Config: json.RawMessage(`{"tool":"shell","args":{"command":"ls"}}`),
		})
	}
	return out
}

// nodeID names a node deterministically ("n0", "n1", ...).
func nodeID(i int) string {
	return "n" + strings.Repeat("0", 0) + itoa(i)
}

// itoa is a tiny integer formatter (the contract test avoids extra imports).
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// dagEdges builds n acyclic edges over the tool nodes: every edge points from
// a lower to a higher index, so the graph can never hold a cycle.
func dagEdges(n int) []Edge {
	out := make([]Edge, 0, n)
	for i := 0; len(out) < n && i < 99; i++ {
		for j := i + 1; j < 99 && len(out) < n; j++ {
			out = append(out, Edge{From: nodeID(i), To: nodeID(j)})
		}
	}
	return out
}

// TestTheStorePortShape: the port is the contract the agent-facing tool
// reads through, so its method set is fixed.
func TestTheStorePortShape(t *testing.T) {
	typ := reflect.TypeOf((*Store)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Store must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 2 {
		names := make([]string, 0, typ.NumMethod())
		for i := 0; i < typ.NumMethod(); i++ {
			names = append(names, typ.Method(i).Name)
		}
		t.Fatalf("the port must have exactly Get and List, got %v", names)
	}
	get, ok := typ.MethodByName("Get")
	if !ok {
		t.Fatal("the port must have Get")
	}
	if get.Type.NumOut() != 2 || get.Type.Out(0) != reflect.TypeOf(Workflow{}) || get.Type.Out(1).Kind() != reflect.Bool {
		t.Fatalf("Get must return (Workflow, bool), got %v", get.Type)
	}
	list, ok := typ.MethodByName("List")
	if !ok {
		t.Fatal("the port must have List")
	}
	if list.Type.NumOut() != 1 || list.Type.Out(0) != reflect.TypeOf([]Workflow{}) {
		t.Fatalf("List must return []Workflow, got %v", list.Type)
	}
}
