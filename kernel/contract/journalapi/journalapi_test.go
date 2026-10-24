// SPDX-License-Identifier: MIT

package journalapi

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/eventapi"
)

// TestTheReadPortCarriesExactlyOneVerb: the port is Tail, and Tail answers
// the last n records.
//
// One verb, on purpose: the reader a tool binds must not be handed the
// journal's whole surface (append, recover, verify) — a tool that recalls
// the daemon's past runs has no business writing to the history.
func TestTheReadPortCarriesExactlyOneVerb(t *testing.T) {
	typ := reflect.TypeOf((*Reader)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Reader must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 1 {
		t.Fatalf("the port has %d methods, want 1: %v", typ.NumMethod(), typ)
	}
	m, ok := typ.MethodByName("Tail")
	if !ok {
		t.Fatal("the port must have a Tail method")
	}
	if m.Type.NumIn() != 1 {
		t.Errorf("Tail must take 1 argument, got %v", m.Type)
	}
	if m.Type.In(0) != reflect.TypeOf(int(0)) {
		t.Errorf("Tail must take the window size, got %v", m.Type.In(0))
	}
	if m.Type.NumOut() != 2 {
		t.Errorf("Tail must answer 2 values, got %v", m.Type)
	}
	if m.Type.Out(0) != reflect.TypeOf([]Record{}) {
		t.Errorf("Tail must answer the contract's records, got %v", m.Type.Out(0))
	}
	if m.Type.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Errorf("Tail must answer an error, got %v", m.Type.Out(1))
	}
}

// TestTheRecordKeepsItsFields: the projection carries exactly the four
// things a reader folds — the kind, the run it belongs to, when it happened
// and what it carried. A fifth field would be a second shape to keep in
// step; a missing one would be a history that cannot be folded.
func TestTheRecordKeepsItsFields(t *testing.T) {
	typ := reflect.TypeOf(Record{})
	want := []string{"Kind", "CorrelationID", "TSUnixMS", "Payload"}
	if typ.NumField() != len(want) {
		t.Fatalf("the record has %d fields, want %d", typ.NumField(), len(want))
	}
	for i, name := range want {
		if got := typ.Field(i).Name; got != name {
			t.Errorf("record field %d = %q, want %q", i, got, name)
		}
	}
	if typ.Field(0).Type != reflect.TypeOf(eventapi.Kind("")) {
		t.Errorf("Record.Kind must be the contract's kind, got %v", typ.Field(0).Type)
	}
	if typ.Field(3).Type != reflect.TypeOf(json.RawMessage(nil)) {
		t.Errorf("Record.Payload must be the raw JSON body, got %v", typ.Field(3).Type)
	}
}
