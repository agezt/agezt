// SPDX-License-Identifier: MIT
package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	graphs "github.com/agezt/agezt/kernel/workflow"
	"reflect"
	"testing"
	"time"
)

type graphPort struct {
	items []graphs.Workflow
	refs  []string
	lists int
}

func (p *graphPort) List() []graphs.Workflow { p.lists++; return p.items }
func (p *graphPort) Get(ref string) (graphs.Workflow, bool) {
	p.refs = append(p.refs, ref)
	for _, w := range p.items {
		if w.ID == ref || w.Name == ref {
			return w, true
		}
	}
	return graphs.Workflow{}, false
}

type historyPort struct {
	events []*event.Event
	cause  error
	reads  int
}

func (p *historyPort) Range(fn func(*event.Event) error) error {
	p.reads++
	for _, e := range p.events {
		if err := fn(e); err != nil {
			return err
		}
	}
	return p.cause
}
func object(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestWorkflowProjectionRetainsCountsTriggerPriorityAndFullGraph(t *testing.T) {
	for _, test := range []struct{ config, kind, detail string }{{`{}`, "manual", ""}, {`{"kind":"webhook","interval_sec":30,"daily_at":"09:00","subject":"fixture"}`, "webhook", "POST /hooks/owned"}, {`{"kind":"cron","interval_sec":30,"daily_at":"09:00","subject":"fixture"}`, "cron", "every 30s"}, {`{"kind":"cron","daily_at":"9:00","subject":"fixture"}`, "cron", "daily at 09:00"}, {`{"kind":"event","subject":"fixture"}`, "event", "on fixture"}} {
		graph := graphs.Workflow{Name: "owned", Nodes: []graphs.Node{{ID: "start", Type: graphs.NodeTrigger, Config: json.RawMessage(test.config)}, {ID: "tail", Type: graphs.NodeTransform, Config: json.RawMessage(`{"template":"ok"}`)}}, Edges: []graphs.Edge{{From: "start", To: "tail"}}}
		row := Project(graph)
		if row.NodeCount != 2 || row.EdgeCount != 1 || row.TriggerKind != test.kind || row.TriggerDetail != test.detail {
			t.Fatal(test, row)
		}
		light := object(t, row)
		if _, ok := light["nodes"]; ok {
			t.Fatal(light)
		}
		if _, ok := light["edges"]; ok {
			t.Fatal(light)
		}
		for _, key := range []string{"id", "name", "enabled", "created_ms", "updated_ms", "node_count", "edge_count", "trigger_kind"} {
			if _, ok := light[key]; !ok {
				t.Fatal(key, light)
			}
		}
		if test.detail == "" {
			if _, ok := light["trigger_detail"]; ok {
				t.Fatal(light)
			}
		}
		for _, key := range []string{"last_run", "description"} {
			if _, ok := light[key]; ok {
				t.Fatal(key, light)
			}
		}
		full := object(t, ProjectFull(graph))
		if len(full["nodes"].([]any)) != 2 || len(full["edges"].([]any)) != 1 {
			t.Fatal(full)
		}
		if !reflect.DeepEqual(full["nodes"].([]any)[0].(map[string]any)["config"], object(t, json.RawMessage(test.config))) {
			t.Fatal(full)
		}
	}
	zero := object(t, ProjectFull(graphs.Workflow{}))
	if zero["nodes"] != nil {
		t.Fatal(zero)
	}
	if _, ok := zero["edges"]; ok {
		t.Fatal(zero)
	}
}
func TestWorkflowListPreparationOptInSingleScanAndPartialHistory(t *testing.T) {
	graph := &graphPort{items: []graphs.Workflow{{Name: "owned", Enabled: true}, {Name: "other", Enabled: false}}}
	history := &historyPort{cause: errors.New("partial history"), events: []*event.Event{started("owned", "old", 100), started("owned", "new", 200), finished("owned", "old", 300, false), finished("owned", "new", 500, true), started("foreign", "alien", 900)}}
	s := NewReads(graph, history, nil)
	listing := s.PrepareList()
	if graph.lists != 1 || history.reads != 0 {
		t.Fatal(graph.lists, history.reads)
	}
	out, err := listing.List(context.Background(), ListInput{})
	if err != nil || out.Count != 2 || out.EnabledCount != 1 || out.Workflows[0].Name != "owned" || out.Workflows[1].Name != "other" || out.Workflows[0].LastRun != nil || history.reads != 0 {
		t.Fatal(out, err, history.reads)
	}
	out, err = listing.List(context.Background(), ListInput{WithRuns: true})
	if err != nil || history.reads != 1 || graph.lists != 1 || out.Workflows[0].LastRun == nil || *out.Workflows[0].LastRun != (LastRun{Status: "failed", AtMS: 500, DurationMS: 300}) || out.Workflows[1].LastRun != nil {
		t.Fatal(out, err, history.reads, graph.lists)
	}
	row := object(t, LastRun{Status: "running", AtMS: 0})
	if _, ok := row["duration_ms"]; ok {
		t.Fatal(row)
	}
	if row["at_ms"] != float64(0) {
		t.Fatal(row)
	}
	empty := NewReads(&graphPort{}, history, nil).PrepareList()
	out, err = empty.List(context.Background(), ListInput{WithRuns: true})
	wire := object(t, out)
	if err != nil || out.Count != 0 || out.EnabledCount != 0 || wire["workflows"] == nil || len(wire["workflows"].([]any)) != 0 || history.reads != 2 {
		t.Fatal(out, err, wire, history.reads)
	}
}
func TestWorkflowShowAndTemplateGalleryRetainReferencesAndRequiredFields(t *testing.T) {
	graph := &graphPort{items: []graphs.Workflow{{ID: "owned-id", Name: "owned", Description: "present", Nodes: []graphs.Node{{ID: "start", Type: graphs.NodeTrigger}}}}}
	calls := 0
	s := NewReads(graph, &historyPort{}, func() []graphs.Template {
		calls++
		return []graphs.Template{{Name: "template", Workflow: graph.items[0]}}
	})
	out, err := s.Show(context.Background(), ShowInput{Ref: " owned "})
	if err != nil || out.Workflow.Name != "owned" || out.Workflow.Description != "present" || !reflect.DeepEqual(graph.refs, []string{"owned"}) {
		t.Fatal(out, err, graph.refs)
	}
	missing, err := s.Show(context.Background(), ShowInput{Ref: " missing "})
	if err == nil || err.Error() != "unknown workflow:  missing " || !reflect.DeepEqual(missing, ShowOutput{}) {
		t.Fatal(missing, err)
	}
	gallery, err := s.Templates(context.Background(), TemplatesInput{})
	if err != nil || calls != 1 || gallery.Count != 1 || gallery.Templates[0].NodeCount != 1 || gallery.Templates[0].Workflow.Name != "owned" {
		t.Fatal(gallery, err, calls)
	}
	wire := object(t, gallery)
	row := wire["templates"].([]any)[0].(map[string]any)
	for _, key := range []string{"name", "title", "description", "category", "node_count", "workflow"} {
		if _, ok := row[key]; !ok {
			t.Fatal(key, row)
		}
	}
	empty, err := NewReads(graph, &historyPort{}, func() []graphs.Template { return nil }).Templates(context.Background(), TemplatesInput{})
	if err != nil || empty.Templates == nil || empty.Count != 0 {
		t.Fatal(empty, err)
	}
	builtin, err := NewReads(graph, &historyPort{}, nil).Templates(context.Background(), TemplatesInput{})
	if err != nil || builtin.Count != len(graphs.Templates()) {
		t.Fatal(builtin, err)
	}
}
func TestWorkflowReadsUseOwnedActualGraphStoreAndJournal(t *testing.T) {
	store, err := graphs.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	graph, _, err := store.Save(graphs.Workflow{Name: "owned", Nodes: []graphs.Node{{ID: "start", Type: graphs.NodeTrigger}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetEnabled(graph.ID, false); err != nil {
		t.Fatal(err)
	}
	now := int64(10)
	j, err := journal.Open(t.TempDir(), journal.Options{Now: func() time.Time { return time.UnixMilli(now) }})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if _, err := j.Append(event.Spec{Actor: "fixture", Kind: event.KindWorkflowStarted, Subject: "workflow.owned", CorrelationID: "owned"}); err != nil {
		t.Fatal(err)
	}
	now = 25
	if _, err := j.Append(event.Spec{Actor: "fixture", Kind: event.KindWorkflowCompleted, Subject: "workflow.owned", CorrelationID: "owned"}); err != nil {
		t.Fatal(err)
	}
	out, err := NewReads(store, j, nil).PrepareList().List(context.Background(), ListInput{WithRuns: true})
	if err != nil || out.Count != 1 || out.EnabledCount != 0 || out.Workflows[0].LastRun == nil || *out.Workflows[0].LastRun != (LastRun{Status: "completed", AtMS: 25, DurationMS: 15}) {
		t.Fatal(out, err)
	}
}
