// SPDX-License-Identifier: MIT
package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

type fixtureJournal struct {
	events []*event.Event
	err    error
	calls  int
}

func (r *fixtureJournal) Range(visit func(*event.Event) error) error {
	r.calls++
	for _, e := range r.events {
		if err := visit(e); err != nil {
			return err
		}
	}
	return r.err
}
func delivery(kind event.Kind, ts, seq int64, payload string) *event.Event {
	return &event.Event{Kind: kind, TSUnixMS: ts, Seq: seq, Payload: json.RawMessage(payload)}
}

func TestWebhookObservabilityOrderingPagingPresenceAndWindow(t *testing.T) {
	r := &fixtureJournal{events: []*event.Event{
		delivery(event.KindWebhookDelivered, 800, 1, `{"url":"a","event_kind":"task.completed","status":201,"attempts":2}`),
		delivery(event.KindWebhookFailed, 900, 2, `{"url":"b","error":"owned failure","attempts":3}`),
		delivery(event.KindWebhookDelivered, 900, 3, `{"url":"a"}`),
		delivery(event.KindWebhookFailed, 700, 4, `{broken`),
		delivery(event.KindTaskReceived, 1000, 5, `{"url":"irrelevant"}`),
	}}
	s := NewObservability(r, func() int64 { return 1000 })
	limit := 2
	out, err := s.Log(context.Background(), LogInput{Limit: &limit})
	if err != nil || len(out.Deliveries) != 2 || out.Count != 2 || out.NextCursor != "900:2" || out.Deliveries[0].Seq != 3 || out.Deliveries[1].Seq != 2 {
		t.Fatal(out, err)
	}
	if !out.Deliveries[0].OK || out.Deliveries[1].OK || out.Deliveries[0].TSUnixMS != 900 || out.Deliveries[0].Status == nil || *out.Deliveries[0].Status != 0 || out.Deliveries[0].Error != nil || out.Deliveries[1].Status != nil || out.Deliveries[1].Error == nil || *out.Deliveries[1].Error != "owned failure" {
		t.Fatal("row presence", out)
	}
	page, err := s.Log(context.Background(), LogInput{Limit: &limit, Cursor: out.NextCursor})
	if err != nil || page.Count != 2 || page.Deliveries[0].Seq != 1 || page.Deliveries[1].Seq != 4 {
		t.Fatal(page, err)
	}
	first := page.Deliveries[0]
	if first.URL != "a" || first.EventKind != "task.completed" || first.Attempts != 2 || first.Status == nil || *first.Status != 201 {
		t.Fatal("payload lost", first)
	}
	failed, err := s.Log(context.Background(), LogInput{FailedOnly: true, SinceMS: 100})
	if err != nil || failed.Count != 1 || failed.Deliveries[0].Seq != 2 {
		t.Fatal(failed, err)
	}
	stats, err := s.Stats(context.Background(), 200)
	if err != nil || stats.Total != 3 || stats.Delivered != 2 || stats.Failed != 1 || stats.FailureRate != 1.0/3 || stats.WindowMS != 200 || len(stats.ByURL) != 2 || stats.ByURL["a"] != (URLCounts{Delivered: 2}) || stats.ByURL["b"] != (URLCounts{Failed: 1}) {
		t.Fatal(stats, err)
	}
	all, err := s.Stats(context.Background(), -7)
	if err != nil || all.Total != 4 || all.WindowMS != -7 || all.ByURL[""] != (URLCounts{Failed: 1}) {
		t.Fatal(all, err)
	}
	if r.calls != 5 {
		t.Fatal(r.calls)
	}
}

func TestWebhookObservabilityLimitsEmptyMalformedAndErrors(t *testing.T) {
	r := &fixtureJournal{}
	for i := 0; i < 1002; i++ {
		r.events = append(r.events, delivery(event.KindWebhookFailed, int64(i), int64(i), `{"error":""}`))
	}
	s := NewObservability(r, nil)
	for _, limit := range []int{-1, 0, 1, 1001} {
		out, err := s.Log(context.Background(), LogInput{Limit: &limit, Cursor: "bad"})
		want := limit
		if want < 1 {
			want = 1
		}
		if want > 1000 {
			want = 1000
		}
		if err != nil || out.Count != want || len(out.Deliveries) != want || out.Deliveries[0].Error == nil || *out.Deliveries[0].Error != "" {
			t.Fatal(limit, out.Count, err)
		}
	}
	r.events = nil
	out, err := s.Log(context.Background(), LogInput{})
	stats, statsErr := s.Stats(context.Background(), 0)
	if err != nil || statsErr != nil || out.Deliveries == nil || out.Count != 0 || out.NextCursor != "" || stats.ByURL == nil || stats.Total != 0 || stats.FailureRate != 0 {
		t.Fatal(out, stats, err, statsErr)
	}
	cause := errors.New("owned Range failure")
	r.err = cause
	if _, err := s.Log(context.Background(), LogInput{}); !errors.Is(err, cause) {
		t.Fatal(err)
	}
	if _, err := s.Stats(context.Background(), 0); !errors.Is(err, cause) {
		t.Fatal(err)
	}
}
