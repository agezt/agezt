// SPDX-License-Identifier: MIT
package workflow

import (
	"encoding/json"
	"github.com/agezt/agezt/kernel/platform/schema"
	graphs "github.com/agezt/agezt/kernel/workflow"
	"reflect"
)

func schemaNode(t reflect.Type) (map[string]any, error) {
	raw, err := schema.FromType(t, true)
	if err != nil {
		return nil, err
	}
	var node map[string]any
	err = json.Unmarshal(raw, &node)
	return node, err
}
func objectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": true, "properties": properties, "required": required}
}
func graphOutputSchemas() (map[string]json.RawMessage, error) {
	typ := reflect.TypeFor[graphs.Node]()
	fields := make([]reflect.StructField, typ.NumField())
	for i := range fields {
		fields[i] = typ.Field(i)
		if fields[i].Name == "Config" {
			fields[i].Type = reflect.TypeFor[any]()
		}
	}
	node, err := schemaNode(reflect.StructOf(fields))
	if err != nil {
		return nil, err
	}
	full, err := schemaNode(reflect.TypeFor[Record]())
	if err != nil {
		return nil, err
	}
	edges, err := schemaNode(reflect.TypeFor[[]graphs.Edge]())
	if err != nil {
		return nil, err
	}
	properties := full["properties"].(map[string]any)
	properties["nodes"] = map[string]any{"type": []string{"array", "null"}, "items": node}
	properties["edges"] = edges
	required := full["required"].([]any)
	full["required"] = append(required, "nodes")
	boolean := map[string]any{"type": "boolean"}
	text := map[string]any{"type": "string"}
	integer := map[string]any{"type": "integer"}
	template := objectSchema(map[string]any{"name": text, "title": text, "description": text, "category": text, "node_count": integer, "workflow": full}, "name", "title", "description", "category", "node_count", "workflow")
	nodes := map[string]map[string]any{"show": objectSchema(map[string]any{"workflow": full}, "workflow"), "save": objectSchema(map[string]any{"workflow": full, "created": boolean}, "workflow", "created"), "copilot": objectSchema(map[string]any{"workflow": full, "correlation_id": text}, "workflow", "correlation_id"), "templates": objectSchema(map[string]any{"templates": map[string]any{"type": []string{"array", "null"}, "items": template}, "count": integer}, "templates", "count")}
	out := map[string]json.RawMessage{}
	for key, node := range nodes {
		raw, err := json.Marshal(node)
		if err != nil {
			return nil, err
		}
		out[key] = raw
	}
	return out, nil
}
