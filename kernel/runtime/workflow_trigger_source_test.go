// SPDX-License-Identifier: MIT

package runtime_test

// The proof that a triggered run says WHY it ran. A workflow fired by the
// cron/event trigger runner used to be journalled as source:"manual" — the
// trigger never told the host what fired it, so every automatic run looked
// like a human had pressed the button. The host now carries the trigger's
// source into the run's provenance.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/workflows/workflow"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// workflowStartedPayload returns the workflow.started event of one run.
func workflowStartedPayload(t *testing.T, k *runtime.Kernel, name string) map[string]any {
	t.Helper()
	var started map[string]any
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "workflow."+name || e.Kind != event.KindWorkflowStarted {
			return nil
		}
		_ = json.Unmarshal(e.Payload, &started)
		return nil
	})
	if started == nil {
		t.Fatalf("no workflow.started event for %q", name)
	}
	return started
}

// TestFireWorkflow_CronTriggerIsJournalledAsCron: a run the cron trigger
// fired is journalled as source:"cron" — NOT as "manual", which is what the
// trigger-less run is.
func TestFireWorkflow_CronTriggerIsJournalledAsCron(t *testing.T) {
	k := openWorkflowKernel(t, mock.New(mock.FinalText("unused")), &echoTool{})
	saveFlow(t, k, workflow.Workflow{
		Name: "cron-fired-flow",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "shape", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"ok"}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "shape"}},
	})
	w, _ := k.Workflows().Get("cron-fired-flow")
	k.FireWorkflow(context.Background(), w, nil, workflow.Trigger{Source: "cron", Reason: "cron interval"})

	started := workflowStartedPayload(t, k, "cron-fired-flow")
	if started["source"] != "cron" {
		t.Fatalf("cron-triggered run source = %v, want cron", started["source"])
	}
	if started["runner"] != "cron" {
		t.Fatalf("cron-triggered run runner = %v, want cron", started["runner"])
	}
}

// TestFireWorkflow_EventTriggerIsJournalledAsEvent: a run the event trigger
// fired is journalled as source:"event" and names the matched subject.
func TestFireWorkflow_EventTriggerIsJournalledAsEvent(t *testing.T) {
	k := openWorkflowKernel(t, mock.New(mock.FinalText("unused")), &echoTool{})
	saveFlow(t, k, workflow.Workflow{
		Name: "event-fired-flow",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "shape", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"ok"}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "shape"}},
	})
	w, _ := k.Workflows().Get("event-fired-flow")
	k.FireWorkflow(context.Background(), w, nil, workflow.Trigger{Source: "event", Subject: "task.failed", Reason: "event task.failed"})

	started := workflowStartedPayload(t, k, "event-fired-flow")
	if started["source"] != "event" {
		t.Fatalf("event-triggered run source = %v, want event", started["source"])
	}
	if started["trigger_subject"] != "task.failed" {
		t.Fatalf("event-triggered run trigger_subject = %v, want task.failed", started["trigger_subject"])
	}
}

// TestRunWorkflow_WithoutATriggerStaysManual: a run nobody triggered is
// still "manual" — the fix labels the triggered runs, it does not relabel
// the hand-pressed ones.
func TestRunWorkflow_WithoutATriggerStaysManual(t *testing.T) {
	k := openWorkflowKernel(t, mock.New(mock.FinalText("unused")), &echoTool{})
	saveFlow(t, k, workflow.Workflow{
		Name: "hand-run-flow",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "shape", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"ok"}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "shape"}},
	})
	if _, err := k.RunWorkflow(context.Background(), k.NewCorrelation(), "hand-run-flow", nil); err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}
	started := workflowStartedPayload(t, k, "hand-run-flow")
	if started["source"] != "manual" {
		t.Fatalf("hand-run source = %v, want manual", started["source"])
	}
}
