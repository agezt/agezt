// SPDX-License-Identifier: MIT

// Package jsonshape is the shared vocabulary of "a JSON body has a shape".
//
// Two contracts now name the shape of a body the daemon reads: the channel
// webhook family (kernel/contract/channelhookapi) and the ACP server surface
// (kernel/contract/acpserverapi). Both need the same three questions
// answered the same way:
//
//  1. What fields does this schema name, and what types does it declare for
//     them?
//  2. Does this schema AGREE with the Go type the daemon reads the body as?
//     A field the daemon reads but the schema does not name is a hole in the
//     door; a field the schema names but the daemon does not read is a promise
//     the daemon cannot keep.
//  3. Which word does the registry use for this Go type? ("string",
//     "number", "boolean", "object", "array", and — for a field read as raw
//     JSON — "any".)
//
// The answers live HERE, once, because a guard that each contract had to
// carry would be two copies of the same question — and two copies drift.
//
// The package is pure: the standard library only (archcheck's principle P4),
// like every other contract package.
package jsonshape

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// rawMessageType is the Go type a two-shaped field is read as.
var rawMessageType = reflect.TypeOf(json.RawMessage{})

// numberType is the Go type a numeric id is read as. A json.Number is a
// string underneath, but it only ever HOLDS a number — the vendor sends it as
// one, and the decoder refuses a JSON string for it — so the registry's word
// for it is "number", not "string".
var numberType = reflect.TypeOf(json.Number(""))

// Field is one field a body names: its wire name and the types the contract
// declares for it.
type Field struct {
	// Name is the wire name ("text").
	Name string
	// Types are the registry's words for the field's type ("string"), or the
	// two shapes a field is genuinely read in ("string", "array").
	Types []string
}

// Node is a node of a JSON Schema, as the door's validator reads it.
type Node struct {
	Type                 any             `json:"type"`
	Properties           map[string]Node `json:"properties"`
	Items                *Node           `json:"items"`
	AdditionalProperties json.RawMessage `json:"additionalProperties"`
}

// node is a schema node with its field names and types already read.
type node struct {
	words    []string
	children map[string]node
	item     *node
	// additional is the schema the node gives the VALUES of a map-shaped
	// field; it is nil when the node names no shape for them.
	additional *node
}

// Fields returns the fields a schema names, with the types it declares for
// them.
//
// It answers none for a schema that cannot be read.
func Fields(shape json.RawMessage) []Field {
	root, ok := readShape(shape)
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

// CheckAgainst compares a body schema with the Go type the daemon reads it
// as: the same fields, with the same types, in both directions.
//
// A field the daemon reads but the schema does not name is a hole in the
// door; a field the schema names but the daemon does not read is a promise the
// daemon cannot keep. Both are reported, and the report names them.
func CheckAgainst(shape json.RawMessage, typ reflect.Type) error {
	var doc Node
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

// readShape reads a schema into the tree the guards walk.
func readShape(shape json.RawMessage) (node, bool) {
	var doc Node
	if err := json.Unmarshal(shape, &doc); err != nil {
		return node{}, false
	}
	return readNode(doc), true
}

// readNode reads one schema node into the tree the guards walk.
func readNode(doc Node) node {
	out := node{words: SchemaWords(doc.Type)}
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
	out.additional = readAdditional(doc.AdditionalProperties)
	return out
}

// readAdditional reads a node's additionalProperties into the schema it gives
// the values of a map-shaped field.
//
// The schema may say "true" (any value is allowed) or "false" (no value is
// allowed) — neither names a shape, so neither is read here.
func readAdditional(raw json.RawMessage) *node {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return nil
	}
	var doc Node
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	child := readNode(doc)
	return &child
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
					childPath(path, f.Name)))
				continue
			}
			compareNode(problems, childPath(path, f.Name), f.Type, child)
		}
		for name := range want.children {
			if !hasField(typ, name) {
				*problems = append(*problems, fmt.Sprintf("%q is named by the schema but the daemon does not read it",
					childPath(path, name)))
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
	case reflect.Map:
		// A map-shaped field holds values the daemon reads one by one, so the
		// schema must say what a value looks like: the shape of Discord's
		// attachments, for one. A schema that says "any value goes" leaves
		// every value unmeasured, which is the hole the door exists to close.
		if want.additional == nil {
			*problems = append(*problems, fmt.Sprintf(
				"%q is a map in the daemon but the schema names no shape for its values", path))
			return
		}
		compareNode(problems, path+"{}", typ.Elem(), *want.additional)
	}
}

// childPath is the path of a field inside its parent — the way the door's
// validator writes it ("$.event.text", "$.data.options[0]").
func childPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
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
// A json.RawMessage is the one field a body may read as RAW: the daemon cannot
// say what shape it has (Discord sends a string for a text option and a number
// for a number one), so the contract names it "any" and the door does not
// measure it. The guard holds the two together in both directions: "any" fits
// a raw field and nothing else.
func shapeWords(typ reflect.Type) []string {
	if typ == rawMessageType {
		return []string{"any"}
	}
	if typ == numberType {
		return []string{"number"}
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

// SchemaWords are the type words a schema node declares.
func SchemaWords(raw any) []string {
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
