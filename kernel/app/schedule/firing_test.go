// SPDX-License-Identifier: MIT

package schedule_test

import (
	"context"
	"encoding/json"
	"errors"
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"reflect"
	"testing"
	"time"
)

type firingReader struct {
	events []*event.Event
	cause  error
	reads  int
}

func (r *firingReader) Range(fn func(*event.Event) error) error {
	r.reads++
	for _, e := range r.events {
		if err := fn(e); err != nil {
			return err
		}
	}
	return r.cause
}
func firingEvent(t *testing.T, corr string, ms, seq int64, payload any) *event.Event {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return &event.Event{Kind: event.KindScheduleFired, CorrelationID: corr, TSUnixMS: ms, Seq: seq, Payload: raw}
}
func firingFixture(t *testing.T) (*firingReader, map[string]appschedule.FiringRun) {
	t.Helper()
	reader := &firingReader{events: []*event.Event{
		{Kind: event.KindTaskReceived, CorrelationID: "manual", TSUnixMS: 500, Seq: 7},
		firingEvent(t, "complete", 100, 1, map[string]any{"schedule_id": "A", "intent": "Alpha", "model": "m1", "target": cadence.TargetWorkflow, "workflow": "owned-flow", "agent": "writer", "executor": " override ", "category": " custom ", "effect_class": " effect ", "uses_llm": false, "autonomy_runbook": map[string]any{"retry": 3}}),
		firingEvent(t, "failed", 200, 2, map[string]any{"schedule_id": "B", "intent": "ALPHA fail", "target": cadence.TargetTool, "tool": "shell"}),
		firingEvent(t, "abandoned", 250, 3, map[string]any{"schedule_id": "C", "intent": "Gamma", "target": cadence.TargetSystemTask, "system_task": cadence.SystemTaskCatalogSync}),
		firingEvent(t, "running", 300, 4, map[string]any{"schedule_id": "A", "intent": "alpha", "agent": "runner"}),
		firingEvent(t, "tie-failed", 300, 5, map[string]any{"schedule_id": "A", "intent": "alpha tie", "target": "unknown"}),
		{Kind: event.KindScheduleFired, CorrelationID: "legacy", TSUnixMS: 400, Seq: 6, Payload: json.RawMessage(`{`)},
	}}
	runs := map[string]appschedule.FiringRun{"complete": {Completed: true, Failed: true, Abandoned: true, StartedUnixMS: 100, CompletedUnixMS: 80, SpentMicrocents: 10, AnswerPreview: "done"}, "failed": {Failed: true, Abandoned: true, StartedUnixMS: 150, FailedUnixMS: 140, SpentMicrocents: 20}, "abandoned": {Abandoned: true, SpentMicrocents: 30}, "running": {}, "tie-failed": {Failed: true, StartedUnixMS: 295, FailedUnixMS: 303, FailReason: "denied", SpentMicrocents: 40, AnswerPreview: "retry"}}
	return reader, runs
}
func TestScheduleFiringViewsRetainSharedOutcomePrecedenceDurationMetadataAndLatestTie(t *testing.T) {
	reader, runs := firingFixture(t)
	service := appschedule.NewFiringService(reader, func() (map[string]appschedule.FiringRun, error) { return runs, nil })
	out, err := service.Fires(context.Background(), appschedule.FiresInput{Limit: 20})
	if err != nil || out.Count != 6 || out.NextCursor != "" {
		t.Fatal(out, err)
	}
	ids := []string{}
	byCorr := map[string]appschedule.FireRecord{}
	for _, row := range out.Fires {
		ids = append(ids, row.CorrelationID)
		byCorr[row.CorrelationID] = row
	}
	if !reflect.DeepEqual(ids, []string{"legacy", "tie-failed", "running", "abandoned", "failed", "complete"}) {
		t.Fatal(ids)
	}
	c, f, a, r, l := byCorr["complete"], byCorr["failed"], byCorr["abandoned"], byCorr["running"], byCorr["legacy"]
	if c.Status != "completed" || c.DurationMS != -20 || c.SpentMC != 10 || c.AnswerPreview != "done" || c.Model != "m1" || c.Executor != "override" || c.Category != "custom" || c.EffectClass != "effect" || c.UsesLLM || c.Action != "run workflow owned-flow" || c.AutonomyRunbook["retry"] != float64(3) {
		t.Fatal(c)
	}
	if f.Status != "failed" || f.Reason != "" || f.DurationMS != 0 || f.Executor != "tool" || f.UsesLLM || f.Action != "run tool shell" {
		t.Fatal(f)
	}
	if a.Status != "abandoned" || a.Executor != "daemon" || a.Category != "catalog" || a.EffectClass != "config_update" || a.UsesLLM || a.Action != "run system task "+cadence.SystemTaskCatalogSync {
		t.Fatal(a)
	}
	if r.Status != "running" || !r.UsesLLM || r.Action != "wake runner: alpha" || l.Status != "running" || l.ScheduleID != "" || l.Executor != "agent" || !l.UsesLLM || l.Intent != "" || l.Action != "" {
		t.Fatal(r, l)
	}
	latest, err := service.Latest(context.Background())
	if err != nil || len(latest) != 3 || latest["A"] != (appschedule.LastFiring{FiredMS: 300, Status: "running"}) || latest["B"] != (appschedule.LastFiring{FiredMS: 200, Status: "failed"}) || latest["C"] != (appschedule.LastFiring{FiredMS: 250, Status: "abandoned"}) {
		t.Fatal(latest, err)
	}
	stats, err := service.Stats(context.Background(), appschedule.StatsInput{SinceMS: -3})
	want := appschedule.StatsOutput{Total: 6, Completed: 1, Failed: 2, Running: 2, Abandoned: 1, Terminal: 4, SuccessRate: 0.25, SpentMicrocents: 100, Schedules: 3, FailedByReason: map[string]int{"unknown": 1, "denied": 1}, WindowMS: -3}
	if err != nil || !reflect.DeepEqual(stats, want) {
		t.Fatal(stats, want, err)
	}
}
func TestScheduleFiringFiltersRunBeforeLimitAndCursorKeepsStrictlyOlderTie(t *testing.T) {
	reader, runs := firingFixture(t)
	service := appschedule.NewFiringService(reader, func() (map[string]appschedule.FiringRun, error) { return runs, nil })
	for _, tc := range []struct {
		in     appschedule.FiresInput
		ids    []string
		cursor string
	}{
		{appschedule.FiresInput{Limit: 2}, []string{"legacy", "tie-failed"}, "300:5"},
		{appschedule.FiresInput{Limit: 2, CursorOK: true, CursorMS: 300, CursorSeq: 5}, []string{"running", "abandoned"}, "250:3"},
		{appschedule.FiresInput{Limit: 1, ID: "A", Status: "completed"}, []string{"complete"}, "100:1"},
		{appschedule.FiresInput{Limit: 20, Intent: "aLpHa"}, []string{"tie-failed", "running", "failed", "complete"}, ""},
		{appschedule.FiresInput{Limit: 20, ID: " A "}, []string{}, ""},
		{appschedule.FiresInput{Limit: 20, Status: "FAILED"}, []string{}, ""},
		{appschedule.FiresInput{Limit: 20, CutoffMS: 200}, []string{"legacy", "tie-failed", "running", "abandoned", "failed"}, ""},
		{appschedule.FiresInput{Limit: 0}, []string{"legacy"}, "400:6"},
		{appschedule.FiresInput{Limit: 2000}, []string{"legacy", "tie-failed", "running", "abandoned", "failed", "complete"}, ""},
	} {
		out, err := service.Fires(context.Background(), tc.in)
		ids := []string{}
		for _, row := range out.Fires {
			ids = append(ids, row.CorrelationID)
		}
		if err != nil || out.Count != len(tc.ids) || !reflect.DeepEqual(ids, tc.ids) || out.NextCursor != tc.cursor {
			t.Fatal(tc.in, out, ids, tc.ids, err)
		}
	}
	stats, err := service.Stats(context.Background(), appschedule.StatsInput{ID: "A", CutoffMS: 250, SinceMS: 99})
	want := appschedule.StatsOutput{Total: 2, Failed: 1, Running: 1, Terminal: 1, SpentMicrocents: 40, Schedules: 1, FailedByReason: map[string]int{"denied": 1}, WindowMS: 99}
	if err != nil || !reflect.DeepEqual(stats, want) {
		t.Fatal(stats, want, err)
	}
}
func TestScheduleFiringOutputsRetainRequiredZeroFieldsOptionalRunbookAndEmptyCollections(t *testing.T) {
	reader, runs := firingFixture(t)
	service := appschedule.NewFiringService(reader, func() (map[string]appschedule.FiringRun, error) { return runs, nil })
	out, err := service.Fires(context.Background(), appschedule.FiresInput{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var view struct {
		Fires []map[string]json.RawMessage `json:"fires"`
	}
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatal(err)
	}
	required := []string{"correlation_id", "schedule_id", "fired_unix_ms", "intent", "model", "target", "agent", "workflow", "system_task", "tool", "executor", "category", "effect_class", "uses_llm", "action", "status", "reason", "duration_ms", "spent_mc", "answer_preview"}
	for i, row := range view.Fires {
		for _, key := range required {
			if _, ok := row[key]; !ok {
				t.Fatal(i, key, string(raw))
			}
		}
		_, present := row["autonomy_runbook"]
		if present != (out.Fires[i].CorrelationID == "complete") {
			t.Fatal(row)
		}
	}
	empty := appschedule.NewFiringService(&firingReader{}, func() (map[string]appschedule.FiringRun, error) { return map[string]appschedule.FiringRun{}, nil })
	fires, err := empty.Fires(context.Background(), appschedule.FiresInput{Limit: 20})
	raw, _ = json.Marshal(fires)
	if err != nil || string(raw) != `{"fires":[],"count":0,"next_cursor":""}` {
		t.Fatal(fires, err, string(raw))
	}
	stats, err := empty.Stats(context.Background(), appschedule.StatsInput{})
	if err != nil || stats.FailedByReason == nil || len(stats.FailedByReason) != 0 || stats.Total != 0 || stats.SuccessRate != 0 {
		t.Fatal(stats, err)
	}
	latest, err := empty.Latest(context.Background())
	if err != nil || latest == nil || len(latest) != 0 {
		t.Fatal(latest, err)
	}
}
func TestScheduleFiringViewsRetainOriginalRunAndJournalCausesWithZeroOutputs(t *testing.T) {
	cause := errors.New("owned firing read cause")
	for _, phase := range []string{"runs", "journal-empty", "journal-partial"} {
		t.Run(phase, func(t *testing.T) {
			reader, runs := firingFixture(t)
			if phase == "journal-empty" {
				reader.events = nil
			}
			if phase != "runs" {
				reader.cause = cause
			}
			calls := 0
			service := appschedule.NewFiringService(reader, func() (map[string]appschedule.FiringRun, error) {
				calls++
				if phase == "runs" {
					return nil, cause
				}
				return runs, nil
			})
			out, err := service.Fires(context.Background(), appschedule.FiresInput{Limit: 20})
			if err != cause || !reflect.DeepEqual(out, appschedule.FiresOutput{}) {
				t.Fatal(out, err)
			}
			stats, err := service.Stats(context.Background(), appschedule.StatsInput{})
			if err != cause || !reflect.DeepEqual(stats, appschedule.StatsOutput{}) {
				t.Fatal(stats, err)
			}
			latest, err := service.Latest(context.Background())
			if err != cause || latest != nil || calls != 3 || phase == "runs" && reader.reads != 0 || phase != "runs" && reader.reads != 3 {
				t.Fatal(latest, err, calls, reader.reads)
			}
		})
	}
}
func TestScheduleFiringServiceUsesActualOwnedJournalSequenceAndLatestSameMSTie(t *testing.T) {
	j, err := journal.Open(t.TempDir(), journal.Options{Now: func() time.Time { return time.UnixMilli(100) }})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	var lastSeq int64
	for _, corr := range []string{"first", "second"} {
		e, err := j.Append(event.Spec{Kind: event.KindScheduleFired, Subject: "schedule", Actor: "fixture", CorrelationID: corr, Payload: map[string]any{"schedule_id": "owned"}})
		if err != nil {
			t.Fatal(err)
		}
		lastSeq = e.Seq
	}
	service := appschedule.NewFiringService(j, func() (map[string]appschedule.FiringRun, error) {
		return map[string]appschedule.FiringRun{"first": {Completed: true}, "second": {Failed: true, FailReason: "late"}}, nil
	})
	out, err := service.Fires(context.Background(), appschedule.FiresInput{Limit: 1})
	if err != nil || out.Count != 1 || out.Fires[0].CorrelationID != "second" || out.NextCursor != journal.EncodeCursor(100, lastSeq) {
		t.Fatal(out, err)
	}
	latest, err := service.Latest(context.Background())
	if err != nil || latest["owned"] != (appschedule.LastFiring{Status: "completed", FiredMS: 100}) {
		t.Fatal(latest, err)
	}
}

func TestScheduleFiringLimitRetainsThousandRowCapBeforeCursorBoundary(t *testing.T) {
	reader := &firingReader{}
	for i := 1; i <= 1005; i++ {
		reader.events = append(reader.events, &event.Event{Kind: event.KindScheduleFired, CorrelationID: "fixture", TSUnixMS: int64(i), Seq: int64(i)})
	}
	service := appschedule.NewFiringService(reader, func() (map[string]appschedule.FiringRun, error) { return map[string]appschedule.FiringRun{}, nil })
	out, err := service.Fires(context.Background(), appschedule.FiresInput{Limit: 2000})
	if err != nil || out.Count != 1000 || len(out.Fires) != 1000 || out.Fires[999].FiredUnixMS != 6 || out.NextCursor != "6:6" {
		t.Fatal(out.Count, len(out.Fires), out.NextCursor, err)
	}
}
