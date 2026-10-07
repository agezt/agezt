// SPDX-License-Identifier: MIT
package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	graphs "github.com/agezt/agezt/kernel/workflow"
	"reflect"
	"testing"
	"time"
)

func runEvent(kind event.Kind, corr string, ts int64, payload any) *event.Event {
	raw, _ := json.Marshal(payload)
	return &event.Event{Subject: "workflow.owned", Kind: kind, CorrelationID: corr, TSUnixMS: ts, Payload: raw}
}
func TestWorkflowHistoryPreparationCanonicalSubjectBoundsAndEmpty(t *testing.T) {
	reader := &graphPort{items: []graphs.Workflow{{Name: "owned", ID: "owned-id"}}}
	j := &historyPort{}
	s := NewReads(reader, j, nil)
	h, err := s.PrepareRuns(" owned-id ")
	if err != nil || h.name != "owned" || !reflect.DeepEqual(reader.refs, []string{"owned-id"}) || j.reads != 0 {
		t.Fatal(h, err, reader.refs, j.reads)
	}
	reader.items[0].Name = "renamed"
	for i := 0; i < 105; i++ {
		j.events = append(j.events, runEvent(event.KindWorkflowStarted, fmt.Sprint(i), int64(105-i), nil))
	}
	for _, test := range []struct{ limit, count int }{{-1, 20}, {0, 20}, {1, 1}, {99, 99}, {100, 100}, {101, 100}, {1000, 100}} {
		out, err := h.Runs(context.Background(), RunsInput{Limit: test.limit})
		if err != nil || out.Count != test.count || len(out.Runs) != test.count || out.Workflow != "owned" || out.Runs[0].CorrelationID != "104" || out.Runs[0].Status != "running" {
			t.Fatal(test, out, err)
		}
	}
	if j.reads != 7 {
		t.Fatal(j.reads)
	}
	missing, err := s.PrepareRuns(" missing ")
	if !errors.Is(err, graphs.ErrNotFound) || err.Error() != "unknown workflow:  missing " || !reflect.DeepEqual(missing, History{}) {
		t.Fatal(missing, err)
	}
	empty := History{name: "owned", journal: &historyPort{}}
	out, err := empty.Runs(context.Background(), RunsInput{})
	wire := object(t, out)
	if err != nil || out.Runs == nil || out.Count != 0 || wire["runs"] == nil || len(wire["runs"].([]any)) != 0 {
		t.Fatal(out, wire, err)
	}
}
func TestWorkflowHistoryRetainsNodePresenceFilteringMetadataAndTerminalOrder(t *testing.T) {
	events := []*event.Event{runEvent(event.KindWorkflowStarted, "owned", 0, map[string]any{"source": "manual", "runner": "workflow", "agent": "agent", "schedule_id": "schedule", "standing_id": "standing", "trigger_subject": "subject", "parent_correlation_id": "parent"}), runEvent(event.KindWorkflowNode, "owned", 1, map[string]any{"node": "first", "type": "tool", "label": "label", "port": "error", "ok": false, "handled": false, "error": "failure", "input": "in", "output": "out", "attempts": 2}), runEvent(event.KindWorkflowNode, "owned", 2, map[string]any{"node": "minimal", "attempts": 1}), runEvent(event.KindWorkflowNode, "probe", 3, map[string]any{"node": "test", "test": true}), runEvent(event.KindWorkflowNode, "missing-node", 4, map[string]any{"type": "tool"}), runEvent(event.KindWorkflowCompleted, "terminal-only", -2, map[string]any{"executed": []any{"late", map[string]any{"raw": true}}}), runEvent(event.KindWorkflowFailed, "owned", 10, map[string]any{"error": "failed", "executed": []any{"first"}}), runEvent(event.KindWorkflowCompleted, "owned", 11, map[string]any{"executed": []any{"first", "minimal"}}), runEvent(event.KindWorkflowStarted, "", 20, nil), {Subject: "workflow.foreign", Kind: event.KindWorkflowStarted, CorrelationID: "foreign", TSUnixMS: 30}, {Subject: "workflow.owned", Kind: event.KindStandingCreated, CorrelationID: "irrelevant", TSUnixMS: 40}}
	j := &historyPort{events: events}
	out, err := (History{name: "owned", journal: j}).Runs(context.Background(), RunsInput{})
	if err != nil || out.Count != 2 || out.Runs[0].CorrelationID != "terminal-only" || out.Runs[1].CorrelationID != "owned" {
		t.Fatal(out, err)
	}
	terminal := object(t, out.Runs[0])
	for _, key := range []string{"correlation_id", "status", "started_ms", "node_events"} {
		if _, ok := terminal[key]; !ok {
			t.Fatal(key, terminal)
		}
	}
	if terminal["started_ms"] != float64(0) || terminal["node_events"] != nil {
		t.Fatal(terminal)
	}
	if _, ok := terminal["finished_ms"]; ok {
		t.Fatal(terminal)
	}
	if len(out.Runs[0].Executed) != 2 {
		t.Fatal(out)
	}
	owned := out.Runs[1]
	if owned.Status != "completed" || owned.Error != "failed" || owned.FinishedMS != 11 || owned.StartedMS != 0 || len(owned.NodeEvents) != 2 || owned.Source != "manual" || owned.Runner != "workflow" || owned.Agent != "agent" || owned.ScheduleID != "schedule" || owned.StandingID != "standing" || owned.TriggerSubject != "subject" || owned.ParentCorrelation != "parent" {
		t.Fatal(owned)
	}
	node := object(t, owned.NodeEvents[0])
	if node["ok"] != false || node["handled"] != false || node["attempts"] != float64(2) || node["input"] != "in" || node["output"] != "out" || node["type"] != "tool" || node["label"] != "label" || node["port"] != "error" || node["error"] != "failure" {
		t.Fatal(node)
	}
	minimal := object(t, owned.NodeEvents[1])
	if !reflect.DeepEqual(minimal, map[string]any{"node": "minimal", "ts_ms": float64(2), "ok": true}) {
		t.Fatal(minimal)
	}
	// Starts overwrite metadata/time but do not clear an earlier terminal status,
	// finish or error; matching legacy fold order is part of this migration.
	j.events = append(j.events, runEvent(event.KindWorkflowStarted, "owned", 99, nil))
	out, err = (History{name: "owned", journal: j}).Runs(context.Background(), RunsInput{})
	owned = out.Runs[1]
	if err != nil || owned.Status != "completed" || owned.FinishedMS != 11 || owned.Error != "failed" || owned.StartedMS != 99 || owned.Source != "" || owned.ParentCorrelation != "" {
		t.Fatal(owned, err)
	}
}
func TestWorkflowHistoryRetainsPartialDecodeAndJournalCause(t *testing.T) {
	j := &historyPort{events: []*event.Event{{Subject: "workflow.owned", Kind: event.KindWorkflowStarted, CorrelationID: "malformed", TSUnixMS: 5, Payload: json.RawMessage(`{broken`)}, {Subject: "workflow.owned", Kind: event.KindWorkflowNode, CorrelationID: "malformed-node", Payload: json.RawMessage(`{broken`)}, {Subject: "workflow.owned", Kind: event.KindWorkflowNode, CorrelationID: "partial", Payload: json.RawMessage(`{"node":"partial","ok":false,"attempts":"wrong","handled":false}`)}}}
	out, err := (History{name: "owned", journal: j}).Runs(context.Background(), RunsInput{})
	if err != nil || out.Count != 2 || out.Runs[0].CorrelationID != "partial" || len(out.Runs[0].NodeEvents) != 1 || out.Runs[0].NodeEvents[0].OK || out.Runs[0].NodeEvents[0].Handled == nil || *out.Runs[0].NodeEvents[0].Handled || out.Runs[1].StartedMS != 5 || out.Runs[1].Source != "" {
		t.Fatal(out, err)
	}
	cause := errors.New("owned range cause")
	j.cause = cause
	out, err = (History{name: "owned", journal: j}).Runs(context.Background(), RunsInput{})
	if !errors.Is(err, cause) || err.Error() != "journal: owned range cause" || !reflect.DeepEqual(out, RunsOutput{}) {
		t.Fatal(out, err)
	}
}
func TestWorkflowHistoryNodeZeroFieldsRemainRequired(t *testing.T) {
	wire := object(t, NodeEvent{Node: "zero", OK: false})
	if !reflect.DeepEqual(wire, map[string]any{"node": "zero", "ts_ms": float64(0), "ok": false}) {
		t.Fatal(wire)
	}
}
func TestWorkflowHistoryReadsOwnedActualJournal(t *testing.T) {
	now := int64(0)
	j, err := journal.Open(t.TempDir(), journal.Options{Now: func() time.Time { return time.UnixMilli(now) }})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	for i, kind := range []event.Kind{event.KindWorkflowStarted, event.KindWorkflowNode, event.KindWorkflowCompleted} {
		now = int64(i)
		payload := map[string]any{}
		if kind == event.KindWorkflowNode {
			payload["node"] = "owned"
			payload["ok"] = false
			payload["handled"] = false
		}
		if _, err := j.Append(event.Spec{Subject: "workflow.owned", Kind: kind, Actor: "fixture", CorrelationID: "owned", Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	out, err := (History{name: "owned", journal: j}).Runs(context.Background(), RunsInput{Limit: 1})
	if err != nil || out.Count != 1 || out.Runs[0].StartedMS != 0 || out.Runs[0].FinishedMS != 2 || out.Runs[0].Status != "completed" || out.Runs[0].NodeEvents[0].OK {
		t.Fatal(out, err)
	}
}
