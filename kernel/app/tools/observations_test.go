// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/platform/journalview"
	"reflect"
	"strings"
	"testing"
)

type observationJournal struct {
	events []*event.Event
	cause  error
	reads  int
}

func (p *observationJournal) Range(fn func(*event.Event) error) error {
	p.reads++
	for _, ev := range p.events {
		if err := fn(ev); err != nil {
			return err
		}
	}
	return p.cause
}
func observationEvent(seq, ts int64, kind event.Kind, corr string, payload any) *event.Event {
	raw, _ := json.Marshal(payload)
	return &event.Event{Seq: seq, TSUnixMS: ts, Kind: kind, Subject: "tool", Actor: "fixture", CorrelationID: corr, Payload: raw}
}
func observationFixture() []*event.Event {
	events := []*event.Event{
		observationEvent(0, 10, event.KindToolInvoked, "a", map[string]any{"call_id": "reused", "input": map[string]any{"marker": "A"}}),
		observationEvent(1, 12, event.KindToolInvoked, "b", map[string]any{"call_id": "reused", "input": map[string]any{"marker": "B"}}),
		observationEvent(2, 15, event.KindToolInvoked, "skip", map[string]any{"call_id": "s", "input": map[string]any{"marker": "skipped"}}),
		observationEvent(3, 50, event.KindToolInvoked, "back", map[string]any{"call_id": "back", "input": nil}),
		observationEvent(4, 32, event.KindToolResult, "b", map[string]any{"tool": "foo", "call_id": "reused", "output": " \n" + strings.Repeat("🙂", 120) + "\t ", "error": false, "observation_trust": "untrusted", "observation_source": "fixture", "directive_like": true, "directive_matches": []any{"role"}}),
		observationEvent(5, 40, event.KindToolResult, "a", map[string]any{"tool": "foo", "call_id": "reused", "output": "A", "error": false}),
		observationEvent(6, 45, event.KindToolResult, "denied", map[string]any{"tool": "denied-tool", "call_id": "reused", "output": " denied \n policy ", "error": true}),
		observationEvent(7, 50, event.KindToolResult, "skip", map[string]any{"tool": "foo", "call_id": "s", "output": "", "error": true, "not_executed": true}),
		observationEvent(8, 40, event.KindToolResult, "back", map[string]any{"tool": "foo", "call_id": "back", "output": "clock back", "error": false}),
		observationEvent(9, 55, event.KindToolResult, "malformed", nil),
		observationEvent(10, 60, event.KindInfo, "noise", nil),
	}
	events[9].Payload = json.RawMessage(`{bad`)
	return events
}
func TestToolObservationsLogScopedJoinCutoffSkippedProvenanceAndCursor(t *testing.T) {
	port := &observationJournal{events: observationFixture()}
	service := NewObservations(port)
	out, err := service.Log(context.Background(), LogInput{Page: journalview.Input{Limit: 20}})
	if err != nil || out.Count != 6 || out.NextCursor != "" || port.reads != 1 {
		t.Fatal(out, err, port.reads)
	}
	rows := map[string]map[string]any{}
	for _, item := range out.Invocations {
		row := observationWireRow(t, item)
		rows[row["correlation_id"].(string)] = row
		for _, key := range []string{"actor", "correlation_id", "tool", "call_id", "input", "output", "error", "duration_ms", "observation_trust", "observation_source", "directive_like", "directive_matches", "seq", "ts_unix_ms"} {
			if _, ok := row[key]; !ok {
				t.Fatal("missing required row field", key, row)
			}
		}
	}
	if rows["a"]["input"] != `{"marker":"A"}` || rows["b"]["input"] != `{"marker":"B"}` || rows["a"]["duration_ms"] != float64(30) || rows["b"]["duration_ms"] != float64(20) || rows["denied"]["input"] != "" || rows["denied"]["duration_ms"] != float64(0) || rows["skip"]["duration_ms"] != float64(0) || rows["skip"]["not_executed"] != true || rows["back"]["duration_ms"] != float64(0) {
		t.Fatal(rows)
	}
	if rows["b"]["output"] != strings.Repeat("🙂", 100)+"…" || rows["b"]["observation_trust"] != "untrusted" || rows["b"]["observation_source"] != "fixture" || rows["b"]["directive_like"] != true || !reflect.DeepEqual(rows["b"]["directive_matches"], []any{"role"}) {
		t.Fatal(rows["b"])
	}
	if _, ok := rows["a"]["not_executed"]; ok {
		t.Fatal(rows["a"])
	}
	if matches, ok := rows["a"]["directive_matches"]; !ok || matches != nil {
		t.Fatal(rows["a"])
	}
	cutoff, err := service.Log(context.Background(), LogInput{Page: journalview.Input{Limit: 20, CutoffMS: 40}})
	if err != nil || cutoff.Count != 5 {
		t.Fatal(cutoff, err)
	}
	for _, item := range cutoff.Invocations {
		row := observationWireRow(t, item)
		if row["correlation_id"] == "a" && (row["input"] != `{"marker":"A"}` || row["duration_ms"] != float64(30)) {
			t.Fatal(row)
		}
	}
	page1, _ := service.Log(context.Background(), LogInput{Page: journalview.Input{Limit: 2}})
	page2, _ := service.Log(context.Background(), LogInput{Page: journalview.Input{Limit: 2, Cursor: page1.NextCursor}})
	if page1.Count != 2 || page2.Count != 2 || page1.NextCursor == "" || page2.NextCursor == "" || page1.Invocations[0].Seq != int64(9) || page1.Invocations[1].Seq != int64(7) || page2.Invocations[0].Seq != int64(6) || page2.Invocations[1].Seq != int64(8) {
		t.Fatal(page1, page2)
	}
	fallback, _ := service.Log(context.Background(), LogInput{Page: journalview.Input{Limit: 2, Cursor: "bad-cursor"}})
	if !reflect.DeepEqual(fallback, page1) {
		t.Fatal(fallback, page1)
	}
	for _, tc := range []struct {
		in    LogInput
		count int
	}{{LogInput{ErrorsOnly: true, Page: journalview.Input{Limit: 20}}, 2}, {LogInput{Tool: "foo", Page: journalview.Input{Limit: 20}}, 4}, {LogInput{SlowMS: 20, Page: journalview.Input{Limit: 20}}, 2}, {LogInput{ErrorsOnly: true, SlowMS: 1, Page: journalview.Input{Limit: 20}}, 0}} {
		got, err := service.Log(context.Background(), tc.in)
		if err != nil || got.Count != tc.count {
			t.Fatal(tc, got, err)
		}
	}
}
func TestToolObservationsStatsScopedLatencyRatesBucketsCutoffAndEmpty(t *testing.T) {
	port := &observationJournal{events: observationFixture()}
	out, err := NewObservations(port).Stats(context.Background(), StatsInput{WindowMS: 123})
	wantDuration := map[string]any{"count": float64(2), "avg": float64(25), "min": float64(20), "max": float64(30), "p50": float64(20), "p95": float64(30)}
	if err != nil || out.Total != 6 || out.Errored != 2 || out.ErrorRate != float64(2)/6 || out.Tools != 3 || out.WindowMS != 123 || !reflect.DeepEqual(observationWire(t, out)["duration_ms"], wantDuration) || !reflect.DeepEqual(out.ErrorsByMessage, map[string]int{"denied policy": 1, "(no message)": 1}) {
		t.Fatal(out, err)
	}
	if !reflect.DeepEqual(observationWire(t, out)["by_tool"].(map[string]any)["foo"], map[string]any{"calls": float64(4), "errors": float64(1), "avg_ms": float64(25)}) {
		t.Fatal(out)
	}
	if out.ByTool["denied-tool"].AvgMS != nil {
		t.Fatal(out)
	}
	if _, ok := out.ByTool["unknown"]; !ok {
		t.Fatal(out)
	}
	cut, err := NewObservations(port).Stats(context.Background(), StatsInput{CutoffMS: 40})
	if err != nil || cut.Total != 5 || cut.DurationMS.Count != 1 || cut.DurationMS.Avg != int64(30) {
		t.Fatal(cut, err)
	}
	filtered, err := NewObservations(port).Stats(context.Background(), StatsInput{Tool: "foo"})
	if err != nil || filtered.Total != 4 || filtered.Errored != 1 || filtered.Tools != 1 || filtered.ErrorRate != .25 {
		t.Fatal(filtered, err)
	}
	empty, err := NewObservations(&observationJournal{}).Stats(context.Background(), StatsInput{})
	if err != nil || empty.Total != 0 || empty.ErrorRate != 0 || empty.Tools != 0 || empty.ByTool == nil || empty.ErrorsByMessage == nil || empty.DurationMS.Count != 0 || empty.DurationMS.Avg != int64(0) {
		t.Fatal(empty, err)
	}
}
func TestToolObservationsRepeatedInvocationKeepsLastObservedAndErrorsKeepCause(t *testing.T) {
	events := []*event.Event{observationEvent(0, 10, event.KindToolInvoked, "a", map[string]any{"call_id": "id", "input": "first"}), observationEvent(1, 20, event.KindToolInvoked, "a", map[string]any{"call_id": "id", "input": "last"}), observationEvent(2, 30, event.KindToolResult, "a", map[string]any{"call_id": "id", "tool": "foo"})}
	service := NewObservations(&observationJournal{events: events})
	log, err := service.Log(context.Background(), LogInput{Page: journalview.Input{Limit: 20}})
	stats, statsErr := service.Stats(context.Background(), StatsInput{})
	if err != nil || statsErr != nil || log.Count != 1 || log.Invocations[0].Input != `"last"` || log.Invocations[0].DurationMS != int64(10) || stats.DurationMS.Avg != int64(10) {
		t.Fatal(log, stats, err, statsErr)
	}
	cause := errors.New("owned range error")
	service = NewObservations(&observationJournal{events: events, cause: cause})
	log, err = service.Log(context.Background(), LogInput{Page: journalview.Input{Limit: 20}})
	stats, statsErr = service.Stats(context.Background(), StatsInput{})
	if err != cause || statsErr != cause || !reflect.DeepEqual(log, LogOutput{}) || !reflect.DeepEqual(stats, StatsOutput{}) {
		t.Fatal(log, stats, err, statsErr)
	}
}
func TestToolObservationsOwnedJournalAndPreviewBoundaries(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{" \t \n", ""}, {" one \n two \t", "one two"}, {strings.Repeat("🙂", 100), strings.Repeat("🙂", 100)}, {strings.Repeat("🙂", 101), strings.Repeat("🙂", 100) + "…"}} {
		if got := previewString(tc.in); got != tc.want {
			t.Fatal(got, tc.want)
		}
	}
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if _, err := j.Append(event.Spec{Subject: "tool", Kind: event.KindToolResult, Actor: "fixture", Payload: map[string]any{"tool": "foo", "call_id": "denied", "error": true}}); err != nil {
		t.Fatal(err)
	}
	log, err := NewObservations(j).Log(context.Background(), LogInput{Page: journalview.Input{Limit: 20}})
	if err != nil || log.Count != 1 || log.Invocations[0].Seq != int64(0) || log.Invocations[0].DurationMS != int64(0) {
		t.Fatal(log, err)
	}
}

func observationWire(t *testing.T, out StatsOutput) map[string]any {
	t.Helper()
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	return wire
}
func observationWireRow(t *testing.T, item LogItem) map[string]any {
	t.Helper()
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	return wire
}
