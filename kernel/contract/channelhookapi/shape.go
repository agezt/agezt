// SPDX-License-Identifier: MIT

package channelhookapi

// The contract's own guard: the schema a member names against the Go type the
// daemon reads it as.
//
// The check is here — in the contract, the one home of the family's
// vocabulary — because the family's members are many packages, and a guard
// that each of them had to carry would be thirteen copies of the same
// question. A member asks the contract once, when it is built:
//
//	if err := channelhookapi.CheckShape(channelhookapi.MemberWebhook, reflect.TypeOf(inboundEnvelope{})); err != nil {
//	    panic("channels/webhook: the contract and the plugin disagree: " + err.Error())
//	}
//
// so a contract that mistyped a field stops the daemon instead of quietly
// disagreeing with the member that reads it.
//
// The expectation is the DAEMON's own type, not the schema's: a schema that
// mistyped a field would then agree with itself and pass.

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// rawMessageType is the Go type a two-shaped field is read as.
var rawMessageType = reflect.TypeOf(json.RawMessage{})

// Field is one field a member's body names: its wire name and the types the
// contract declares for it.
type Field struct {
	// Name is the wire name ("text").
	Name string
	// Types are the registry's words for the field's type ("string"), or the
	// two shapes a field is genuinely read in ("string", "array").
	Types []string
}

// schemaNode is a node of a JSON Schema, as the door's validator reads it.
type schemaNode struct {
	Type       any                   `json:"type"`
	Properties map[string]schemaNode `json:"properties"`
	Items      *schemaNode           `json:"items"`
}

// node is a schema node with its field names and types already read.
type node struct {
	words    []string
	children map[string]node
	item     *node
}

// Fields returns the fields a member's body names, with the types the
// contract declares for them.
//
// It answers none for a member the contract does not know.
func Fields(memberID string) []Field {
	root, ok := memberNode(memberID)
	if !ok {
		return nil
	}
	out := make([]Field, 0, len(root.children))
	for name, child := range root.children {
		out = append(out, Field{Name: name, Types: child.words})
	}
	sortFields(out)
	return out
}

// sortFields keeps the field list in a stable order, so a guard that walks it
// reports the same thing every run.
func sortFields(fields []Field) {
	for i := 1; i < len(fields); i++ {
		for j := i; j > 0 && fields[j].Name < fields[j-1].Name; j-- {
			fields[j], fields[j-1] = fields[j-1], fields[j]
		}
	}
}

// memberNode reads a member's schema into the tree the guards walk.
func memberNode(memberID string) (node, bool) {
	shape, ok := Body(memberID)
	if !ok {
		return node{}, false
	}
	var doc schemaNode
	if err := json.Unmarshal(shape, &doc); err != nil {
		return node{}, false
	}
	return readNode(doc), true
}

// readNode reads one schema node into the tree the guards walk.
func readNode(doc schemaNode) node {
	out := node{words: schemaWords(doc.Type)}
	if len(doc.Properties) > 0 {
		out.children = make(map[string]node, len(doc.Properties))
		for name, child := range doc.Properties {
			out.children[name] = readNode(child)
		}
	}
	if doc.Items != nil {
		item := readNode(*doc.Items)
		out.item = &item
	}
	return out
}

// CheckShape compares a member's body schema with the Go type the daemon
// reads it as: the same fields, with the same types, in both directions.
//
// A field the daemon reads but the schema does not name is a hole in the
// door; a field the schema names but the daemon does not read is a promise the
// daemon cannot keep. Both are reported, and the report names them.
func CheckShape(memberID string, typ reflect.Type) error {
	shape, ok := Body(memberID)
	if !ok || len(shape) == 0 {
		return fmt.Errorf("the contract names no body for the %q member", memberID)
	}
	if len(Fields(memberID)) == 0 {
		return fmt.Errorf("the contract names no field for the %q member", memberID)
	}
	return checkShapeAgainst(shape, typ)
}

// checkShapeAgainst compares a body schema with the Go type the daemon reads
// it as — the same comparison, for a schema that is not in the table.
func checkShapeAgainst(shape json.RawMessage, typ reflect.Type) error {
	var doc schemaNode
	if err := json.Unmarshal(shape, &doc); err != nil {
		return fmt.Errorf("the body schema is not readable: %w", err)
	}
	var problems []string
	compareNode(&problems, "", typ, readNode(doc))
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(problems, "; "))
}

// compareNode holds one node of the schema to the daemon's own type.
func compareNode(problems *[]string, path string, typ reflect.Type, want node) {
	got := shapeWords(typ)
	if strings.Join(got, "|") != strings.Join(want.words, "|") {
		*problems = append(*problems, fmt.Sprintf("%q is declared as %v, the daemon reads it as %v",
			path, want.words, got))
	}
	typ = derefType(typ)
	if typ == nil || typ == rawMessageType {
		return
	}
	switch typ.Kind() {
	case reflect.Struct:
		for _, f := range directFields(typ) {
			child, ok := want.children[f.Name]
			if !ok {
				*problems = append(*problems, fmt.Sprintf("%q is read by the daemon but the schema does not name it",
					path+f.Name))
				continue
			}
			compareNode(problems, path+f.Name, f.Type, child)
		}
		for name := range want.children {
			if !hasField(typ, name) {
				*problems = append(*problems, fmt.Sprintf("%q is named by the schema but the daemon does not read it",
					name))
			}
		}
	case reflect.Slice, reflect.Array:
		elem := typ.Elem()
		if want.item == nil {
			*problems = append(*problems, fmt.Sprintf("%q is a list in the daemon but the schema names no item type",
				path))
			return
		}
		if elem.Kind() == reflect.Struct {
			compareNode(problems, path+"[]", elem, *want.item)
			return
		}
		if got, want := shapeWords(elem), want.item.words; strings.Join(got, "|") != strings.Join(want, "|") {
			*problems = append(*problems, fmt.Sprintf("the items of %q are declared as %v, the daemon reads them as %v",
				path, want, got))
		}
	}
}

// hookField is one wire field of a struct.
type hookField struct {
	Name string
	Type reflect.Type
}

// directFields lists a struct's OWN wire fields.
func directFields(typ reflect.Type) []hookField {
	typ = derefType(typ)
	var out []hookField
	if typ == nil || typ == rawMessageType || typ.Kind() != reflect.Struct {
		return out
	}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		out = append(out, hookField{Name: name, Type: f.Type})
	}
	return out
}

// hasField reports whether the daemon reads a field under that name.
func hasField(typ reflect.Type, name string) bool {
	for _, f := range directFields(typ) {
		if f.Name == name {
			return true
		}
	}
	return false
}

// shapeWords are the registry's words for a Go type — the type the daemon
// reads a field as.
//
// A json.RawMessage is the one field a body may read in two shapes: the text
// itself, or an array of typed parts. It is the only union, and the registry's
// own words name it.
func shapeWords(typ reflect.Type) []string {
	if typ == rawMessageType {
		return []string{"string", "array"}
	}
	switch derefType(typ).Kind() {
	case reflect.String:
		return []string{"string"}
	case reflect.Bool:
		return []string{"boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return []string{"number"}
	case reflect.Slice, reflect.Array:
		return []string{"array"}
	case reflect.Map:
		return []string{"object"}
	case reflect.Struct:
		return []string{"object"}
	}
	return nil
}

// schemaWords are the type words a schema node declares.
func schemaWords(raw any) []string {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		var out []string
		for _, item := range v {
			s, ok := item.(string)
			if !ok || s == "" {
				return nil
			}
			out = append(out, s)
		}
		return out
	}
	return nil
}

// derefType follows the pointers a handler's struct uses to say "this field
// may be absent".
func derefType(typ reflect.Type) reflect.Type {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}
