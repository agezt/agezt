// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// The console's own answers are Go STRUCTS, and the browser's catalogue is
// rendered from SCHEMAS. This file is the step between the two: a row becomes
// a schema, and the schema becomes the browser's type.
//
// The rows live in the contracts (`kernel/contract/consoleapi`,
// `kernel/contract/rollbackapi`) and the native routes name them
// (`opapi.NativeRoute.Answer`). So the field names are written once — in the
// contract — and the browser's type is DERIVED from them. A browser view that
// spells a daemon field by hand is a second contract; this renderer is what
// makes the second one unnecessary.
//
// The rules the registry's schemas already teach:
//
//   - a member WITHOUT `omitempty` is required; one WITH it may be absent.
//   - a POINTER says *absent*: a required pointer may be null, and the browser
//     folds a null into an absent member; an optional pointer is simply
//     absent.
//   - a free-form value (an `any`, a raw JSON body) says nothing: the schema
//     is empty and the browser reads it as any.
func schemaOf(value any) (json.RawMessage, error) {
	return schemaOfType(reflect.TypeOf(value), false)
}

// schemaOfType renders the schema of one Go type.
//
// nullable says the value may be null: a required pointer member may be, an
// optional one may not (it is absent instead).
func schemaOfType(t reflect.Type, nullable bool) (json.RawMessage, error) {
	if t == nil {
		return json.RawMessage(`{}`), nil
	}
	if t == reflect.TypeOf(json.RawMessage{}) {
		// A raw JSON body is whatever the daemon marshalled: the contract may
		// not name it, so the schema says nothing.
		return json.RawMessage(`{}`), nil
	}
	switch t.Kind() {
	case reflect.Pointer:
		return schemaOfType(t.Elem(), true)
	case reflect.Interface:
		return json.RawMessage(`{}`), nil
	case reflect.String:
		return typedSchema("string", nullable)
	case reflect.Bool:
		return typedSchema("boolean", nullable)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return typedSchema("integer", nullable)
	case reflect.Float32, reflect.Float64:
		return typedSchema("number", nullable)
	case reflect.Slice, reflect.Array:
		items, err := schemaOfType(t.Elem(), false)
		if err != nil {
			return nil, err
		}
		return composeSchema(map[string]any{
			"type":  "array",
			"items": items,
		}, nullable)
	case reflect.Map:
		values, err := schemaOfType(t.Elem(), false)
		if err != nil {
			return nil, err
		}
		return composeSchema(map[string]any{
			"type":                 "object",
			"additionalProperties": values,
		}, nullable)
	case reflect.Struct:
		properties := map[string]json.RawMessage{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "" {
				name = field.Name
			}
			if name == "-" {
				continue
			}
			optional := strings.Contains(options, "omitempty")
			// A plain value is never null; only a POINTER may be. The
			// pointer's own branch adds the null arm, and the renderer folds it
			// away for a member the daemon may omit.
			member, err := schemaOfType(field.Type, false)
			if err != nil {
				return nil, err
			}
			properties[name] = member
			if !optional {
				required = append(required, name)
			}
		}
		return composeSchema(map[string]any{
			"type":       "object",
			"properties": properties,
			"required":   required,
		}, nullable)
	default:
		return nil, fmt.Errorf("the type %s has no schema", t)
	}
}

// typedSchema renders the schema of a scalar type.
func typedSchema(name string, nullable bool) (json.RawMessage, error) {
	return composeSchema(map[string]any{"type": name}, nullable)
}

// composeSchema renders a schema node, adding the null arm when the value may
// be null.
func composeSchema(node map[string]any, nullable bool) (json.RawMessage, error) {
	if nullable {
		node["type"] = []string{node["type"].(string), "null"}
	}
	raw, err := json.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("render the schema: %w", err)
	}
	return raw, nil
}
