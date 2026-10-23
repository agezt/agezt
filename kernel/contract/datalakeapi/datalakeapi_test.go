// SPDX-License-Identifier: MIT

package datalakeapi

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestTheSchemaWireShape: a collection's definition is PERSISTED (the lake
// writes one JSON file per collection), so the row's keys are a contract with
// every collection file already on disk — and with the Web UI that reads
// them.
func TestTheSchemaWireShape(t *testing.T) {
	sc := Schema{
		Name:      "expenses",
		Title:     "Expenses",
		Icon:      "wallet",
		View:      "expense",
		Desc:      "what we spent",
		Fields:    []Field{{Name: "amount", Type: "money", Label: "Amount"}},
		Builtin:   true,
		System:    true,
		CreatedMs: 10,
		CreatedBy: "daemon",
	}
	raw, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"name", "title", "icon", "view", "desc", "fields", "builtin", "system", "created_ms", "created_by"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("the wire must carry %q: %s", key, raw)
		}
	}
	fields, _ := got["fields"].([]any)
	if len(fields) != 1 {
		t.Fatalf("the wire must carry the fields: %s", raw)
	}
	field, _ := fields[0].(map[string]any)
	for _, key := range []string{"name", "type", "label"} {
		if _, ok := field[key]; !ok {
			t.Fatalf("the field wire must carry %q: %s", key, raw)
		}
	}
}

// TestTheOptionalFieldsFallOutOfTheWire: an empty optional field must not
// appear — a stored collection with `"title": ""` would be a different file
// than one without the key.
func TestTheOptionalFieldsFallOutOfTheWire(t *testing.T) {
	raw, err := json.Marshal(Schema{Name: "x"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"title", "icon", "view", "desc", "fields", "builtin", "system", "created_by"} {
		if _, ok := got[key]; ok {
			t.Fatalf("an empty %q must fall out of the wire: %s", key, raw)
		}
	}
	for _, key := range []string{"name", "created_ms"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("the wire must always carry %q: %s", key, raw)
		}
	}
}

// TestTheOptionalFieldColumnsFallOutOfTheWire: a column with no type and no
// label must not carry empty keys — the stored schema would differ from the
// same schema written by another version.
func TestTheOptionalFieldColumnsFallOutOfTheWire(t *testing.T) {
	raw, err := json.Marshal(Field{Name: "amount"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"type", "label"} {
		if _, ok := got[key]; ok {
			t.Fatalf("an empty %q must fall out of the wire: %s", key, raw)
		}
	}
	if _, ok := got["name"]; !ok {
		t.Fatalf("the wire must always carry the name: %s", raw)
	}
}

// TestTheRecordWireShape: a record is persisted too, and its provenance
// answers the operator's "which agent put this here?".
func TestTheRecordWireShape(t *testing.T) {
	rec := Record{
		ID:        "r1",
		Fields:    map[string]any{"amount": 12.5},
		CreatedMs: 10,
		UpdatedMs: 20,
		CreatedBy: "agent-a",
		UpdatedBy: "agent-b",
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"id", "fields", "created_ms", "updated_ms", "created_by", "updated_by"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("the wire must carry %q: %s", key, raw)
		}
	}
	fields, _ := got["fields"].(map[string]any)
	if fields["amount"] != 12.5 {
		t.Fatalf("the field map must survive the wire: %s", raw)
	}

	empty, err := json.Marshal(Record{ID: "r2", Fields: map[string]any{}})
	if err != nil {
		t.Fatalf("Marshal empty: %v", err)
	}
	var gotEmpty map[string]any
	if err := json.Unmarshal(empty, &gotEmpty); err != nil {
		t.Fatalf("Unmarshal empty: %v", err)
	}
	for _, key := range []string{"created_by", "updated_by"} {
		if _, ok := gotEmpty[key]; ok {
			t.Fatalf("an empty %q must fall out of the wire: %s", key, empty)
		}
	}
	for _, key := range []string{"id", "fields", "created_ms", "updated_ms"} {
		if _, ok := gotEmpty[key]; !ok {
			t.Fatalf("the wire must always carry %q: %s", key, empty)
		}
	}
}

// TestTheCollectionSummaryCarriesTheSchema: the listing's summary embeds the
// collection's definition — so a renamed schema key would empty the summary
// the UI renders.
func TestTheCollectionSummaryCarriesTheSchema(t *testing.T) {
	raw, err := json.Marshal(CollectionInfo{Schema: Schema{Name: "expenses", View: "expense"}, Count: 3})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"name", "view", "count"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("the wire must carry %q: %s", key, raw)
		}
	}
	if got["count"] != float64(3) {
		t.Fatalf("count = %v", got["count"])
	}
}

// TestTheErrorsSayWhatTheyAre: the tool maps these three refusals to three
// different answers, so the messages are part of the contract.
func TestTheErrorsSayWhatTheyAre(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
	}{
		{ErrNotFound, "datalake: not found"},
		{ErrExists, "datalake: collection already exists"},
		{ErrSystem, "datalake: system collection cannot be dropped"},
	} {
		if c.err == nil {
			t.Fatalf("the %q error must exist", c.want)
		}
		if c.err.Error() != c.want {
			t.Errorf("error = %q, want %q", c.err.Error(), c.want)
		}
	}
}

// TestTheLakePortShape: the port is the contract the agent-facing tool drives
// the lake through, so its method set is fixed — an extra method would be a
// capability the plugin could reach without the daemon's consent.
func TestTheLakePortShape(t *testing.T) {
	typ := reflect.TypeOf((*Lake)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Lake must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 9 {
		names := make([]string, 0, typ.NumMethod())
		for i := 0; i < typ.NumMethod(); i++ {
			names = append(names, typ.Method(i).Name)
		}
		t.Fatalf("the port must have exactly the nine lake methods, got %v", names)
	}
	for _, name := range []string{"ListCollections", "CreateCollection", "DropCollection", "Schema", "Insert", "Get", "Update", "Delete", "Query"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the port must have %s", name)
		}
	}
	list, _ := typ.MethodByName("ListCollections")
	if list.Type.NumOut() != 1 || list.Type.Out(0) != reflect.TypeOf([]CollectionInfo{}) {
		t.Errorf("ListCollections must return []CollectionInfo, got %v", list.Type)
	}
	insert, _ := typ.MethodByName("Insert")
	if insert.Type.NumIn() != 3 || insert.Type.NumOut() != 2 ||
		insert.Type.In(1) != reflect.TypeOf(map[string]any{}) ||
		insert.Type.Out(0) != reflect.TypeOf(Record{}) {
		t.Errorf("Insert must take (coll, fields, actor) and return (Record, error), got %v", insert.Type)
	}
	query, _ := typ.MethodByName("Query")
	if query.Type.NumIn() != 2 || query.Type.NumOut() != 2 ||
		query.Type.In(1) != reflect.TypeOf(Query{}) ||
		query.Type.Out(0) != reflect.TypeOf([]Record{}) {
		t.Errorf("Query must take (coll, Query) and return ([]Record, error), got %v", query.Type)
	}
}
