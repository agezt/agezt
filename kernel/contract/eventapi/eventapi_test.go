// SPDX-License-Identifier: MIT

package eventapi

import (
	"reflect"
	"testing"
)

// TestThePublishPortCarriesExactlyOneVerb: the port is Publish, and Publish
// answers one thing — did the event land.
//
// The platform's bus answers two (the event it wrote, and the error). The
// durable event it would return carries the hash chain, and the contract
// layer may not name that (archcheck's principle P4) — so the port asks the
// only question a publisher can answer without it.
func TestThePublishPortCarriesExactlyOneVerb(t *testing.T) {
	typ := reflect.TypeOf((*Publisher)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Publisher must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 1 {
		t.Fatalf("the port has %d methods, want 1: %v", typ.NumMethod(), typ)
	}
	m, ok := typ.MethodByName("Publish")
	if !ok {
		t.Fatal("the port must have a Publish method")
	}
	if m.Type.NumIn() != 1 {
		t.Errorf("Publish must take 1 argument, got %v", m.Type)
	}
	if m.Type.In(0) != reflect.TypeOf(Spec{}) {
		t.Errorf("Publish must take the contract's spec, got %v", m.Type.In(0))
	}
	if m.Type.NumOut() != 1 {
		t.Errorf("Publish must answer 1 value, got %v", m.Type)
	}
	if m.Type.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Errorf("Publish must answer an error, got %v", m.Type.Out(0))
	}
}

// TestTheSpecKeepsItsFields: the row the bus takes, field for field and in
// order. The seven fields are what a caller fills in; a dropped one is a
// caller that cannot say WHO emitted an event or WHICH run it belongs to.
func TestTheSpecKeepsItsFields(t *testing.T) {
	typ := reflect.TypeOf(Spec{})
	want := []string{"Subject", "Kind", "Actor", "CorrelationID", "CausationID", "Payload", "Tags"}
	if typ.NumField() != len(want) {
		t.Fatalf("the spec has %d fields, want %d", typ.NumField(), len(want))
	}
	for i, name := range want {
		if got := typ.Field(i).Name; got != name {
			t.Errorf("spec field %d = %q, want %q", i, got, name)
		}
	}
	if typ.Field(1).Type != reflect.TypeOf(Kind("")) {
		t.Errorf("Spec.Kind must be the contract's kind, got %v", typ.Field(1).Type)
	}
}

// TestTheKindIsAStringRow: the discriminator is a plain string row — the
// vocabulary belongs to the package that owns the event registry, and the
// contract only names the TYPE.
func TestTheKindIsAStringRow(t *testing.T) {
	if reflect.TypeOf(Kind("")).Kind() != reflect.String {
		t.Fatalf("Kind must be a string row, got %v", reflect.TypeOf(Kind("")).Kind())
	}
	if reflect.TypeOf(Kind("")).Name() != "Kind" {
		t.Fatalf("the kind row must be named Kind, got %v", reflect.TypeOf(Kind("")).Name())
	}
}
