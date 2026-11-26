// SPDX-License-Identifier: MIT

package acpserverapi

// The contract's own guards.
//
// The door measures an arriving params object against the schema this package
// names, so a schema that is malformed or says nothing about a field would
// either refuse everything or accept anything. The tests below hold the table
// to that — and the shape guard to itself: a schema that mistyped a field
// must be CAUGHT, or the guard would agree with a wrong schema and pass.

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/jsonshape"
)

// TestTheMethodTableIsWellFormed: every method has an operation name and a
// wire name, and no two claim the same one.
//
// The operation name is the registry's grammar — an identifier, not a path —
// because the refusal reads "operation session_new input: …", not "operation
// session/new input: …".
func TestTheMethodTableIsWellFormed(t *testing.T) {
	ops := map[string]bool{}
	names := map[string]bool{}
	withBody := 0
	for _, m := range Methods() {
		if m.Op == "" {
			t.Fatalf("a method has no operation name: %+v", m)
		}
		if m.Name == "" {
			t.Fatalf("a method has no wire name: %+v", m)
		}
		if strings.ContainsAny(m.Op, "/. ") {
			t.Errorf("the operation name %q is not an identifier — the registry names operations, not paths", m.Op)
		}
		if ops[m.Op] {
			t.Errorf("duplicate operation name %q", m.Op)
		}
		ops[m.Op] = true
		if names[m.Name] {
			t.Errorf("duplicate wire name %q", m.Name)
		}
		names[m.Name] = true
		if len(m.Body) > 0 {
			withBody++
		}
	}
	if withBody == 0 {
		t.Fatal("no method names a params shape — the door has nothing to measure")
	}
}

// TestEveryParamsSchemaIsASchemaTheDoorCanCheck holds the shape the door
// measures a params object against.
//
// Three rules, and each one is a rule the door has:
//   - the schema parses: a schema that does not is refused by the validator
//     and every params object with it would be refused too;
//   - the schema carries no `required` list — the door checks TYPES, and
//     presence is the handler's business;
//   - an undeclared field stays allowed — a body is the stable SUBSET of what
//     the daemon reads.
//
// And every property declares a type, at every level: a property without one
// would accept anything, which is the hole the door exists to close.
func TestEveryParamsSchemaIsASchemaTheDoorCanCheck(t *testing.T) {
	bodies := 0
	for _, m := range Methods() {
		if len(m.Body) == 0 {
			continue
		}
		bodies++
		var doc jsonshape.Node
		if err := json.Unmarshal(m.Body, &doc); err != nil {
			t.Fatalf("the %q method's schema is not readable JSON: %v", m.Name, err)
		}
		if doc.Type != "object" {
			t.Fatalf("the %q method's schema is the %q schema, want object", m.Name, doc.Type)
		}
		var raw struct {
			Required             []string `json:"required"`
			AdditionalProperties any      `json:"additionalProperties"`
		}
		if err := json.Unmarshal(m.Body, &raw); err != nil {
			t.Fatalf("the %q method's schema is not readable JSON: %v", m.Name, err)
		}
		if len(raw.Required) != 0 {
			t.Fatalf("the %q method's schema carries the required list %v — the door checks types, not presence",
				m.Name, raw.Required)
		}
		if raw.AdditionalProperties != true {
			t.Fatalf("the %q method's schema does not allow an undeclared field — a body is the stable subset of what the daemon reads",
				m.Name)
		}
		if len(doc.Properties) == 0 {
			t.Fatalf("the %q method's schema names no field", m.Name)
		}
		checkSchemaNode(t, m.Name, "$", doc)
	}
	if bodies == 0 {
		t.Fatal("no method names a params shape — the door has nothing to measure")
	}
}

// checkSchemaNode holds ONE node of a params schema to the door's rules: it
// declares a type, it asks for no presence, and it allows an undeclared
// field.
func checkSchemaNode(t *testing.T, method, path string, node jsonshape.Node) {
	t.Helper()
	if len(jsonshape.SchemaWords(node.Type)) == 0 {
		t.Fatalf("the %q method's %q property declares no type — the door would accept anything for it",
			method, path)
	}
	if len(node.Properties) > 0 {
		for name, child := range node.Properties {
			checkSchemaNode(t, method, path+"."+name, child)
		}
	}
	if node.Items != nil {
		checkSchemaNode(t, method, path+"[]", *node.Items)
	}
	raw := bytes.TrimSpace(node.AdditionalProperties)
	if len(raw) == 0 {
		return
	}
	if raw[0] == 'f' {
		t.Fatalf("the %q method's %q property refuses an undeclared field — a body is the stable subset of what the daemon reads",
			method, path)
	}
	if raw[0] != '{' {
		return
	}
	var child jsonshape.Node
	if err := json.Unmarshal(raw, &child); err != nil {
		t.Fatalf("the %q method's %q property has an unreadable value shape: %v", method, path, err)
	}
	checkSchemaNode(t, method, path+"{}", child)
}

// TestTheLookupAnswersTheTable: the lookups the door uses answer the table,
// and answer none for a method the contract does not know.
func TestTheLookupAnswersTheTable(t *testing.T) {
	for _, m := range Methods() {
		got, ok := Lookup(m.Name)
		if !ok {
			t.Errorf("the %q method is in the table, but the lookup does not answer it", m.Name)
			continue
		}
		if got.Op != m.Op || got.Name != m.Name {
			t.Errorf("the lookup answers %+v for the %q method, want %+v", got, m.Name, m)
		}
		op, ok := Op(m.Name)
		if !ok || op != m.Op {
			t.Errorf("the operation of the %q method is %q (found=%v), want %q", m.Name, op, ok, m.Op)
		}
		shape, ok := ParamsSchema(m.Name)
		if len(m.Body) == 0 {
			if ok {
				t.Errorf("the %q method reads no params, but the lookup answers a schema", m.Name)
			}
			continue
		}
		if !ok {
			t.Errorf("the %q method names a shape, but the lookup answers none", m.Name)
			continue
		}
		if string(shape) != string(m.Body) {
			t.Errorf("the %q method's shape is %s, the lookup answers %s", m.Name, m.Body, shape)
		}
	}
	if _, ok := Lookup("no/such/method"); ok {
		t.Error("the lookup answers a method the contract does not know")
	}
	if _, ok := ParamsSchema("no/such/method"); ok {
		t.Error("the lookup answers a shape for a method the contract does not know")
	}
	if _, ok := Op("no/such/method"); ok {
		t.Error("the lookup answers an operation for a method the contract does not know")
	}
}

// TestTheMethodsThatReadNoParams: the methods the server reads no params of
// are named, and they name no shape — so the door only ever has to measure
// JSON objects, and a caller can tell "this method takes no params" from
// "this method is not in the contract".
//
// The `initialize` params (the client's protocol version and capabilities)
// are NOT named: the server answers with its own and reads none of them. The
// cancellation notifications carry a session id the server does not read at
// this version either — the per-prompt context does the cancelling.
func TestTheMethodsThatReadNoParams(t *testing.T) {
	for _, name := range []string{MethodInitialize, MethodSessionCancel, MethodCancelNotification} {
		if _, ok := ParamsSchema(name); ok {
			t.Errorf("the %q method reads no params, but the lookup answers a schema", name)
		}
		found := false
		for _, m := range Methods() {
			if m.Name == name {
				found = true
				if len(m.Body) != 0 {
					t.Errorf("the %q method reads no params, but the table names one", name)
				}
			}
		}
		if !found {
			t.Errorf("the %q method is not in the surface's roster", name)
		}
	}
}

// TestTheShapeGuardCatchesAWrongSchema: the guard holds a schema to the
// server's own type, so a schema that mistyped a field — or dropped one — is
// CAUGHT. Without this, the guard would agree with a wrong schema and pass.
func TestTheShapeGuardCatchesAWrongSchema(t *testing.T) {
	type serverParams struct {
		Cwd string `json:"cwd"`
	}
	// A schema that mistypes "cwd" as a number.
	mistyped := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
		`"cwd":{"type":"number"}}}`)
	if err := jsonshape.CheckAgainst(mistyped, reflect.TypeOf(serverParams{})); err == nil {
		t.Error("the guard accepted a schema that mistypes a field")
	}
	// A schema that drops a field the server reads.
	dropped := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{}}`)
	if err := jsonshape.CheckAgainst(dropped, reflect.TypeOf(serverParams{})); err == nil {
		t.Error("the guard accepted a schema that drops a field the server reads")
	}
	// A schema that names a field the server does not read.
	extra := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
		`"cwd":{"type":"string"},"nope":{"type":"string"}}}`)
	if err := jsonshape.CheckAgainst(extra, reflect.TypeOf(serverParams{})); err == nil {
		t.Error("the guard accepted a schema that names a field the server does not read")
	}
	// The schema that agrees: it passes.
	agreed := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
		`"cwd":{"type":"string"}}}`)
	if err := jsonshape.CheckAgainst(agreed, reflect.TypeOf(serverParams{})); err != nil {
		t.Errorf("the guard rejected a schema that agrees with the server: %v", err)
	}
}

// TestTheGuardAnswersNothingForAMethodThatReadsNoParams: a shape cannot be
// checked against a method that has none — so the guard says so rather than
// passing silently.
func TestTheGuardAnswersNothingForAMethodThatReadsNoParams(t *testing.T) {
	type serverParams struct {
		Cwd string `json:"cwd"`
	}
	if err := CheckShape(MethodInitialize, reflect.TypeOf(serverParams{})); err == nil {
		t.Error("the guard passed for a method that reads no params")
	}
	if err := CheckShape("no/such/method", reflect.TypeOf(serverParams{})); err == nil {
		t.Error("the guard passed for a method the contract does not know")
	}
}
