// SPDX-License-Identifier: MIT

package workflowapi

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
			{ID: "pipe", Type: NodePipeline, Config: json.RawMessage(`{"steps":[{"id":"one","tool":"shell","args":{"command":"echo hi"}}]}`)},
			{ID: "think", Type: NodeLLM, Config: json.RawMessage(`{"prompt":"summarize {{shape.output}}"}`)},
			{ID: "wait", Type: NodeDelay, Config: json.RawMessage(`{"seconds":1}`)},
			// A webhook trigger with a long-enough secret, and a cron trigger
			// at the shortest interval the rule allows.
			{ID: "hook", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"webhook","secret":"123456789012"}`)},
			{ID: "tick", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"cron","interval_sec":30}`)},
		},
		[]Edge{
			{From: "start", To: "fetch"},
			{From: "fetch", To: "check"},
			{From: "check", To: "shape", Port: "true"},
			{From: "check", To: "wait", Port: "false"},
			{From: "shape", To: "pipe"},
			{From: "shape", To: "think"},
		},
	)
	// The graph above carries two trigger nodes, so it is checked in two
	// halves: each half has exactly one trigger.
	one := graph([]Node{trigger(), toolNode("fetch")}, []Edge{{From: "start", To: "fetch"}})
	if err := Validate(one); err != nil {
		t.Fatalf("valid workflow rejected: %v", err)
	}
	hook := graph([]Node{{ID: "hook", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"webhook","secret":"123456789012"}`)}}, nil)
	if err := Validate(hook); err != nil {
		t.Fatalf("a webhook trigger with a long secret must pass: %v", err)
	}
	tick := graph([]Node{{ID: "tick", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"cron","interval_sec":30}`)}}, nil)
	if err := Validate(tick); err != nil {
		t.Fatalf("a cron trigger at the minimum interval must pass: %v", err)
	}
	// A failable node may wire an "error" port — that is how a failed node
	// hands over to a branch instead of failing the run.
	failable := graph(
		[]Node{trigger(), toolNode("a"), toolNode("b")},
		[]Edge{{From: "start", To: "a"}, {From: "a", To: "b", Port: "error"}})
	if err := Validate(failable); err != nil {
		t.Fatalf("an error port on a failable node must pass: %v", err)
	}
	// A switch may wire its "default" port — the branch that runs when no
	// case matched.
	switched := graph(
		[]Node{trigger(),
			{ID: "sw", Type: NodeSwitch, Config: json.RawMessage(`{"value":"{{x}}","cases":[{"equals":"a","port":"p"}]}`)},
			toolNode("t")},
		[]Edge{{From: "start", To: "sw"}, {From: "sw", To: "t", Port: "default"}})
	if err := Validate(switched); err != nil {
		t.Fatalf("the switch's default port must pass: %v", err)
	}
	if err := Validate(w); err == nil {
		t.Fatal("two trigger nodes must be refused")
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
		{"edge from a ghost", graph([]Node{trigger()}, []Edge{{From: "ghost", To: "start"}}), "unknown node"},
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
		{"transform without a template", graph([]Node{trigger(), {ID: "a", Type: NodeTransform, Config: json.RawMessage(`{}`)}}, nil), "template"},
		// Reliability bounds (M808).
		{"node timeout out of range", graph([]Node{trigger(), {ID: "a", Type: NodeTool, TimeoutSec: 601, Config: json.RawMessage(`{"tool":"x"}`)}}, nil), "timeout_sec"},
		{"node retries out of range", graph([]Node{trigger(), {ID: "a", Type: NodeTool, Retries: 6, Config: json.RawMessage(`{"tool":"x"}`)}}, nil), "retries"},
		{"retry delay out of range", graph([]Node{trigger(), {ID: "a", Type: NodeTool, Retries: 1, RetryDelaySec: 61, Config: json.RawMessage(`{"tool":"x"}`)}}, nil), "retry_delay_sec"},
		{"retries on a node that cannot fail", graph([]Node{trigger(), {ID: "a", Type: NodeTransform, Retries: 1, Config: json.RawMessage(`{"template":"x"}`)}}, nil), "failable"},
		{"timeout on a trigger", graph([]Node{{ID: "s", Type: NodeTrigger, TimeoutSec: 30}}, nil), "timeout_sec is meaningless"},
		// Trigger configs (M799).
		{"cron without timing", graph([]Node{{ID: "s", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"cron"}`)}}, nil), "exactly one of"},
		{"cron both timings", graph([]Node{{ID: "s", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"cron","interval_sec":60,"daily_at":"09:00"}`)}}, nil), "exactly one of"},
		{"cron interval too small", graph([]Node{{ID: "s", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"cron","interval_sec":5}`)}}, nil), "interval_sec"},
		{"cron bad daily_at", graph([]Node{{ID: "s", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"cron","daily_at":"25:99"}`)}}, nil), "HH:MM"},
		{"event without subject", graph([]Node{{ID: "s", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"event"}`)}}, nil), "subject"},
		{"event on workflow.*", graph([]Node{{ID: "s", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"event","subject":"workflow.x"}`)}}, nil), "self-referential"},
		{"event on everything", graph([]Node{{ID: "s", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"event","subject":">"}`)}}, nil), "too broad"},
		{"unknown trigger kind", graph([]Node{{ID: "s", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"vibes"}`)}}, nil), "trigger kind"},
		// The webhook trigger's secret is the only gate on the
		// internet-facing hook path, so its length is the rule.
		{"webhook without secret", graph([]Node{{ID: "s", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"webhook"}`)}}, nil), "secret"},
		{"webhook with a short secret", graph([]Node{{ID: "s", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"webhook","secret":"short"}`)}}, nil), "secret"},
		{"event on everything-star", graph([]Node{{ID: "s", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"event","subject":"*"}`)}}, nil), "too broad"},
		{"cron interval below the minimum", graph([]Node{{ID: "s", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"cron","interval_sec":29}`)}}, nil), "interval_sec"},
		// M800 node library.
		{"http bad method", graph([]Node{trigger(), {ID: "a", Type: NodeHTTP, Config: json.RawMessage(`{"method":"DELETE","url":"https://x"}`)}}, nil), "GET or POST"},
		{"http no url", graph([]Node{trigger(), {ID: "a", Type: NodeHTTP, Config: json.RawMessage(`{"method":"GET"}`)}}, nil), "url"},
		{"code missing parts", graph([]Node{trigger(), {ID: "a", Type: NodeCode, Config: json.RawMessage(`{"language":"python"}`)}}, nil), "language and code"},
		{"map missing template", graph([]Node{trigger(), {ID: "a", Type: NodeMap, Config: json.RawMessage(`{"items":"{{x}}"}`)}}, nil), "items and template"},
		{"filter bad op", graph([]Node{trigger(), {ID: "a", Type: NodeFilter, Config: json.RawMessage(`{"items":"{{x}}","left":"{{item}}","op":"vibes"}`)}}, nil), "filter op"},
		{"switch no cases", graph([]Node{trigger(), {ID: "a", Type: NodeSwitch, Config: json.RawMessage(`{"value":"{{x}}","cases":[]}`)}}, nil), "cases"},
		{"switch reserved port", graph([]Node{trigger(), {ID: "a", Type: NodeSwitch, Config: json.RawMessage(`{"value":"{{x}}","cases":[{"equals":"a","port":"error"}]}`)}}, nil), "named"},
		{"switch dup port", graph([]Node{trigger(), {ID: "a", Type: NodeSwitch, Config: json.RawMessage(`{"value":"{{x}}","cases":[{"equals":"a","port":"p"},{"equals":"b","port":"p"}]}`)}}, nil), "duplicate switch port"},
		{"switch undeclared edge port", graph(
			[]Node{trigger(),
				{ID: "sw", Type: NodeSwitch, Config: json.RawMessage(`{"value":"{{x}}","cases":[{"equals":"a","port":"p"}]}`)},
				toolNode("t")},
			[]Edge{{From: "start", To: "sw"}, {From: "sw", To: "t", Port: "ghost"}}), "undeclared port"},
		{"merge bad mode", graph([]Node{trigger(), {ID: "a", Type: NodeMerge, Config: json.RawMessage(`{"mode":"most"}`)}}, nil), "merge mode"},
		{"approval no description", graph([]Node{trigger(), {ID: "a", Type: NodeApproval, Config: json.RawMessage(`{}`)}}, nil), "description"},
		{"subworkflow no name", graph([]Node{trigger(), {ID: "a", Type: NodeSubflow, Config: json.RawMessage(`{}`)}}, nil), "workflow name"},
		{"pipeline no steps", graph([]Node{trigger(), {ID: "a", Type: NodePipeline, Config: json.RawMessage(`{"steps":[]}`)}}, nil), "pipeline needs"},
		{"pipeline bad step id", graph([]Node{trigger(), {ID: "a", Type: NodePipeline, Config: json.RawMessage(`{"steps":[{"id":"Bad ID","tool":"echo"}]}`)}}, nil), "pipeline step id"},
		{"pipeline duplicate step id", graph([]Node{trigger(), {ID: "a", Type: NodePipeline, Config: json.RawMessage(`{"steps":[{"id":"x","tool":"echo"},{"id":"x","tool":"echo"}]}`)}}, nil), "duplicate pipeline step"},
		{"pipeline step without tool", graph([]Node{trigger(), {ID: "a", Type: NodePipeline, Config: json.RawMessage(`{"steps":[{"id":"x"}]}`)}}, nil), "needs a tool"},
		{"pipeline bad output schema", graph([]Node{trigger(), {ID: "a", Type: NodePipeline, Config: json.RawMessage(`{"steps":[{"id":"x","tool":"echo","output_schema":"nope"}]}`)}}, nil), "output_schema"},
		// The step's own size rule never fires: the step's schema lives
		// INSIDE the node's config, and the node's config cap is checked
		// first. So the refusal that answers an oversized step schema is the
		// node's cap.
		{"pipeline oversized output schema", graph([]Node{trigger(), {ID: "a", Type: NodePipeline, Config: json.RawMessage(`{"steps":[{"id":"x","tool":"echo","output_schema":"` + strings.Repeat("y", 33*1024) + `"}]}`)}}, nil), "config exceeds"},
		{"error port on non-failable", graph(
			[]Node{trigger(), {ID: "a", Type: NodeTransform, Config: json.RawMessage(`{"template":"x"}`)}, toolNode("t")},
			[]Edge{{From: "start", To: "a"}, {From: "a", To: "t", Port: "error"}}), "default port"},
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

// TestCanonicalDailyAt pins the normalizer that keeps a daily_at comparable.
// The runner decides "has today's time arrived?" with a STRING compare against
// now.Format("15:04"), which is always two-digit, so a stored one-digit hour
// must be padded or it sorts above every possible clock value and never fires.
func TestCanonicalDailyAt(t *testing.T) {
	cases := []struct{ in, want string }{
		// The broken class: one-digit hours must gain the leading zero.
		{"9:05", "09:05"},
		{"0:00", "00:00"},
		{"1:30", "01:30"},
		// Already canonical — must pass through untouched (no double padding).
		{"09:05", "09:05"},
		{"00:00", "00:00"},
		{"23:59", "23:59"},
		// Surrounding whitespace is what the runner already trimmed.
		{"  9:05  ", "09:05"},
		{"", ""},
		// Values Validate rejects are returned unchanged rather than silently
		// blanked: a normalizer must never disable a trigger the operator set.
		{"25:99", "25:99"},
		{"9:5", "9:5"},
		{"abc", "abc"},
		{"09:05:00", "09:05:00"},
	}
	for _, tc := range cases {
		if got := canonicalDailyAt(tc.in); got != tc.want {
			t.Errorf("canonicalDailyAt(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
