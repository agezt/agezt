// SPDX-License-Identifier: MIT

package data

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/datalake"
	"github.com/agezt/agezt/kernel/platform/schema"
)

func lake(t *testing.T) *datalake.Lake {
	t.Helper()
	clock := int64(1_000)
	l, err := datalake.Open(t.TempDir(), func() int64 { clock++; return clock })
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func req[T any](t *testing.T, raw string) T {
	t.Helper()
	var in T
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func encode(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestUnavailable(t *testing.T) {
	ctx := context.Background()
	s := New(nil)
	for _, err := range []error{
		func() error { _, err := s.Collections(ctx, CollectionsRequest{}); return err }(),
		func() error { _, err := s.Records(ctx, RecordsRequest{}); return err }(),
		func() error { _, err := s.Insert(ctx, InsertRequest{}); return err }(),
		func() error { _, err := s.Update(ctx, UpdateRequest{}); return err }(),
		func() error { _, err := s.Delete(ctx, DeleteRequest{}); return err }(),
		func() error { _, err := s.Create(ctx, CreateRequest{}); return err }(),
		func() error { _, err := s.Drop(ctx, DropRequest{}); return err }(),
	} {
		if err == nil || err.Error() != "data lake unavailable" {
			t.Fatal("no lake refuses before any argument", err)
		}
	}
}

func TestArguments(t *testing.T) {
	for raw, want := range map[string]string{``: "args.collection required", `"  "`: "args.collection required", `null`: "args.collection must be a string", `7`: "args.collection must be a string"} {
		if _, err := required(json.RawMessage(raw), "collection"); err == nil || err.Error() != want {
			t.Fatal(raw, err)
		}
	}
	if got, err := required(json.RawMessage(`" a "`), "id"); err != nil || got != " a " {
		t.Fatal("a required value stays untrimmed", got, err)
	}
	for raw, want := range map[string]string{``: "", `null`: "", `7`: "", `" x "`: "x"} {
		if got := text(json.RawMessage(raw)); got != want {
			t.Fatal(raw, got)
		}
	}
	for raw, want := range map[string]bool{``: false, `true`: true, `false`: false, `"true"`: true, `"1"`: true, `"yes"`: false, `1`: false} {
		if got := flag(json.RawMessage(raw)); got != want {
			t.Fatal(raw, got)
		}
	}
	for raw, want := range map[string]int{``: 0, `null`: 0, `3.9`: 3, `-2.5`: -2, `"42"`: 42, `" 4"`: 0, `"-1"`: 0, `"1e3"`: 0, `""`: 0, `true`: 0} {
		if got := count(json.RawMessage(raw)); got != want {
			t.Fatal(raw, got)
		}
	}
	for raw, want := range map[string]string{`7`: "args.record must be an object", `"x"`: "args.record must be an object", `[]`: "args.record must be an object"} {
		if _, err := object(json.RawMessage(raw), "record"); err == nil || err.Error() != want {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{``, `null`} {
		if m, err := object(json.RawMessage(raw), "record"); m != nil || err != nil {
			t.Fatal(raw, m, err)
		}
	}
}

func TestCollectionsAndCreate(t *testing.T) {
	ctx := context.Background()
	l := lake(t)
	s := New(l)
	base, err := s.Collections(ctx, CollectionsRequest{})
	if err != nil || base.Count != len(base.Collections) {
		t.Fatal(base, err)
	}
	for raw, want := range map[string]string{
		`{}`:                                 "collection.name required",
		`{"collection":null}`:                "collection.name required",
		`{"collection":{"name":7}}`:          "collection.name required",
		`{"collection":"books"}`:             "args.collection must be an object",
		`{"collection":{"name":"bad name"}}`: `datalake: invalid collection name "bad name" (use letters, digits, - or _)`,
	} {
		if _, err := s.Create(ctx, req[CreateRequest](t, raw)); err == nil || err.Error() != want {
			t.Fatalf("%s: %v, want %q", raw, err, want)
		}
	}
	out, err := s.Create(ctx, req[CreateRequest](t, `{"collection":{"name":"books","title":"Books","icon":7,"view":"table","desc":"d","fields":[{"name":"title","type":"text","label":"Title"},"skip",{"name":"year","type":9}]}}`))
	if got := encode(t, out); err != nil || got != `{"collection":{"name":"books","title":"Books","icon":"","view":"table","desc":"d","fields":[{"name":"title","type":"text","label":"Title"},{"name":"year","type":"","label":""}],"builtin":false,"system":false,"count":0,"created_ms":1001,"created_by":"operator"}}` {
		t.Fatal(got, err)
	}
	if _, err := s.Create(ctx, req[CreateRequest](t, `{"collection":{"name":"books"}}`)); err == nil || err.Error() != "collection already exists: books" {
		t.Fatal(err)
	}
	after, _ := s.Collections(ctx, CollectionsRequest{})
	if after.Count != base.Count+1 {
		t.Fatal(after)
	}
	bare := collection(datalake.Schema{Name: "n"}, 3)
	if got := encode(t, bare); got != `{"name":"n","title":"","icon":"","view":"","desc":"","fields":[],"builtin":false,"system":false,"count":3,"created_ms":0,"created_by":""}` {
		t.Fatal("every collection field is present", got)
	}
}

func TestRecords(t *testing.T) {
	ctx := context.Background()
	l := lake(t)
	s := New(l)
	if _, err := s.Create(ctx, req[CreateRequest](t, `{"collection":{"name":"notes"}}`)); err != nil {
		t.Fatal(err)
	}
	for raw, want := range map[string]string{
		`{"collection":"notes","record":[1]}`: "args.record must be an object",
		`{"collection":"ghost","record":{}}`:  "no such collection or record: ghost",
		`{"record":{}}`:                       "args.collection required",
	} {
		if _, err := s.Insert(ctx, req[InsertRequest](t, raw)); err == nil || err.Error() != want {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	a, err := s.Insert(ctx, req[InsertRequest](t, `{"collection":"notes","record":{"text":"Alpha","n":1.5}}`))
	if err != nil || a.Record.CreatedBy != "operator" || a.Record.Fields["text"] != "Alpha" {
		t.Fatal(a, err)
	}
	b, _ := s.Insert(ctx, req[InsertRequest](t, `{"collection":"notes","record":null}`))
	if got := encode(t, b.Record.Fields); got != "null" && got != "{}" {
		t.Fatal("a null record inserts no fields", got)
	}
	for raw, want := range map[string]string{
		`{"collection":"notes"}`:                       "args.id required",
		`{"collection":"notes","id":"x","record":"y"}`: "args.record must be an object",
		`{"collection":"notes","id":"x","record":{}}`:  "no such collection or record: notes/x",
	} {
		if _, err := s.Update(ctx, req[UpdateRequest](t, raw)); err == nil || err.Error() != want {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	up, err := s.Update(ctx, req[UpdateRequest](t, `{"collection":"notes","id":"`+a.Record.ID+`","record":{"text":"Beta"}}`))
	if err != nil || up.Record.UpdatedBy != "operator" || up.Record.Fields["text"] != "Beta" {
		t.Fatal(up, err)
	}
	if _, err := s.Records(ctx, req[RecordsRequest](t, `{"collection":"ghost"}`)); err == nil || err.Error() != "no such collection or record: ghost" {
		t.Fatal(err)
	}
	out, err := s.Records(ctx, req[RecordsRequest](t, `{"collection":"notes","search":" bet ","limit":"5"}`))
	if err != nil || out.Count != 1 || out.Records[0].ID != a.Record.ID || out.Schema.Name != "notes" || out.Schema.Count != 1 || out.Collection != "notes" {
		t.Fatal("search finds the edited record", encode(t, out), err)
	}
	none, _ := s.Records(ctx, req[RecordsRequest](t, `{"collection":"notes","search":"no-such-text"}`))
	if none.Count != 0 || encode(t, none.Records) != "[]" {
		t.Fatal("no match is an empty array", encode(t, none))
	}
	first, _ := s.Records(ctx, req[RecordsRequest](t, `{"collection":"notes","limit":1.9}`))
	if first.Count != 1 || first.Records[0].ID != b.Record.ID {
		t.Fatal("a truncated limit keeps the newest record", encode(t, first))
	}
	rest, _ := s.Records(ctx, req[RecordsRequest](t, `{"collection":"notes","offset":"1"}`))
	if rest.Count != 1 || rest.Records[0].ID != a.Record.ID {
		t.Fatal("an offset skips the newest record", encode(t, rest))
	}
	// The newest record sorts last by text, so a text sort disagrees with the
	// creation-time default in both directions.
	c, err := s.Insert(ctx, req[InsertRequest](t, `{"collection":"notes","record":{"text":"Zulu"}}`))
	if err != nil {
		t.Fatal(err)
	}
	position := func(out RecordsOutput, id string) int {
		for i, r := range out.Records {
			if r.ID == id {
				return i
			}
		}
		t.Fatal("missing", id, encode(t, out))
		return -1
	}
	asc, _ := s.Records(ctx, req[RecordsRequest](t, `{"collection":"notes","sort":" text "}`))
	desc, _ := s.Records(ctx, req[RecordsRequest](t, `{"collection":"notes","sort":"text","desc":"1"}`))
	newest, _ := s.Records(ctx, req[RecordsRequest](t, `{"collection":"notes"}`))
	if position(asc, a.Record.ID) > position(asc, c.Record.ID) || position(desc, c.Record.ID) > position(desc, a.Record.ID) || position(newest, c.Record.ID) > position(newest, a.Record.ID) {
		t.Fatal("a text sort orders Beta before Zulu, descending reverses it, and the default is newest first", encode(t, asc), encode(t, desc), encode(t, newest))
	}
	if got := encode(t, record(datalake.Record{ID: "r"})); got != `{"id":"r","fields":null,"created_ms":0,"updated_ms":0,"created_by":"","updated_by":""}` {
		t.Fatal("every record field is present", got)
	}
	if got := encode(t, record(datalake.Record{ID: "r", Fields: map[string]any{}, CreatedMs: 1, UpdatedMs: 2, CreatedBy: "agent", UpdatedBy: "operator"})); got != `{"id":"r","fields":{},"created_ms":1,"updated_ms":2,"created_by":"agent","updated_by":"operator"}` {
		t.Fatal("provenance is copied field by field", got)
	}
	for raw, want := range map[string]string{
		`{"collection":"notes"}`:          "args.id required",
		`{"collection":"notes","id":7}`:   "args.id must be a string",
		`{"collection":"notes","id":"x"}`: "no such collection or record: notes/x",
	} {
		if _, err := s.Delete(ctx, req[DeleteRequest](t, raw)); err == nil || err.Error() != want {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	del, err := s.Delete(ctx, req[DeleteRequest](t, `{"collection":"notes","id":"`+b.Record.ID+`"}`))
	if got := encode(t, del); err != nil || got != `{"deleted":true,"id":"`+b.Record.ID+`"}` {
		t.Fatal(got, err)
	}
}

func TestDrop(t *testing.T) {
	ctx := context.Background()
	l := lake(t)
	s := New(l)
	if _, err := l.SeedBuiltins("test"); err != nil {
		t.Fatal(err)
	}
	var system string
	for _, c := range l.ListCollections() {
		if c.System {
			system = c.Name
		}
	}
	if system == "" {
		t.Fatal("the built-ins include a system collection")
	}
	if _, err := s.Create(ctx, req[CreateRequest](t, `{"collection":{"name":"tmp"}}`)); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{`{}`: "args.name required", `{"name":null}`: "args.name must be a string", `{"name":"ghost"}`: "no such collection: ghost"}
	if system != "" {
		cases[`{"name":"`+system+`"}`] = "built-in collection cannot be dropped: " + system
	}
	for raw, want := range cases {
		if _, err := s.Drop(ctx, req[DropRequest](t, raw)); err == nil || err.Error() != want {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	out, err := s.Drop(ctx, req[DropRequest](t, `{"name":"tmp"}`))
	if got := encode(t, out); err != nil || got != `{"dropped":"tmp"}` {
		t.Fatal(got, err)
	}
	if !errors.Is(dataErr("x", datalake.ErrExists), datalake.ErrExists) {
		t.Fatal("other errors pass through")
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("provider is required")
	}
	ops, err := Operations(func(context.Context) *Service { return nil })
	if err != nil || len(ops) != 7 {
		t.Fatal(len(ops), err)
	}
	for i, want := range []struct {
		name, method, path string
		read               bool
		input, output      reflect.Type
		props              string
	}{
		{"data_collections", "GET", "/api/data/collections", true, reflect.TypeFor[CollectionsRequest](), reflect.TypeFor[CollectionsOutput](), ""},
		{"data_records", "GET", "/api/data/records", true, reflect.TypeFor[RecordsRequest](), reflect.TypeFor[RecordsOutput](), `"collection":{},"search":{},"sort":{},"desc":{},"limit":{},"offset":{}`},
		{"data_insert", "POST", "/api/data/insert", false, reflect.TypeFor[InsertRequest](), reflect.TypeFor[RecordOutput](), `"collection":{},"record":{}`},
		{"data_update", "POST", "/api/data/update", false, reflect.TypeFor[UpdateRequest](), reflect.TypeFor[RecordOutput](), `"collection":{},"id":{},"record":{}`},
		{"data_delete", "POST", "/api/data/delete", false, reflect.TypeFor[DeleteRequest](), reflect.TypeFor[DeleteOutput](), `"collection":{},"id":{}`},
		{"data_create_collection", "", "", false, reflect.TypeFor[CreateRequest](), reflect.TypeFor[CreateOutput](), `"collection":{}`},
		{"data_drop_collection", "", "", false, reflect.TypeFor[DropRequest](), reflect.TypeFor[DropOutput](), `"name":{}`},
	} {
		s := ops[i].Spec()
		out, err := schema.FromType(want.output, false)
		if err != nil {
			t.Fatal(err)
		}
		if s.Name != want.name || s.ReadOnly != want.read || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.HTTP.Method != want.method || s.HTTP.Path != want.path || s.Input != want.input || s.Output != want.output || string(s.OutputSchema) != string(out) {
			t.Fatalf("%s spec: %+v", want.name, s)
		}
		if want.props != "" && !strings.Contains(string(s.InputSchema), `"properties":{`+want.props+`}`) {
			t.Fatal(want.name, string(s.InputSchema))
		}
		for _, in := range []string{`{}`, `{"collection":null,"id":7,"record":"x","tenant":"t"}`} {
			if err := schema.ValidateJSON(s.InputSchema, json.RawMessage(in)); err != nil {
				t.Fatal(want.name, in, err)
			}
		}
	}
}
