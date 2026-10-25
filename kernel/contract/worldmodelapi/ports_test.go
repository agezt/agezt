// SPDX-License-Identifier: MIT

package worldmodelapi

import (
	"reflect"
	"testing"
)

// TestTheStorePortIsExactlyEightVerbsWide: the store port is what the layers
// ABOVE the module drive — eight verbs, no more.
//
// The width is pinned because a WIDER port would let a caller reach past the
// store's own rules (the store snapshots atomically and content-addresses),
// and a NARROWER one would force a second shape to be kept in step.
func TestTheStorePortIsExactlyEightVerbsWide(t *testing.T) {
	typ := reflect.TypeOf((*Store)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Store must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 8 {
		t.Fatalf("the store port has %d methods, want 8", typ.NumMethod())
	}
	for _, name := range []string{"PutEntity", "GetEntity", "AllEntities", "PutRelation", "GetRelation", "AllRelations", "Count", "Close"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the store port must have a %s method", name)
		}
	}
}

// TestTheGraphPortIsExactlyFourteenVerbsWide: the graph port is the fourteen
// verbs the layers above actually drive, no more.
//
// The width is pinned because a WIDER port would let a caller reach past the
// graph's own rules (it content-addresses, normalizes, journals
// durable-before-publish and soft-deletes); a NARROWER one would force a
// second shape to be kept in step with this one.
func TestTheGraphPortIsExactlyFourteenVerbsWide(t *testing.T) {
	typ := reflect.TypeOf((*Graph)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Graph must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 14 {
		t.Fatalf("the graph port has %d methods, want 14", typ.NumMethod())
	}
	for _, name := range []string{"Upsert", "EditEntity", "Relate", "Resolve", "ResolveQuiet", "IsActiveSubject", "Neighbors", "Forget", "Get", "Entities", "Relations", "Count", "Decay", "Tool"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the graph port must have a %s method", name)
		}
	}
}

// TestThePublisherPortIsOneVerb: the publisher answers "which event did it
// write?" and nothing else.
//
// The durable envelope carries the hash chain, and the contract layer may not
// name it — so the port answers with the event's ID, which is what the graph
// records as the entity's provenance.
func TestThePublisherPortIsOneVerb(t *testing.T) {
	typ := reflect.TypeOf((*Publisher)(nil)).Elem()
	if typ.NumMethod() != 1 {
		t.Fatalf("the publisher port has %d methods, want 1", typ.NumMethod())
	}
	m, ok := typ.MethodByName("Publish")
	if !ok {
		t.Fatal("the publisher port must have a Publish method")
	}
	if m.Type.NumOut() != 2 {
		t.Fatalf("Publish must answer (id, error), got %v", m.Type)
	}
	if m.Type.Out(0).Kind() != reflect.String {
		t.Errorf("Publish must answer a string id, got %v", m.Type.Out(0))
	}
	if m.Type.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Errorf("Publish must answer an error, got %v", m.Type.Out(1))
	}
}

// TestTheGraphPortNamesOnlyContractRows: every type the graph port names must
// be the contract's row or a builtin. A module's type in a port's signature
// would drag that module into every layer that holds the port.
func TestTheGraphPortNamesOnlyContractRows(t *testing.T) {
	typ := reflect.TypeOf((*Graph)(nil)).Elem()
	entity := reflect.TypeOf(Entity{})
	relation := reflect.TypeOf(Relation{})
	scored := reflect.TypeOf([]ScoredEntity{})
	neighbors := reflect.TypeOf([]Neighbor{})
	spec := reflect.TypeOf(UpsertSpec{})
	opts := reflect.TypeOf(DecayOptions{})
	verb := reflect.TypeOf(VerbOwns)
	errType := reflect.TypeOf((*error)(nil)).Elem()
	str := reflect.TypeOf("")
	boolean := reflect.TypeOf(false)
	intType := reflect.TypeOf(0)

	check := func(name string, in, out []reflect.Type) {
		m, ok := typ.MethodByName(name)
		if !ok {
			t.Fatalf("the graph port must have a %s method", name)
		}
		if m.Type.NumIn() != len(in) {
			t.Errorf("%s takes %d arguments, want %d", name, m.Type.NumIn(), len(in))
			return
		}
		for i := range in {
			if m.Type.In(i) != in[i] {
				t.Errorf("%s argument %d is %v, want %v", name, i, m.Type.In(i), in[i])
			}
		}
		if m.Type.NumOut() != len(out) {
			t.Errorf("%s answers %d values, want %d", name, m.Type.NumOut(), len(out))
			return
		}
		for i := range out {
			if m.Type.Out(i) != out[i] {
				t.Errorf("%s answer %d is %v, want %v", name, i, m.Type.Out(i), out[i])
			}
		}
	}

	check("Upsert", []reflect.Type{str, spec}, []reflect.Type{entity, boolean, errType})
	check("EditEntity", []reflect.Type{str, str, reflect.TypeOf([]string{}), reflect.TypeOf(map[string]string{})}, []reflect.Type{entity, boolean, errType})
	check("Relate", []reflect.Type{str, str, verb, str}, []reflect.Type{relation, errType})
	check("Resolve", []reflect.Type{str, str, intType}, []reflect.Type{scored, errType})
	check("ResolveQuiet", []reflect.Type{str, intType}, []reflect.Type{scored, errType})
	check("IsActiveSubject", []reflect.Type{str}, []reflect.Type{str, boolean})
	check("Neighbors", []reflect.Type{str}, []reflect.Type{neighbors, errType})
	check("Forget", []reflect.Type{str, str}, []reflect.Type{boolean, errType})
	check("Get", []reflect.Type{str}, []reflect.Type{entity, boolean, errType})
	check("Entities", nil, []reflect.Type{reflect.TypeOf([]Entity{}), errType})
	check("Relations", nil, []reflect.Type{reflect.TypeOf([]Relation{}), errType})
	check("Count", nil, []reflect.Type{intType})
	check("Decay", []reflect.Type{str, opts}, []reflect.Type{intType, errType})
}

// TestTheActiveRulesTravelWithTheRows: the rules that decide whether an
// entity or a relation still counts live with the ROWS.
//
// A Go type alias cannot carry methods, so a rule left in the module could
// not be reached by the layers above it — and these rules decide what the
// graph still believes.
func TestTheActiveRulesTravelWithTheRows(t *testing.T) {
	live := Entity{ID: "e-1"}
	if !live.Active() {
		t.Fatal("a plain entity must be active")
	}
	forgotten := Entity{ID: "e-2", Tombstoned: true}
	if forgotten.Active() {
		t.Fatal("a forgotten entity must not be active")
	}
	superseded := Entity{ID: "e-3", SupersededBy: "e-4"}
	if superseded.Active() {
		t.Fatal("a superseded entity must not be active")
	}
	edge := Relation{ID: "r-1"}
	if !edge.Active() {
		t.Fatal("a plain relation must be active")
	}
	if (Relation{ID: "r-2", Tombstoned: true}).Active() {
		t.Fatal("a forgotten relation must not be active")
	}
}

// TestTheNormalizersAreTheContracts: the open vocabularies keep their
// permissive rules — an unknown kind is kept, an empty one falls back.
func TestTheNormalizersAreTheContracts(t *testing.T) {
	if NormalizeKind("  PROJECT ") != KindProject {
		t.Fatalf("a kind is lower-cased and trimmed: %q", NormalizeKind("  PROJECT "))
	}
	if NormalizeKind("") != DefaultKind {
		t.Fatalf("an empty kind falls back: %q", NormalizeKind(""))
	}
	if NormalizeKind("unheard-of") != "unheard-of" {
		t.Fatal("an unknown kind is kept verbatim — the graph never refuses to learn")
	}
	if NormalizeVerb("  OWNS ") != VerbOwns {
		t.Fatalf("a verb is lower-cased and trimmed: %q", NormalizeVerb("  OWNS "))
	}
	if NormalizeVerb("") != DefaultVerb {
		t.Fatalf("an empty verb falls back: %q", NormalizeVerb(""))
	}
}

// TestTheSentinelIsTheContracts: the error the layers above the module
// compare against must be the contract's.
func TestTheSentinelIsTheContracts(t *testing.T) {
	if ErrEmptyName == nil {
		t.Fatal("the contract must own the empty-name sentinel")
	}
	if ErrEmptyName.Error() != "worldmodel: empty entity name" {
		t.Fatalf("the sentinel reads %q", ErrEmptyName.Error())
	}
}
