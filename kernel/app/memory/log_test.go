// SPDX-License-Identifier: MIT

package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	appmemory "github.com/agezt/agezt/kernel/app/memory"
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
func memoryLogFixture() logReader {
	return logReader{events: []event.Event{
		{Kind: event.KindMemoryWritten, TSUnixMS: 100, Seq: 1, Payload: json.RawMessage(`{"id":"write","type":"FACT","subject":"fact"}`)},
		{Kind: event.KindMemoryWritten, TSUnixMS: 100, Seq: 2, Payload: json.RawMessage(`{"action":"revive","id":"revive","type":"FACT"}`)},
		{Kind: event.KindMemoryForgotten, TSUnixMS: 101, Seq: 3, Payload: json.RawMessage(`{"id":"forget"}`)},
		{Kind: event.KindMemorySuperseded, TSUnixMS: 102, Seq: 4, Payload: json.RawMessage(`{"old_id":"old","new_id":"new"}`)},
		{Kind: event.KindMemoryPromoted, TSUnixMS: 103, Seq: 5, Payload: json.RawMessage(`{"id":"shared","subject":"topic","from_scope":"agent"}`)},
		{Kind: event.KindProviderFallback, TSUnixMS: 104, Seq: 6, Payload: json.RawMessage(`{}`)},
	}}
}

func TestMemoryJournalLogRetainsRowsAliasesAndPaging(t *testing.T) {
	service := appmemory.NewLog(memoryLogFixture())
	ctx := context.Background()
	page, err := service.Log(ctx, appmemory.LogInput{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page.Count != 2 || len(page.Ops) != 2 || page.Ops[0].Op != "promote" || page.Ops[0].Subject != "topic (was private to agent)" || page.Ops[1].ID != "old" || page.Ops[1].Subject != "→ new" || page.NextCursor != "102:4" {
		t.Fatalf("first page=%+v", page)
	}
	page, err = service.Log(ctx, appmemory.LogInput{Limit: 2, Cursor: page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if page.Count != 2 || page.Ops[0].ID != "forget" || page.Ops[1].ID != "revive" || page.NextCursor != "100:2" {
		t.Fatalf("second page=%+v", page)
	}
	page, err = service.Log(ctx, appmemory.LogInput{Limit: 2, Cursor: page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if page.Count != 1 || page.Ops[0].ID != "write" || page.NextCursor != "" {
		t.Fatalf("last page=%+v", page)
	}
	for _, tc := range []struct {
		filter string
		count  int
		id     string
	}{{"written", 2, "revive"}, {"write", 2, "revive"}, {"forgotten", 1, "forget"}, {"forget", 1, "forget"}, {"superseded", 1, "old"}, {"supersede", 1, "old"}, {"promoted", 1, "shared"}, {"promote", 1, "shared"}, {"revive", 1, "revive"}, {"absent", 0, ""}} {
		out, err := service.Log(ctx, appmemory.LogInput{Limit: 100, OpFilter: tc.filter})
		if err != nil {
			t.Fatal(err)
		}
		if out.Count != tc.count || len(out.Ops) != tc.count || (tc.count > 0 && out.Ops[0].ID != tc.id) {
			t.Errorf("filter=%s out=%+v", tc.filter, out)
		}
	}
	out, err := service.Log(ctx, appmemory.LogInput{Limit: 100, CutoffMS: 102})
	if err != nil {
		t.Fatal(err)
	}
	if out.Count != 2 || out.Ops[1].TSUnixMS != 102 {
		t.Fatalf("inclusive cutoff=%+v", out)
	}
}

func TestMemoryJournalLogRetainsSelectedReaderEmptyWireAndCauses(t *testing.T) {
	ctx := context.Background()
	reader := logReader{events: []event.Event{{Kind: event.KindMemoryForgotten, TSUnixMS: 0, Seq: 0, Payload: json.RawMessage(`{"id":"only-selected"}`)}}}
	out, err := appmemory.NewLog(reader).Log(ctx, appmemory.LogInput{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"ops":[{"op":"forget","id":"only-selected","type":"","subject":"","ts_unix_ms":0,"seq":0}],"count":1,"next_cursor":""}` {
		t.Fatalf("row field presence=%s", raw)
	}
	out, err = appmemory.NewLog(logReader{}).Log(ctx, appmemory.LogInput{Limit: 20})
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
	cause := errors.New("fixture journal unavailable")
	if _, err := appmemory.NewLog(logReader{cause: cause}).Log(ctx, appmemory.LogInput{Limit: 20}); !errors.Is(err, cause) {
		t.Fatalf("journal cause=%v", err)
	}
	out, err = appmemory.NewLog(logReader{events: []event.Event{{Kind: event.KindMemoryWritten, Payload: json.RawMessage(`{`)}}}).Log(ctx, appmemory.LogInput{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if out.Count != 1 || out.Ops[0].Op != "write" || out.Ops[0].ID != "" {
		t.Fatalf("legacy malformed payload=%+v", out)
	}
}
