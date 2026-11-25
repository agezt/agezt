// SPDX-License-Identifier: MIT

package agentgwapi

// The contract's own guards.
//
// The gateway's door measures an arriving body against the schema this
// package names, so a schema that is malformed or says nothing about a field
// would either refuse everything or accept anything. The three tests below
// hold the table to that.

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

// TestEveryBodySchemaIsASchemaTheDoorCanCheck holds the shape the door
// measures a body against.
//
// Three rules, and each one is a rule the door has:
//   - the schema parses: a schema that does not is refused by the validator
//     and every body with it would be refused too;
//   - the schema carries no `required` list — the door checks TYPES, and
//     presence is the handler's business;
//   - an undeclared field stays allowed — a body is the stable SUBSET of what
//     the daemon reads.
//
// And every property declares a type: a property without one would accept
// anything, which is the hole the door exists to close.
func TestEveryBodySchemaIsASchemaTheDoorCanCheck(t *testing.T) {
	bodies := 0
	for _, r := range Routes() {
		if len(r.Body) == 0 {
			continue
		}
		bodies++
		var doc struct {
			Type                 string                     `json:"type"`
			AdditionalProperties bool                       `json:"additionalProperties"`
			Required             []string                   `json:"required"`
			Properties           map[string]json.RawMessage `json:"properties"`
		}
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
		if !doc.AdditionalProperties {
			t.Fatalf("the %q route's schema does not allow an undeclared field — a body is the stable subset of what the daemon reads",
				r.ID)
		}
		if len(doc.Properties) == 0 {
			t.Fatalf("the %q route's schema names no field", r.ID)
		}
		for name, raw := range doc.Properties {
			var prop struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(raw, &prop); err != nil {
				t.Fatalf("the %q route's %q property is not readable: %v", r.ID, name, err)
			}
			if prop.Type == "" {
				t.Fatalf("the %q route's %q property declares no type — the door would accept anything for it",
					r.ID, name)
			}
		}
	}
	if bodies == 0 {
		t.Fatal("no route names a request body — the gateway's door has nothing to guard")
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
