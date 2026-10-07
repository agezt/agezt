// SPDX-License-Identifier: MIT
package workflow

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	graphs "github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"testing"
)

type writeCall struct {
	Method, Corr, Ref, Reason string
	Workflow                  graphs.Workflow
	Enabled                   bool
}
type lifecyclePort struct {
	Calls   []writeCall
	Stored  graphs.Workflow
	Changed bool
	Cause   error
}

func (p *lifecyclePort) SaveWorkflow(corr string, w graphs.Workflow) (graphs.Workflow, bool, error) {
	p.Calls = append(p.Calls, writeCall{Method: "save", Corr: corr, Workflow: w})
	return p.Stored, p.Changed, p.Cause
}
func (p *lifecyclePort) RestoreWorkflow(corr string, w graphs.Workflow, reason string) (graphs.Workflow, bool, error) {
	p.Calls = append(p.Calls, writeCall{Method: "restore", Corr: corr, Workflow: w, Reason: reason})
	return p.Stored, p.Changed, p.Cause
}
func (p *lifecyclePort) SetWorkflowEnabled(corr, ref string, enabled bool) (graphs.Workflow, error) {
	p.Calls = append(p.Calls, writeCall{Method: "enable", Corr: corr, Ref: ref, Enabled: enabled})
	return p.Stored, p.Cause
}
func (p *lifecyclePort) RemoveWorkflow(corr, ref string) (bool, error) {
	p.Calls = append(p.Calls, writeCall{Method: "remove", Corr: corr, Ref: ref})
	return p.Changed, p.Cause
}
func TestWorkflowLifecycleRetainsFacadeInputsResultsAndProjections(t *testing.T) {
	input := graphs.Workflow{ID: "posted", Name: " raw name ", Nodes: []graphs.Node{{ID: "start", Type: graphs.NodeTrigger}}}
	stored := graphs.Workflow{ID: "stored", Name: "stored", CreatedMS: 11, UpdatedMS: 22, Enabled: false, Nodes: input.Nodes}
	p := &lifecyclePort{Stored: stored}
	s := NewLifecycle(p)
	ctx := context.Background()
	saved, err := s.Save(ctx, SaveInput{CorrelationID: "save-corr", Workflow: input})
	if err != nil || saved.Created || !reflect.DeepEqual(saved.Workflow, ProjectFull(stored)) || !reflect.DeepEqual(p.Calls[0], writeCall{Method: "save", Corr: "save-corr", Workflow: input}) {
		t.Fatal(saved, err, p.Calls)
	}
	wire := object(t, saved)
	if wire["created"] != false || wire["workflow"].(map[string]any)["enabled"] != false {
		t.Fatal(wire)
	}
	p.Changed = true
	restored, err := s.Restore(ctx, RestoreInput{CorrelationID: "restore-corr", Workflow: input, Reason: " exact reason "})
	if err != nil || !restored.Created || !reflect.DeepEqual(restored.Workflow, ProjectFull(stored)) || !reflect.DeepEqual(p.Calls[1], writeCall{Method: "restore", Corr: "restore-corr", Workflow: input, Reason: " exact reason "}) {
		t.Fatal(restored, err, p.Calls)
	}
	enabled, err := s.SetEnabled(ctx, EnableInput{CorrelationID: "enable-corr", Ref: " raw ref ", Enabled: true})
	if err != nil || !reflect.DeepEqual(enabled.Workflow, Project(stored)) || !reflect.DeepEqual(p.Calls[2], writeCall{Method: "enable", Corr: "enable-corr", Ref: " raw ref ", Enabled: true}) {
		t.Fatal(enabled, err, p.Calls)
	}
	light := object(t, enabled.Workflow)
	if _, ok := light["nodes"]; ok {
		t.Fatal(light)
	}
	removed, err := s.Remove(ctx, RemoveInput{CorrelationID: "remove-corr", Ref: " raw ref "})
	if err != nil || !removed.Removed || !reflect.DeepEqual(p.Calls[3], writeCall{Method: "remove", Corr: "remove-corr", Ref: " raw ref "}) {
		t.Fatal(removed, err, p.Calls)
	}
	p.Changed = false
	removed, err = s.Remove(ctx, RemoveInput{Ref: "missing"})
	if err != nil || object(t, removed)["removed"] != false {
		t.Fatal(removed, err)
	}
}
func TestWorkflowLifecyclePreservesCausesAndZeroFailureOutputs(t *testing.T) {
	cause := errors.New("owned writer cause")
	p := &lifecyclePort{Stored: graphs.Workflow{Name: "must not escape"}, Changed: true, Cause: cause}
	s := NewLifecycle(p)
	ctx := context.Background()
	saved, err := s.Save(ctx, SaveInput{})
	if err != cause || !reflect.DeepEqual(saved, SaveOutput{}) {
		t.Fatal(saved, err)
	}
	saved, err = s.Restore(ctx, RestoreInput{})
	if err != cause || !reflect.DeepEqual(saved, SaveOutput{}) {
		t.Fatal(saved, err)
	}
	enabled, err := s.SetEnabled(ctx, EnableInput{})
	if err != cause || !reflect.DeepEqual(enabled, EnableOutput{}) {
		t.Fatal(enabled, err)
	}
	removed, err := s.Remove(ctx, RemoveInput{})
	if err != cause || !reflect.DeepEqual(removed, RemoveOutput{}) {
		t.Fatal(removed, err)
	}
	p.Cause = errors.Join(graphs.ErrNotFound, errors.New("extra"))
	enabled, err = s.SetEnabled(ctx, EnableInput{Ref: " missing "})
	var unknown UnknownWorkflowError
	if !errors.Is(err, graphs.ErrNotFound) || !errors.As(err, &unknown) || unknown.Ref != " missing " || err.Error() != "unknown workflow:  missing " || !reflect.DeepEqual(enabled, EnableOutput{}) {
		t.Fatal(enabled, err, unknown)
	}
}
func TestWorkflowLifecycleUsesActualRuntimeIdentityAndEvents(t *testing.T) {
	provider := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewLifecycle(k)
	ctx := context.Background()
	input := graphs.Workflow{Name: "owned", Nodes: []graphs.Node{{ID: "start", Type: graphs.NodeTrigger}}}
	saved, err := s.Save(ctx, SaveInput{CorrelationID: "save1", Workflow: input})
	if err != nil || !saved.Created || saved.Workflow.ID == "" || !saved.Workflow.Enabled {
		t.Fatal(saved, err)
	}
	original := saved.Workflow
	paused, err := s.SetEnabled(ctx, EnableInput{CorrelationID: "pause", Ref: "owned", Enabled: false})
	if err != nil || paused.Workflow.Enabled {
		t.Fatal(paused, err)
	}
	input.Description = "updated"
	input.ID = "posted-ignored"
	input.CreatedMS = 1
	input.Enabled = true
	saved, err = s.Save(ctx, SaveInput{CorrelationID: "save2", Workflow: input})
	if err != nil || saved.Created || saved.Workflow.ID != original.ID || saved.Workflow.CreatedMS != original.CreatedMS || saved.Workflow.Enabled || saved.Workflow.Description != "updated" {
		t.Fatal(saved, err)
	}
	input.ID = "checkpoint-owned"
	input.Enabled = true
	input.CreatedMS = 7
	restored, err := s.Restore(ctx, RestoreInput{CorrelationID: "restore", Workflow: input, Reason: "rollback"})
	if err != nil || restored.Created || restored.Workflow.ID != "checkpoint-owned" || restored.Workflow.CreatedMS != 7 || !restored.Workflow.Enabled {
		t.Fatal(restored, err)
	}
	removed, err := s.Remove(ctx, RemoveInput{CorrelationID: "remove", Ref: "owned"})
	if err != nil || !removed.Removed {
		t.Fatal(removed, err)
	}
	if _, found := k.Workflows().Get("owned"); found {
		t.Fatal("removed workflow retained")
	}
	events := []*event.Event{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Subject == "workflow.owned" {
			events = append(events, e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	kinds := []event.Kind{event.KindWorkflowSaved, event.KindWorkflowUpdated, event.KindWorkflowSaved, event.KindWorkflowRestored, event.KindWorkflowRemoved}
	corrs := []string{"save1", "pause", "save2", "restore", "remove"}
	if len(events) != 5 {
		t.Fatal(events)
	}
	for i, e := range events {
		if e.Kind != kinds[i] || e.CorrelationID != corrs[i] || e.Actor != "workflow" {
			t.Fatal(i, e)
		}
	}
	if provider.CallCount() != 0 {
		t.Fatal(provider.CallCount())
	}
	invalid, err := s.Save(ctx, SaveInput{Workflow: graphs.Workflow{Name: ""}})
	if err == nil || !reflect.DeepEqual(invalid, SaveOutput{}) {
		t.Fatal(invalid, err)
	}
	// Runtime publication remains best effort until mandatory app binding.
	if err := k.Journal().Close(); err != nil {
		t.Fatal(err)
	}
	input.Name = "after-journal-close"
	saved, err = s.Save(ctx, SaveInput{Workflow: input})
	if err != nil || !saved.Created {
		t.Fatal(saved, err)
	}
	if _, found := k.Workflows().Get(input.Name); !found {
		t.Fatal("facade write bypassed unexpectedly")
	}
}
