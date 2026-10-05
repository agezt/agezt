// SPDX-License-Identifier: MIT

package world_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	appworld "github.com/agezt/agezt/kernel/app/world"
	"github.com/agezt/agezt/kernel/event"
)

type logReader struct {
	events []event.Event
	cause  error
}

func (r logReader) Range(fn func(*event.Event) error) error {
	if r.cause != nil {
		return r.cause
	}
	for _, e := range r.events {
		copy := e
		if err := fn(&copy); err != nil {
			return err
		}
	}
	return nil
}
func worldLogFixture() logReader {
	return logReader{events: []event.Event{
		{Kind: event.KindWorldEntityUpserted, TSUnixMS: 100, Seq: 1, Payload: json.RawMessage(`{"name":"Ada","kind":"person"}`)},
		{Kind: event.KindWorldRelationUpserted, TSUnixMS: 100, Seq: 2, Payload: json.RawMessage(`{"action":"observe","from":"Ada","verb":"wrote","to":"Notes"}`)},
		{Kind: event.KindWorldForgotten, TSUnixMS: 101, Seq: 3, Payload: json.RawMessage(`{"name":"Ada","what":"entity"}`)},
		{Kind: event.KindWorldForgotten, TSUnixMS: 102, Seq: 4, Payload: json.RawMessage(`{"verb":"owns","what":"relation"}`)},
		{Kind: event.KindMemoryWritten, TSUnixMS: 103, Seq: 5, Payload: json.RawMessage(`{}`)},
	}}
}

func TestWorldJournalLogRetainsLabelsKindAndPageBoundaries(t *testing.T) {
	service := appworld.NewLog(worldLogFixture())
	ctx := context.Background()
	out, err := service.Log(ctx, appworld.LogInput{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if out.Count != 2 || out.Ops[0].Label != "owns" || out.Ops[0].Op != "forget" || out.Ops[0].What != "relation" || out.Ops[1].Label != "Ada" || out.NextCursor != "101:3" {
		t.Fatalf("first page=%+v", out)
	}
	out, err = service.Log(ctx, appworld.LogInput{Limit: 2, Cursor: out.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if out.Count != 2 || out.Ops[0].Label != "Ada wrote Notes" || out.Ops[0].Op != "observe" || out.Ops[1].Label != "Ada [person]" || out.Ops[1].Op != "upsert" || out.NextCursor != "100:1" {
		t.Fatalf("second page=%+v", out)
	}
	out, err = service.Log(ctx, appworld.LogInput{Limit: 2, Cursor: out.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if out.Count != 0 || out.NextCursor != "" {
		t.Fatalf("terminal page=%+v", out)
	}
	for _, tc := range []struct {
		kind  string
		count int
	}{{"", 4}, {"entity", 2}, {"relation", 2}, {"absent", 0}} {
		out, err = service.Log(ctx, appworld.LogInput{Limit: 20, KindFilter: tc.kind})
		if err != nil {
			t.Fatal(err)
		}
		if out.Count != tc.count || len(out.Ops) != tc.count {
			t.Errorf("kind=%s output=%+v", tc.kind, out)
		}
		for _, row := range out.Ops {
			if tc.kind != "" && row.What != tc.kind {
				t.Errorf("foreign kind row=%+v", row)
			}
		}
	}
	out, err = service.Log(ctx, appworld.LogInput{Limit: 20, CutoffMS: 101})
	if err != nil {
		t.Fatal(err)
	}
	if out.Count != 2 || out.Ops[1].TSUnixMS != 101 {
		t.Fatalf("inclusive cutoff=%+v", out)
	}
}

func TestWorldJournalLogRetainsSelectedReaderEmptyWireAndCauses(t *testing.T) {
	ctx := context.Background()
	reader := logReader{events: []event.Event{{Kind: event.KindWorldForgotten, Payload: json.RawMessage(`{}`)}}}
	out, err := appworld.NewLog(reader).Log(ctx, appworld.LogInput{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"ops":[{"op":"forget","what":"","label":"","ts_unix_ms":0,"seq":0}],"count":1,"next_cursor":""}` {
		t.Fatalf("row field presence=%s", raw)
	}
	out, err = appworld.NewLog(logReader{}).Log(ctx, appworld.LogInput{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"ops":[],"count":0,"next_cursor":""}` {
		t.Fatalf("empty wire=%s", raw)
	}
	cause := errors.New("fixture world journal unavailable")
	if _, err := appworld.NewLog(logReader{cause: cause}).Log(ctx, appworld.LogInput{Limit: 20}); !errors.Is(err, cause) {
		t.Fatalf("journal cause=%v", err)
	}
	out, err = appworld.NewLog(logReader{events: []event.Event{{Kind: event.KindWorldEntityUpserted, Payload: json.RawMessage(`{`)}}}).Log(ctx, appworld.LogInput{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if out.Count != 1 || out.Ops[0].Op != "upsert" || out.Ops[0].What != "entity" || out.Ops[0].Label != "" {
		t.Fatalf("legacy malformed payload=%+v", out)
	}
}
