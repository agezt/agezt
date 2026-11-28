// SPDX-License-Identifier: MIT

package schema

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode"
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

// anyJSONValue is the marker a request field carries when it holds ANY JSON
// value.
//
// The contract's own row types implement it (a method is enough — the marker
// is structural, so the schema package needs no import from the contracts).
// Such a field derives the union of the JSON types, which says "any" out
// loud: an empty property schema would say nothing, and the request
// contract's own ratchet refuses a property that names no type.
type anyJSONValue interface{ AnyJSONValue() }

// anyJSONValueTypes is the union a field carrying any JSON value derives.
var anyJSONValueTypes = []string{"object", "array", "string", "number", "boolean", "null"}

// booleanOrString is the marker a request field carries when it holds a
// boolean OR the string a query string carries for one.
//
// A browser's query says "true", not true, so the flag's row says both forms
// are allowed — and the union it derives says so out loud.
type booleanOrString interface{ BooleanOrString() }

// booleanOrStringTypes is the union a flag field derives.
var booleanOrStringTypes = []string{"boolean", "string"}

// jsonObject is the marker a request field carries when it holds a JSON
// object.
//
// The object is named, not "any value": a missing object and a null one are
// different answers, and the schema says so.
type jsonObject interface{ JSONObject() }

// jsonNumber is the marker a request field carries when it holds a number.
//
// A number that must tell "not asked" from "zero" cannot be a plain float, so
// it rides in a wrapper — and the wrapper still derives the number itself.
type jsonNumber interface{ JSONNumber() }

func typeNode(t reflect.Type, allowUnknown bool, visiting map[reflect.Type]bool) (map[string]any, error) {
	if t == nil {
		return nil, fmt.Errorf("missing type")
	}
	if t.Implements(reflect.TypeFor[anyJSONValue]()) {
		return map[string]any{"type": anyJSONValueTypes}, nil
	}
	if t.Implements(reflect.TypeFor[booleanOrString]()) {
		return map[string]any{"type": booleanOrStringTypes}, nil
	}
	if t.Implements(reflect.TypeFor[jsonObject]()) {
		return map[string]any{"type": "object"}, nil
	}
	if t.Implements(reflect.TypeFor[jsonNumber]()) {
		return map[string]any{"type": "number"}, nil
	}
	marshaler, unmarshaler := reflect.TypeFor[json.Marshaler](), reflect.TypeFor[json.Unmarshaler]()
	if t.Implements(marshaler) || t.Implements(unmarshaler) || reflect.PointerTo(t).Implements(marshaler) || reflect.PointerTo(t).Implements(unmarshaler) {
		return nil, fmt.Errorf("custom JSON representation needs an explicit schema: %s", t)
	}
	textMarshaler, textUnmarshaler := reflect.TypeFor[encoding.TextMarshaler](), reflect.TypeFor[encoding.TextUnmarshaler]()
	if t.Implements(textMarshaler) || reflect.PointerTo(t).Implements(textMarshaler) {
		return nil, fmt.Errorf("custom text encoding needs an explicit schema: %s", t)
	}
	if t.Implements(textUnmarshaler) || reflect.PointerTo(t).Implements(textUnmarshaler) {
		return nil, fmt.Errorf("custom text decoding needs an explicit schema: %s", t)
	}
	if t.Kind() == reflect.Pointer {
		node, err := typeNode(t.Elem(), allowUnknown, visiting)
		if err != nil {
			return nil, err
		}
		return nullableNode(node), nil
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
			return nullableNode(map[string]any{"type": "string"}), nil
		}
		item, err := typeNode(t.Elem(), allowUnknown, visiting)
		if err != nil {
			return nil, err
		}
		node := map[string]any{"type": "array", "items": item}
		if t.Kind() == reflect.Slice {
			node = nullableNode(node)
		}
		return node, nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("map keys must be strings: %s", t)
		}
		child, err := typeNode(t.Elem(), allowUnknown, visiting)
		if err != nil {
			return nil, err
		}
		return nullableNode(map[string]any{"type": "object", "additionalProperties": child}), nil
	case reflect.Struct:
		fields, err := structJSONFields(t)
		if err != nil {
			return nil, err
		}
		properties := map[string]any{}
		var required []string
		for _, field := range fields {
			if field.quoted {
				return nil, fmt.Errorf("json string option is unsupported: %s.%s", t, field.name)
			}
			child, err := typeNode(field.typ, allowUnknown, visiting)
			if err != nil {
				return nil, err
			}
			properties[field.name] = child
			if !field.optional {
				required = append(required, field.name)
			}
		}
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": allowUnknown}, nil

	default:
		return nil, fmt.Errorf("unsupported schema type %s", t)
	}
}

// nullableNode preserves validation of non-null values and adds only null to the
// declared type union. Empty interface schemas already admit every JSON value.
func nullableNode(node map[string]any) map[string]any {
	switch typ := node["type"].(type) {
	case string:
		if typ != "null" {
			node["type"] = []string{typ, "null"}
		}
	case []string:
		for _, value := range typ {
			if value == "null" {
				return node
			}
		}
		node["type"] = append(typ, "null")
	}
	return node
}

// jsonField describes a surviving wire field after anonymous-field promotion.
// A nil anonymous pointer can omit every promoted field, independently of its tags.
type jsonField struct {
	name                     string
	typ                      reflect.Type
	depth                    int
	tagged, optional, quoted bool
}

func structJSONFields(root reflect.Type) ([]jsonField, error) {
	candidates := map[string][]jsonField{}
	path := map[reflect.Type]bool{}
	var collect func(reflect.Type, int, bool) error
	collect = func(t reflect.Type, depth int, optional bool) error {
		if path[t] {
			return fmt.Errorf("recursive embedded schema type %s", t)
		}
		path[t] = true
		defer delete(path, t)
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			base := field.Type
			pointer := base.Kind() == reflect.Pointer
			if pointer {
				base = base.Elem()
			}
			if !field.IsExported() && (!field.Anonymous || base.Kind() != reflect.Struct) {
				continue
			}
			tag := field.Tag.Get("json")
			if tag == "-" {
				continue
			}
			parts := strings.Split(tag, ",")
			name := parts[0]
			if !validJSONFieldName(name) {
				name = ""
			}
			if field.Anonymous && name == "" && base.Kind() == reflect.Struct {
				// encoding/json cannot allocate an unexported embedded pointer
				// during decode. Keep that representation explicit at binding.
				if pointer && !field.IsExported() {
					return fmt.Errorf("unexported embedded pointer needs an explicit schema: %s.%s", t, field.Name)
				}
				if err := collect(base, depth+1, optional || pointer); err != nil {
					return err
				}
				continue
			}
			tagged := name != ""
			if name == "" {
				name = field.Name
			}
			candidate := jsonField{name: name, typ: field.Type, depth: depth, tagged: tagged, optional: optional}
			for _, option := range parts[1:] {
				if option == "omitempty" || option == "omitzero" {
					candidate.optional = true
				}
				if option == "string" {
					candidate.quoted = true
				}
			}
			candidates[name] = append(candidates[name], candidate)
		}
		return nil
	}
	if err := collect(root, 0, false); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(candidates))
	for name := range candidates {
		names = append(names, name)
	}
	sort.Strings(names)
	fields := make([]jsonField, 0, len(names))
	for _, name := range names {
		group := candidates[name]
		depth := group[0].depth
		for _, candidate := range group {
			if candidate.depth < depth {
				depth = candidate.depth
			}
		}
		tagged := false
		for _, candidate := range group {
			if candidate.depth == depth && candidate.tagged {
				tagged = true
			}
		}
		count := 0
		var selected jsonField
		for _, candidate := range group {
			if candidate.depth == depth && candidate.tagged == tagged {
				selected = candidate
				count++
			}
		}
		// Same-depth equal-priority conflicts disappear from encoding/json.
		if count == 1 {
			fields = append(fields, selected)
		}
	}
	return fields, nil
}

func validJSONFieldName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", r) || unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		return false
	}
	return true
}
