// SPDX-License-Identifier: MIT

package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ValidateToolInput validates a tool call against the tool's advertised JSON
// Schema before the input reaches policy classification or the tool handler.
// It intentionally implements the small, dependency-free subset Agezt uses in
// first-party tool schemas: type, properties, required, additionalProperties,
// enum, items, and nested schemas.
func ValidateToolInput(def ToolDef, input json.RawMessage) error {
	if !json.Valid(input) {
		return fmt.Errorf("input is not valid JSON")
	}
	schema := bytes.TrimSpace(def.InputSchema)
	if len(schema) == 0 {
		return nil
	}
	var root schemaNode
	if err := decodeJSON(schema, &root); err != nil {
		return fmt.Errorf("tool %q has invalid input schema: %w", def.Name, err)
	}
	var value any
	if err := decodeJSON(input, &value); err != nil {
		return fmt.Errorf("input is not valid JSON: %w", err)
	}
	if err := validateNode(root, value, "$"); err != nil {
		return err
	}
	return nil
}

// LintToolSchema checks that a tool's advertised schema is parseable by the
// same validator the runtime uses at invocation time. Empty schemas remain
// valid for compatibility with test and legacy tools.
func LintToolSchema(def ToolDef) error {
	schema := bytes.TrimSpace(def.InputSchema)
	if len(schema) == 0 {
		return nil
	}
	var root schemaNode
	if err := decodeJSON(schema, &root); err != nil {
		return fmt.Errorf("tool %q has invalid input schema: %w", def.Name, err)
	}
	if err := lintNode(root, "$"); err != nil {
		return fmt.Errorf("tool %q has invalid input schema: %w", def.Name, err)
	}
	return nil
}

type schemaNode struct {
	Type                 any                   `json:"type,omitempty"`
	Properties           map[string]schemaNode `json:"properties,omitempty"`
	Required             []string              `json:"required,omitempty"`
	AdditionalProperties any                   `json:"additionalProperties,omitempty"`
	Items                *schemaNode           `json:"items,omitempty"`
	Enum                 []any                 `json:"enum,omitempty"`
}

func decodeJSON(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(dst)
}

func validateNode(n schemaNode, v any, path string) error {
	if len(n.Enum) > 0 && !enumContains(n.Enum, v) {
		return fmt.Errorf("%s must be one of the declared enum values", path)
	}
	types := schemaTypes(n.Type)
	if len(types) > 0 && !matchesAnyType(types, v) {
		return fmt.Errorf("%s has wrong type: got %s, want %s", path, valueType(v), strings.Join(types, "|"))
	}
	if len(types) == 0 && len(n.Properties) > 0 {
		if _, ok := v.(map[string]any); !ok {
			return fmt.Errorf("%s has wrong type: got %s, want object", path, valueType(v))
		}
	}

	obj, isObj := v.(map[string]any)
	if isObj {
		for _, req := range n.Required {
			if _, ok := obj[req]; !ok {
				return fmt.Errorf("%s.%s is required", path, req)
			}
		}
		for key, val := range obj {
			if child, ok := n.Properties[key]; ok {
				if err := validateNode(child, val, path+"."+key); err != nil {
					return err
				}
				continue
			}
			if err := validateAdditional(n, key, val, path); err != nil {
				return err
			}
		}
	}

	if arr, ok := v.([]any); ok && n.Items != nil {
		for i, val := range arr {
			if err := validateNode(*n.Items, val, path+"["+strconv.Itoa(i)+"]"); err != nil {
				return err
			}
		}
	}
	return nil
}

func lintNode(n schemaNode, path string) error {
	for _, typ := range schemaTypes(n.Type) {
		switch typ {
		case "object", "array", "string", "boolean", "number", "integer", "null":
		default:
			return fmt.Errorf("%s.type %q is not supported", path, typ)
		}
	}
	switch ap := n.AdditionalProperties.(type) {
	case nil, bool:
	case map[string]any:
		var child schemaNode
		raw, err := json.Marshal(ap)
		if err != nil {
			return fmt.Errorf("%s.additionalProperties is invalid: %w", path, err)
		}
		if err := decodeJSON(raw, &child); err != nil {
			return fmt.Errorf("%s.additionalProperties is invalid: %w", path, err)
		}
		if err := lintNode(child, path+".additionalProperties"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%s.additionalProperties must be boolean or object", path)
	}
	for name, child := range n.Properties {
		if err := lintNode(child, path+".properties."+name); err != nil {
			return err
		}
	}
	if n.Items != nil {
		if err := lintNode(*n.Items, path+".items"); err != nil {
			return err
		}
	}
	return nil
}

func validateAdditional(n schemaNode, key string, val any, path string) error {
	switch ap := n.AdditionalProperties.(type) {
	case nil:
		if len(n.Properties) > 0 {
			return fmt.Errorf("%s.%s is not allowed by schema", path, key)
		}
		return nil
	case bool:
		if !ap {
			return fmt.Errorf("%s.%s is not allowed by schema", path, key)
		}
		return nil
	case map[string]any:
		var child schemaNode
		raw, err := json.Marshal(ap)
		if err != nil {
			return fmt.Errorf("%s.%s additionalProperties schema is invalid: %w", path, key, err)
		}
		if err := decodeJSON(raw, &child); err != nil {
			return fmt.Errorf("%s.%s additionalProperties schema is invalid: %w", path, key, err)
		}
		return validateNode(child, val, path+"."+key)
	default:
		return fmt.Errorf("%s.%s additionalProperties schema is invalid", path, key)
	}
}

func schemaTypes(raw any) []string {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func matchesAnyType(types []string, v any) bool {
	for _, typ := range types {
		if matchesType(typ, v) {
			return true
		}
	}
	return false
}

func matchesType(typ string, v any) bool {
	switch typ {
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "number":
		switch v.(type) {
		case json.Number, float64, int, int64:
			return true
		default:
			return false
		}
	case "integer":
		return isInteger(v)
	case "null":
		return v == nil
	default:
		return true // Unknown JSON Schema types are ignored for forward compatibility.
	}
}

func isInteger(v any) bool {
	switch n := v.(type) {
	case json.Number:
		if _, err := n.Int64(); err == nil {
			return true
		}
		f, err := n.Float64()
		return err == nil && math.Trunc(f) == f
	case float64:
		return math.Trunc(n) == n
	case int, int64:
		return true
	default:
		return false
	}
}

func valueType(v any) string {
	switch v.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case nil:
		return "null"
	case json.Number, float64, int, int64:
		return "number"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func enumContains(enum []any, v any) bool {
	for _, item := range enum {
		if jsonEqual(item, v) {
			return true
		}
	}
	return false
}

func jsonEqual(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(ab, bb)
}
