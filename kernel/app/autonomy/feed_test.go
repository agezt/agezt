// SPDX-License-Identifier: MIT
package autonomy

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"reflect"
	"strings"
	"testing"
)

type feedJournal struct {
	events []*event.Event
	cause  error
	calls  []int
}

func (p *feedJournal) Tail(n int) ([]*event.Event, error) {
	p.calls = append(p.calls, n)
	return p.events, p.cause
}
func feedEvent(seq int64, kind event.Kind, subject string, payload any) *event.Event {
	raw, _ := json.Marshal(payload)
	return &event.Event{Seq: seq, TSUnixMS: seq * 10, Kind: kind, Subject: subject, CorrelationID: "owned", Payload: raw}
}
func TestAutonomyFeedRecentWindowOrderSelectionBoundsAndEmpty(t *testing.T) {
	events := []*event.Event{feedEvent(0, event.KindScheduleFired, "owned", map[string]any{"intent": "first"}), feedEvent(1, event.KindToolInvoked, "noise", nil), feedEvent(2, event.KindSubAgentSpawned, "anonymous", map[string]any{"task": "anonymous"}), feedEvent(3, event.KindSubAgentSpawned, "named", map[string]any{"agent": " worker ", "task": "named"}), feedEvent(4, event.KindSkillRestored, "skill", map[string]any{"name": "restored"})}
	for _, tc := range []struct {
		limit int
		seqs  []int64
	}{{0, []int64{4}}, {-10, []int64{4}}, {1, []int64{4}}, {2, []int64{4, 3}}, {60, []int64{4, 3, 0}}} {
		port := &feedJournal{events: events}
		out, err := NewFeed(port).List(context.Background(), FeedInput{Limit: tc.limit})
		var seqs []int64
		for _, item := range out.Items {
			seqs = append(seqs, item.Seq)
			for _, field := range []string{"seq", "ts_unix_ms", "kind", "subject", "category", "title", "correlation_id"} {
				if _, ok := feedWireRow(t, item)[field]; !ok {
					t.Fatal("required zero/empty field omitted", field, item)
				}
			}
		}
		if err != nil || out.Count != len(tc.seqs) || !reflect.DeepEqual(seqs, tc.seqs) || !reflect.DeepEqual(port.calls, []int{2000}) {
			t.Fatal(tc, out, err, seqs, port.calls)
		}
	}
	many := make([]*event.Event, 220)
	for i := range many {
		many[i] = feedEvent(int64(i), event.KindScheduleFired, "owned", nil)
	}
	out, err := NewFeed(&feedJournal{events: many}).List(context.Background(), FeedInput{Limit: 1000})
	if err != nil || out.Count != 200 || out.Items[0].Seq != int64(219) || out.Items[199].Seq != int64(20) {
		t.Fatal(out, err)
	}
	empty, err := NewFeed(&feedJournal{}).List(context.Background(), FeedInput{Limit: 60})
	raw, _ := json.Marshal(empty)
	if err != nil || string(raw) != `{"items":[],"count":0}` {
		t.Fatal(string(raw), err)
	}
}
func TestAutonomyFeedDoctorProjectionRetainsRawOptionalStringsAndZeroNumbers(t *testing.T) {
	payload := map[string]any{"agent": " agent ", "target_agent": "lead", "delegate_to": "owner", "delegated_by": "source", "root_agent": "root", "incident_id": "incident", "root_incident_id": "root-i", "parent_incident_id": "parent", "phase": " completed ", "mode": " routing ", "resolution": "force_chain", "routing_task_type": " code ", "routing_task_model_chain": []any{" model-a ", " ", false, "model-b"}, "routing_force_generation": float64(0), "chain_depth": float64(0)}
	ev := feedEvent(0, event.KindInfo, "doctor.auto_repair", payload)
	ev.CorrelationID = ""
	out, err := NewFeed(&feedJournal{events: []*event.Event{ev}}).List(context.Background(), FeedInput{Limit: 1})
	if err != nil || out.Count != 1 {
		t.Fatal(out, err)
	}
	row := feedWireRow(t, out.Items[0])
	for _, key := range []string{"agent", "target_agent", "delegate_to", "delegated_by", "root_agent", "incident_id", "root_incident_id", "parent_incident_id", "phase", "mode", "resolution", "routing_task_type"} {
		if row[key] != payload[key] {
			t.Fatal(key, row, payload)
		}
	}
	if row["seq"] != float64(0) || row["ts_unix_ms"] != float64(0) || row["correlation_id"] != "" || row["routing_force_generation"] != float64(0) || row["chain_depth"] != float64(0) || !reflect.DeepEqual(row["routing_task_model_chain"], []any{" model-a ", "model-b"}) || row["title"] != "a routing repair rewrote a chain" {
		t.Fatal(row)
	}
	payload["routing_force_generation"] = "0"
	payload["chain_depth"] = nil
	payload["agent"] = ""
	payload["routing_task_model_chain"] = []any{false, " "}
	ev = feedEvent(1, event.KindInfo, "doctor.auto_repair", payload)
	out, _ = NewFeed(&feedJournal{events: []*event.Event{ev}}).List(context.Background(), FeedInput{Limit: 1})
	for _, key := range []string{"routing_force_generation", "chain_depth", "agent", "routing_task_model_chain"} {
		if _, ok := feedWireRow(t, out.Items[0])[key]; ok {
			t.Fatal("wrong type/empty value projected", key, out)
		}
	}
	// Non-doctor lifecycle rows do not inherit doctor enrichment keys.
	ev = feedEvent(2, event.KindScheduleFired, "owned", payload)
	out, _ = NewFeed(&feedJournal{events: []*event.Event{ev}}).List(context.Background(), FeedInput{Limit: 1})
	if _, ok := feedWireRow(t, out.Items[0])["root_agent"]; ok {
		t.Fatal(out)
	}
}
func TestAutonomyFeedMalformedPayloadAndOriginalTailCause(t *testing.T) {
	ev := feedEvent(0, event.KindInfo, "doctor.auto_repair", nil)
	ev.Payload = json.RawMessage(`{bad`)
	out, err := NewFeed(&feedJournal{events: []*event.Event{ev}}).List(context.Background(), FeedInput{Limit: 1})
	if err != nil || out.Count != 1 || out.Items[0].Title != "a doctor action ran" {
		t.Fatal(out, err)
	}
	if _, ok := feedWireRow(t, out.Items[0])["detail"]; ok {
		t.Fatal(out)
	}
	ev.Payload = json.RawMessage(`{}`)
	fallback, fallbackErr := NewFeed(&feedJournal{events: []*event.Event{ev}}).List(context.Background(), FeedInput{Limit: 1})
	if fallbackErr != nil || fallback.Count != 1 || fallback.Items[0].Title != "a doctor action ran" {
		t.Fatal(fallback, fallbackErr)
	}
	cause := errors.New("owned tail failure")
	port := &feedJournal{events: []*event.Event{ev}, cause: cause}
	out, err = NewFeed(port).List(context.Background(), FeedInput{Limit: 1})
	if err != cause || !reflect.DeepEqual(out, FeedOutput{}) || !reflect.DeepEqual(port.calls, []int{2000}) {
		t.Fatal(out, err, port.calls)
	}
}
func TestAutonomyFeedOwnedJournalAndUnicodeClipping(t *testing.T) {
	for _, n := range []int{100, 120} {
		text := strings.Repeat("🙂", n)
		if got := clipDetail(text); got != text {
			t.Fatal("short/boundary Unicode changed", n, got)
		}
	}

	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	first, err := j.Append(event.Spec{Subject: "owned", Kind: event.KindScheduleFired, Actor: "fixture", Payload: map[string]any{"intent": strings.Repeat("🙂", 121)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(event.Spec{Subject: "noise", Kind: event.KindToolInvoked, Actor: "fixture"}); err != nil {
		t.Fatal(err)
	}
	out, err := NewFeed(j).List(context.Background(), FeedInput{Limit: 60})
	if err != nil || out.Count != 1 || out.Items[0].Seq != first.Seq || out.Items[0].CorrelationID != "" || feedWireRow(t, out.Items[0])["detail"] != strings.Repeat("🙂", 119)+"…" {
		t.Fatal(out, err)
	}
}

func feedWireRow(t *testing.T, item FeedItem) map[string]any {
	t.Helper()
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
