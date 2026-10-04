// SPDX-License-Identifier: MIT

package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// FromType derives the supported JSON Schema subset from a handler's Go type.
// Unsupported/cyclic representations fail at registration rather than silently
// advertising a shape that cannot validate the handler's JSON input.
func FromType(t reflect.Type, allowUnknown bool) (json.RawMessage, error) {
	node, err := typeNode(t, allowUnknown, map[reflect.Type]bool{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(node)
}

func typeNode(t reflect.Type, allowUnknown bool, visiting map[reflect.Type]bool) (map[string]any, error) {
	if t == nil {
		return nil, fmt.Errorf("missing type")
	}
	marshaler, unmarshaler := reflect.TypeFor[json.Marshaler](), reflect.TypeFor[json.Unmarshaler]()
	if t.Implements(marshaler) || t.Implements(unmarshaler) || reflect.PointerTo(t).Implements(marshaler) || reflect.PointerTo(t).Implements(unmarshaler) {
		return nil, fmt.Errorf("custom JSON representation needs an explicit schema: %s", t)
	}
	if t.Kind() == reflect.Pointer {
		return nil, fmt.Errorf("nullable pointers are not supported yet: %s", t)
	}
	if visiting[t] {
		return nil, fmt.Errorf("recursive schema type %s", t)
	}
	visiting[t] = true
	defer delete(visiting, t)
	switch t.Kind() {
	case reflect.Interface:
		if t.NumMethod() != 0 {
			return nil, fmt.Errorf("nonempty interface %s", t)
		}
		return map[string]any{}, nil
	case reflect.String:
		return map[string]any{"type": "string"}, nil
	case reflect.Bool:
		return map[string]any{"type": "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}, nil
	case reflect.Slice, reflect.Array:
		if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string"}, nil
		}
		item, err := typeNode(t.Elem(), allowUnknown, visiting)
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "array", "items": item}, nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("map keys must be strings: %s", t)
		}
		if _, err := typeNode(t.Elem(), allowUnknown, visiting); err != nil {
			return nil, err
		}
		return map[string]any{"type": "object", "additionalProperties": true}, nil
	case reflect.Struct:
		properties := map[string]any{}
		var required []string
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			if field.Anonymous {
				return nil, fmt.Errorf("embedded fields need an explicit schema: %s.%s", t, field.Name)
			}
			parts := strings.Split(field.Tag.Get("json"), ",")
			name := parts[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			if len(parts) > 1 {
				for _, option := range parts[1:] {
					if option == "string" {
						return nil, fmt.Errorf("json string option is unsupported: %s.%s", t, field.Name)
					}
				}
			}
			if _, exists := properties[name]; exists {
				return nil, fmt.Errorf("duplicate JSON field %q", name)
			}
			child, err := typeNode(field.Type, allowUnknown, visiting)
			if err != nil {
				return nil, err
			}
			properties[name] = child
			optional := false
			for _, option := range parts[1:] {
				if option == "omitempty" || option == "omitzero" {
					optional = true
				}
			}
			if !optional {
				required = append(required, name)
			}
		}
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": allowUnknown}, nil
	default:
		return nil, fmt.Errorf("unsupported schema type %s", t)
	}
}
