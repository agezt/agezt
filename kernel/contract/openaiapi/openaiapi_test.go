// SPDX-License-Identifier: MIT

package openaiapi

// The contract's own guards.
//
// The surface's door measures an arriving body against the schema this package
// names, so a schema that is malformed or says nothing about a field would
// either refuse everything or accept anything. The three tests below hold the
// table to that.

import (
	"encoding/json"
	"testing"
)

// TestTheRouteTableIsWellFormed: every route has an id, a method and a path,
// and no two routes claim the same id or the same method+path.
func TestTheRouteTableIsWellFormed(t *testing.T) {
	ids := map[string]bool{}
	paths := map[string]bool{}
	for _, r := range Routes() {
		if r.ID == "" {
			t.Fatalf("a route has no id: %+v", r)
		}
		if ids[r.ID] {
			t.Fatalf("duplicate route id %q", r.ID)
		}
		ids[r.ID] = true
		if r.Method == "" || r.Path == "" {
			t.Fatalf("the %q route has no method or no path", r.ID)
		}
		key := r.Method + " " + r.Path
		if paths[key] {
			t.Fatalf("duplicate route %q", key)
		}
		paths[key] = true
	}
}

// TestAnUploadRouteHasNoJSONBody: a route that takes an upload — the audio
// transcription — has no JSON body, so the contract says so and the lookup
// the door uses answers none for it.
//
// The flag is the difference between "this route takes no body" and "this
// route takes a body the door does not measure", and a body the door does not
// measure must never be measured as if it were JSON.
func TestAnUploadRouteHasNoJSONBody(t *testing.T) {
	uploads := 0
	for _, r := range Routes() {
		if !r.Multipart {
			continue
		}
		uploads++
		if len(r.Body) > 0 {
			t.Errorf("the %q route takes an upload, so it has no JSON body", r.ID)
		}
		if _, ok := RequestSchema(r.ID); ok {
			t.Errorf("the %q route takes an upload, but the lookup answers a JSON schema", r.ID)
		}
	}
	if uploads == 0 {
		t.Fatal("no route takes an upload — the contract cannot say what an upload is")
	}
}

// TestEveryBodySchemaIsASchemaTheDoorCanCheck holds the shape the door
// measures a body against.
//
// Three rules, and each one is a rule the door has:
//   - the schema parses: a schema that does not is refused by the validator
//     and every body with it would be refused too;
//   - the schema carries no `required` list — the door checks TYPES, and
//     presence is the handler's business;
//   - an undeclared field stays allowed — a body is the stable SUBSET of what
//     the daemon reads, and this surface is OpenAI's own wire protocol.
//
// And every property declares a type, at every level: a property without one
// would accept anything, which is the hole the door exists to close.
func TestEveryBodySchemaIsASchemaTheDoorCanCheck(t *testing.T) {
	bodies := 0
	for _, r := range Routes() {
		if len(r.Body) == 0 {
			continue
		}
		bodies++
		var doc schemaDoc
		if err := json.Unmarshal(r.Body, &doc); err != nil {
			t.Fatalf("the %q route's schema is not readable JSON: %v", r.ID, err)
		}
		if doc.Type != "object" {
			t.Fatalf("the %q route's schema is the %q schema, want object", r.ID, doc.Type)
		}
		if len(doc.Required) != 0 {
			t.Fatalf("the %q route's schema carries the required list %v — the door checks types, not presence",
				r.ID, doc.Required)
		}
		if doc.AdditionalProperties != true {
			t.Fatalf("the %q route's schema does not allow an undeclared field — a body is the stable subset of what the daemon reads",
				r.ID)
		}
		if len(doc.Properties) == 0 {
			t.Fatalf("the %q route's schema names no field", r.ID)
		}
		checkSchemaNode(t, r.ID, "$", doc)
	}
	if bodies == 0 {
		t.Fatal("no route names a request body — the surface's door has nothing to guard")
	}
}

// TestTheSchemaLookupAnswersTheTable: the lookup the door uses answers the
// schema the table states, and answers none for a route that takes no body.
func TestTheSchemaLookupAnswersTheTable(t *testing.T) {
	for _, r := range Routes() {
		got, ok := RequestSchema(r.ID)
		if len(r.Body) == 0 {
			if ok {
				t.Errorf("the %q route takes no body, but the lookup answers a schema", r.ID)
			}
			continue
		}
		if !ok {
			t.Errorf("the %q route names a body, but the lookup answers none", r.ID)
			continue
		}
		if string(got) != string(r.Body) {
			t.Errorf("the %q route's schema is %s, the lookup answers %s", r.ID, r.Body, got)
		}
	}
	if _, ok := RequestSchema("no_such_route"); ok {
		t.Error("the lookup answers a schema for a route the contract does not know")
	}
}

// schemaDoc is a node of a JSON Schema, as the door's validator reads it.
type schemaDoc struct {
	Type                 any                        `json:"type"`
	Properties           map[string]json.RawMessage `json:"properties"`
	Items                json.RawMessage            `json:"items"`
	Required             []string                   `json:"required"`
	AdditionalProperties any                        `json:"additionalProperties"`
}

// checkSchemaNode holds ONE node of a body schema to the door's rules: it
// declares a type, it asks for no presence, and it allows an undeclared field.
func checkSchemaNode(t *testing.T, routeID, path string, node schemaDoc) {
	t.Helper()
	if !schemaDeclaresAType(node.Type) {
		t.Fatalf("the %q route's %q property declares no type — the door would accept anything for it",
			routeID, path)
	}
	if len(node.Required) != 0 {
		t.Fatalf("the %q route's %q property carries the required list %v — the door checks types, not presence",
			routeID, path, node.Required)
	}
	if len(node.Properties) > 0 && node.AdditionalProperties != true {
		t.Fatalf("the %q route's %q property does not allow an undeclared field — a body is the stable subset of what the daemon reads",
			routeID, path)
	}
	for name, raw := range node.Properties {
		var child schemaDoc
		if err := json.Unmarshal(raw, &child); err != nil {
			t.Fatalf("the %q route's %q property is not readable: %v", routeID, path+"."+name, err)
		}
		checkSchemaNode(t, routeID, path+"."+name, child)
	}
	if len(node.Items) > 0 {
		var item schemaDoc
		if err := json.Unmarshal(node.Items, &item); err != nil {
			t.Fatalf("the %q route's %q items are not readable: %v", routeID, path, err)
		}
		checkSchemaNode(t, routeID, path+"[]", item)
	}
}

// schemaDeclaresAType reports whether a schema node names a type — one word, or
// a list of the two shapes a field is genuinely read in.
func schemaDeclaresAType(raw any) bool {
	switch v := raw.(type) {
	case string:
		return v != ""
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if !ok || s == "" {
				return false
			}
		}
		return len(v) > 0
	}
	return false
}
