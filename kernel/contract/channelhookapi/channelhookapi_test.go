// SPDX-License-Identifier: MIT

package channelhookapi

// The contract's own guards.
//
// The family's door measures an arriving body against the schema this package
// names, so a schema that is malformed or says nothing about a field would
// either refuse everything or accept anything. The tests below hold the table
// to that — and the shape guard to itself: a schema that mistyped a field
// must be CAUGHT, or the guard would agree with a wrong schema and pass.

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

// TestTheMemberTableIsWellFormed: every member has an id and no two claim the
// same id.
//
// A member may take NO JSON body — WeCom reads an XML callback and SMS reads a
// form — and such a member is named in the table precisely so the roster is
// one table; the lookup answers none for it, and the member says so when it
// is built.
func TestTheMemberTableIsWellFormed(t *testing.T) {
	ids := map[string]bool{}
	members := Members()
	if len(members) == 0 {
		t.Fatal("the table names no member — the family's door has nothing to guard")
	}
	withBody := 0
	for _, m := range members {
		if m.ID == "" {
			t.Fatalf("a member has no id: %+v", m)
		}
		if ids[m.ID] {
			t.Fatalf("duplicate member id %q", m.ID)
		}
		ids[m.ID] = true
		if len(m.Body) > 0 {
			withBody++
		}
	}
	if withBody == 0 {
		t.Fatal("no member names a body — the family's door has nothing to measure")
	}
}

// TestTheMembersThatTakeNoJSONBody: the two members that read something other
// than JSON are named, and they name no body — so the door only ever has to
// measure JSON, and a caller can tell "this member has no body" from "this
// member is not in the contract".
func TestTheMembersThatTakeNoJSONBody(t *testing.T) {
	for _, id := range []string{MemberWeCom, MemberSMS} {
		if _, ok := Body(id); ok {
			t.Errorf("the %q member takes no JSON body, but the lookup answers a schema", id)
		}
		found := false
		for _, m := range Members() {
			if m.ID == id {
				found = true
				if len(m.Body) != 0 {
					t.Errorf("the %q member takes no JSON body, but the table names one", id)
				}
			}
		}
		if !found {
			t.Errorf("the %q member is not in the family's roster", id)
		}
	}
}

// TestEveryBodySchemaIsASchemaTheDoorCanCheck holds the shape the door
// measures a body against.
//
// Three rules, and each one is a rule the door has:
//   - the schema parses: a schema that does not is refused by the validator
//     and every body with it would be refused too;
//   - the schema carries no `required` list — the door checks TYPES, and
//     presence is the plugin's business;
//   - an undeclared field stays allowed — a body is the stable SUBSET of what
//     the daemon reads.
//
// And every property declares a type, at every level: a property without one
// would accept anything, which is the hole the door exists to close.
func TestEveryBodySchemaIsASchemaTheDoorCanCheck(t *testing.T) {
	bodies := 0
	for _, m := range Members() {
		if len(m.Body) == 0 {
			continue
		}
		bodies++
		var doc schemaNode
		if err := json.Unmarshal(m.Body, &doc); err != nil {
			t.Fatalf("the %q member's schema is not readable JSON: %v", m.ID, err)
		}
		if doc.Type != "object" {
			t.Fatalf("the %q member's schema is the %q schema, want object", m.ID, doc.Type)
		}
		var raw struct {
			Required             []string `json:"required"`
			AdditionalProperties any      `json:"additionalProperties"`
		}
		if err := json.Unmarshal(m.Body, &raw); err != nil {
			t.Fatalf("the %q member's schema is not readable JSON: %v", m.ID, err)
		}
		if len(raw.Required) != 0 {
			t.Fatalf("the %q member's schema carries the required list %v — the door checks types, not presence",
				m.ID, raw.Required)
		}
		if raw.AdditionalProperties != true {
			t.Fatalf("the %q member's schema does not allow an undeclared field — a body is the stable subset of what the daemon reads",
				m.ID)
		}
		if len(doc.Properties) == 0 {
			t.Fatalf("the %q member's schema names no field", m.ID)
		}
		checkSchemaNode(t, m.ID, "$", doc)
	}
	if bodies == 0 {
		t.Fatal("no member names a body — the family's door has nothing to guard")
	}
}

// TestTheLookupAnswersTheTable: the lookup the door uses answers the schema
// the table states, and answers none for a member the contract does not know.
func TestTheLookupAnswersTheTable(t *testing.T) {
	for _, m := range Members() {
		got, ok := Body(m.ID)
		if len(m.Body) == 0 {
			if ok {
				t.Errorf("the %q member takes no body, but the lookup answers a schema", m.ID)
			}
			continue
		}
		if !ok {
			t.Errorf("the %q member names a body, but the lookup answers none", m.ID)
			continue
		}
		if string(got) != string(m.Body) {
			t.Errorf("the %q member's schema is %s, the lookup answers %s", m.ID, m.Body, got)
		}
	}
	if _, ok := Body("no_such_member"); ok {
		t.Error("the lookup answers a schema for a member the contract does not know")
	}
}

// TestTheShapeGuardCatchesAWrongSchema: the guard holds a schema to the
// daemon's own type, so a schema that mistyped a field — or dropped one — is
// CAUGHT. Without this, the guard would agree with a wrong schema and pass.
func TestTheShapeGuardCatchesAWrongSchema(t *testing.T) {
	type daemonBody struct {
		Text string `json:"text"`
		TSMS int64  `json:"ts_ms"`
	}
	// A schema that mistypes "text" as a number.
	mistyped := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
		`"text":{"type":"number"},"ts_ms":{"type":"number"}}}`)
	if err := checkShapeAgainst(mistyped, reflect.TypeOf(daemonBody{})); err == nil {
		t.Error("the guard accepted a schema that mistypes a field")
	}
	// A schema that drops a field the daemon reads.
	dropped := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
		`"text":{"type":"string"}}}`)
	if err := checkShapeAgainst(dropped, reflect.TypeOf(daemonBody{})); err == nil {
		t.Error("the guard accepted a schema that drops a field the daemon reads")
	}
	// A schema that names a field the daemon does not read.
	extra := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
		`"text":{"type":"string"},"ts_ms":{"type":"number"},"nope":{"type":"string"}}}`)
	if err := checkShapeAgainst(extra, reflect.TypeOf(daemonBody{})); err == nil {
		t.Error("the guard accepted a schema that names a field the daemon does not read")
	}
	// The schema that agrees: it passes.
	agreed := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
		`"text":{"type":"string"},"ts_ms":{"type":"number"}}}`)
	if err := checkShapeAgainst(agreed, reflect.TypeOf(daemonBody{})); err != nil {
		t.Errorf("the guard rejected a schema that agrees with the daemon: %v", err)
	}
}

// TestTheShapeGuardReadsANumberAsANumber: a json.Number is a string
// underneath, but it only ever holds a NUMBER — the vendor sends it as one
// and the decoder refuses a JSON string for it — so the registry's word for
// it is "number".
//
// OneBot's ids are read that way, and a contract that called them strings
// would refuse the numbers the vendor actually sends.
func TestTheShapeGuardReadsANumberAsANumber(t *testing.T) {
	type daemonBody struct {
		UserID json.Number `json:"user_id"`
		Text   string      `json:"text"`
	}
	// The schema that agrees: the id is a number.
	agreed := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
		`"user_id":{"type":"number"},"text":{"type":"string"}}}`)
	if err := checkShapeAgainst(agreed, reflect.TypeOf(daemonBody{})); err != nil {
		t.Errorf("the guard rejected a schema that agrees with the daemon: %v", err)
	}
	// A schema that calls the number a string: the vendor's numbers would be
	// refused at the door.
	mistyped := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
		`"user_id":{"type":"string"},"text":{"type":"string"}}}`)
	if err := checkShapeAgainst(mistyped, reflect.TypeOf(daemonBody{})); err == nil {
		t.Error("the guard accepted a schema that called a json.Number a string")
	}
}

// checkSchemaNode holds ONE node of a body schema to the door's rules: it
// declares a type, it asks for no presence, and it allows an undeclared field.
func checkSchemaNode(t *testing.T, memberID, path string, node schemaNode) {
	t.Helper()
	if len(schemaWords(node.Type)) == 0 {
		t.Fatalf("the %q member's %q property declares no type — the door would accept anything for it",
			memberID, path)
	}
	if len(node.Properties) > 0 {
		for name, child := range node.Properties {
			checkSchemaNode(t, memberID, path+"."+name, child)
		}
	}
	if node.Items != nil {
		checkSchemaNode(t, memberID, path+"[]", *node.Items)
	}
	// A node may say what the VALUES of a map-shaped field look like, and then
	// that shape is held to the same rules. It may not FORBID an undeclared
	// field: a body is the stable subset of what the daemon reads.
	raw := bytes.TrimSpace(node.AdditionalProperties)
	if len(raw) == 0 {
		return
	}
	if raw[0] == 'f' {
		t.Fatalf("the %q member's %q property refuses an undeclared field — a body is the stable subset of what the daemon reads",
			memberID, path)
	}
	if raw[0] != '{' {
		return
	}
	var child schemaNode
	if err := json.Unmarshal(raw, &child); err != nil {
		t.Fatalf("the %q member's %q property has an unreadable value shape: %v", memberID, path, err)
	}
	checkSchemaNode(t, memberID, path+"{}", child)
}

// TestTheShapeGuardHoldsARawField: "any" is the contract's word for a field
// the daemon reads as raw JSON, and the guard holds it to that — in both
// directions.
//
// A raw field Discord sends as a string for one option and as a number for
// another cannot be measured, so the door does not measure it. But the word
// must not become a loophole: a schema that types a raw field ("string") is
// caught, and a schema that says "any" for a field the daemon reads as a
// plain string is caught too.
func TestTheShapeGuardHoldsARawField(t *testing.T) {
	type rawBody struct {
		Text  string          `json:"text"`
		Value json.RawMessage `json:"value"`
	}
	// The schema that agrees: the raw field is "any".
	agreed := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
		`"text":{"type":"string"},"value":{"type":"any"}}}`)
	if err := checkShapeAgainst(agreed, reflect.TypeOf(rawBody{})); err != nil {
		t.Errorf("the guard rejected a schema that agrees with the daemon: %v", err)
	}
	// A schema that types the raw field: the daemon cannot promise a shape.
	mistyped := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
		`"text":{"type":"string"},"value":{"type":"string"}}}`)
	if err := checkShapeAgainst(mistyped, reflect.TypeOf(rawBody{})); err == nil {
		t.Error("the guard accepted a schema that typed a raw field")
	}
	// A schema that calls a plain string "any": a hole in the door.
	loose := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
		`"text":{"type":"any"},"value":{"type":"any"}}}`)
	if err := checkShapeAgainst(loose, reflect.TypeOf(rawBody{})); err == nil {
		t.Error("the guard accepted a schema that called a plain string any")
	}
}

// TestTheShapeGuardHoldsAMapShape: a map-shaped field holds values the daemon
// reads one by one, so the schema must say what a value looks like — and the
// guard must hold it to that.
//
// Discord's resolved attachments are a map: the schema names the shape of one
// attachment, and the guard checks it against the attachment type the daemon
// reads.
func TestTheShapeGuardHoldsAMapShape(t *testing.T) {
	type attachment struct {
		URL string `json:"url"`
	}
	type mapBody struct {
		Attachments map[string]attachment `json:"attachments"`
	}
	agreed := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
		`"attachments":{"type":"object","additionalProperties":{"type":"object","additionalProperties":true,` +
		`"properties":{"url":{"type":"string"}}}}}}`)
	if err := checkShapeAgainst(agreed, reflect.TypeOf(mapBody{})); err != nil {
		t.Errorf("the guard rejected a schema that agrees with the daemon: %v", err)
	}
	// A schema that names no shape for the values: every value unmeasured.
	loose := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
		`"attachments":{"type":"object","additionalProperties":true}}}`)
	if err := checkShapeAgainst(loose, reflect.TypeOf(mapBody{})); err == nil {
		t.Error("the guard accepted a schema that named no shape for a map's values")
	}
	// A schema that mistypes a value's field.
	mistyped := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
		`"attachments":{"type":"object","additionalProperties":{"type":"object","additionalProperties":true,` +
		`"properties":{"url":{"type":"number"}}}}}}`)
	if err := checkShapeAgainst(mistyped, reflect.TypeOf(mapBody{})); err == nil {
		t.Error("the guard accepted a schema that mistyped a map value's field")
	}
}
