// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// goodDraftJSON is a copilot answer wrapped in prose + a markdown fence —
// the extractor must dig the object out of exactly this kind of noise.
const goodDraftJSON = "Here is your workflow:\n```json\n" + `{
  "name": "model-pick",
  "description": "fetch then brief",
  "nodes": [
    {"id": "start", "type": "trigger", "config": {"kind": "manual"}},
    {"id": "fetch", "type": "tool", "label": "Fetch", "config": {"tool": "echo", "args": {"q": "{{trigger.payload.q}}"}}},
    {"id": "brief", "type": "llm", "label": "Brief", "config": {"prompt": "summarize {{fetch.output}}"}}
  ],
  "edges": [
    {"from": "start", "to": "fetch"},
    {"from": "fetch", "to": "brief"}
  ]
}` + "\n```\nEnjoy!"

// TestDraftWorkflow_DesignsValidatedGraph: prose+fenced answer → validated,
// auto-laid-out, name-overridden, journaled — and NOT saved.
func TestDraftWorkflow_DesignsValidatedGraph(t *testing.T) {
	var req agent.CompletionRequest
	prov := mock.New(mock.FinalText(goodDraftJSON))
	prov.OnRequest = func(r agent.CompletionRequest) { req = r }
	k := openWorkflowKernel(t, prov, &echoTool{out: "x"})

	var mu sync.Mutex
	drafted := 0
	sub, err := k.Bus().Subscribe("workflow.>", 16)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Cancel()
	saw := make(chan struct{}, 1)
	go func() {
		for ev := range sub.C {
			if ev.Kind == event.KindWorkflowDrafted {
				mu.Lock()
				drafted++
				mu.Unlock()
				saw <- struct{}{}
				return
			}
		}
	}()

	w, err := k.DraftWorkflow(context.Background(), k.NewCorrelation(), "my-pick", "fetch a thing then brief it")
	if err != nil {
		t.Fatalf("DraftWorkflow: %v", err)
	}
	if w.Name != "my-pick" { // the canvas's name wins over the model's
		t.Fatalf("name = %q", w.Name)
	}
	if len(w.Nodes) != 3 || len(w.Edges) != 2 {
		t.Fatalf("graph shape = %d nodes / %d edges", len(w.Nodes), len(w.Edges))
	}
	// The copilot's contract rode in the request.
	if req.TaskType != "workflow" || !strings.Contains(req.System, "Node types") {
		t.Fatalf("request not copilot-shaped: task=%q", req.TaskType)
	}
	// Auto-layout: depth becomes the row.
	pos := map[string][2]float64{}
	for _, n := range w.Nodes {
		pos[n.ID] = [2]float64{n.X, n.Y}
	}
	if !(pos["start"][1] < pos["fetch"][1] && pos["fetch"][1] < pos["brief"][1]) {
		t.Fatalf("layout rows not by depth: %v", pos)
	}
	// Drafted, not saved.
	if got := len(k.Workflows().List()); got != 0 {
		t.Fatalf("draft must not be saved; store has %d", got)
	}
	<-saw
	mu.Lock()
	defer mu.Unlock()
	if drafted != 1 {
		t.Fatalf("workflow.drafted events = %d", drafted)
	}
}

// TestDraftWorkflow_RepairRound: a first answer that fails validation goes
// back to the model with the exact error; the corrected second answer wins.
func TestDraftWorkflow_RepairRound(t *testing.T) {
	bad := `{"name":"oops","nodes":[{"id":"start","type":"trigger"},{"id":"x","type":"transform","config":{"template":"hi"}}],"edges":[{"from":"start","to":"x"},{"from":"x","to":"start"}]}`
	var prompts []string
	prov := mock.New(mock.FinalText(bad), mock.FinalText(goodDraftJSON))
	prov.OnRequest = func(r agent.CompletionRequest) { prompts = append(prompts, r.Messages[0].Content) }
	k := openWorkflowKernel(t, prov, &echoTool{out: "x"})

	w, err := k.DraftWorkflow(context.Background(), k.NewCorrelation(), "", "loop forever")
	if err != nil {
		t.Fatalf("DraftWorkflow after repair: %v", err)
	}
	if w.Name != "model-pick" { // no override → the model's name stands
		t.Fatalf("name = %q", w.Name)
	}
	if prov.CallCount() != 2 {
		t.Fatalf("calls = %d, want 2", prov.CallCount())
	}
	// The repair prompt carried the validation error and the bad answer.
	if !strings.Contains(prompts[1], "rejected") || !strings.Contains(prompts[1], bad) {
		t.Fatalf("repair prompt missing context: %q", prompts[1])
	}
}

// TestDraftWorkflow_GivesUpAfterRepair: two bad answers → the last
// validation error surfaces; nothing is journaled as drafted.
func TestDraftWorkflow_GivesUpAfterRepair(t *testing.T) {
	prov := mock.New(mock.FinalText("no json here at all"), mock.FinalText("still chatting, sorry"))
	k := openWorkflowKernel(t, prov, &echoTool{out: "x"})

	_, err := k.DraftWorkflow(context.Background(), k.NewCorrelation(), "x", "do something")
	if err == nil || !strings.Contains(err.Error(), "no JSON object") {
		t.Fatalf("err = %v", err)
	}
	if prov.CallCount() != 2 {
		t.Fatalf("calls = %d, want 2", prov.CallCount())
	}
}

// TestDraftWorkflow_RequiresDescription: blank in, refused without a
// provider call.
func TestDraftWorkflow_RequiresDescription(t *testing.T) {
	prov := mock.New()
	k := openWorkflowKernel(t, prov, &echoTool{out: "x"})
	if _, err := k.DraftWorkflow(context.Background(), "", "x", "   "); err == nil {
		t.Fatal("want error for empty description")
	}
	if prov.CallCount() != 0 {
		t.Fatalf("provider called %d times for an empty description", prov.CallCount())
	}
}
