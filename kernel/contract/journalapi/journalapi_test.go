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

// TestTheScanPortCarriesExactlyTwoVerbs: the port is Scan and Head, and
// nothing else.
//
// The scan port exists for the maintenance task that reports the journal's
// pressure: it needs every event's timestamp and the chain's head. It does
// NOT need the journal's append, recover or verify verbs — a reader of the
// history has no business writing to it.
func TestTheScanPortCarriesExactlyTwoVerbs(t *testing.T) {
	typ := reflect.TypeOf((*Scanner)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Scanner must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 2 {
		t.Fatalf("the scan port has %d methods, want 2: %v", typ.NumMethod(), typ)
	}
	scan, ok := typ.MethodByName("Scan")
	if !ok {
		t.Fatal("the scan port must have a Scan method")
	}
	if scan.Type.NumIn() != 1 {
		t.Errorf("Scan must take 1 argument, got %v", scan.Type)
	}
	visit := reflect.TypeOf(func(Record) error { return nil })
	if scan.Type.In(0) != visit {
		t.Errorf("Scan must take the visit function, got %v", scan.Type.In(0))
	}
	if scan.Type.NumOut() != 1 {
		t.Errorf("Scan must answer 1 value, got %v", scan.Type)
	}
	if scan.Type.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Errorf("Scan must answer an error, got %v", scan.Type.Out(0))
	}
	head, ok := typ.MethodByName("Head")
	if !ok {
		t.Fatal("the scan port must have a Head method")
	}
	if head.Type.NumIn() != 0 {
		t.Errorf("Head must take no arguments, got %v", head.Type)
	}
	if head.Type.NumOut() != 2 {
		t.Errorf("Head must answer 2 values, got %v", head.Type)
	}
	if head.Type.Out(0) != reflect.TypeOf(int64(0)) {
		t.Errorf("Head must answer the sequence number, got %v", head.Type.Out(0))
	}
	if head.Type.Out(1) != reflect.TypeOf("") {
		t.Errorf("Head must answer the hash, got %v", head.Type.Out(1))
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
