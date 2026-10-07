// SPDX-License-Identifier: MIT

package schedule

import (
	"encoding/json"
	"github.com/agezt/agezt/kernel/platform/schema"
	"reflect"
)

// Raw request fields require an explicit schema so presence validation stays in the handler.
var scheduleRequestSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"status":{},"id":{},"intent":{},"model":{},"agent":{},"target":{},"workflow":{},"system_task":{},"tool":{},"payload":{},"enabled":{},"count":{},"once_at_unix":{},"cooldown_sec":{},"window_start":{},"window_end":{},"interval_sec":{},"days":{},"at_minutes":{},"tz":{},"limit":{},"cursor":{},"since_ms":{}}}`)

func scheduleOutputSchemas() (json.RawMessage, json.RawMessage, json.RawMessage, error) {
	typ := reflect.TypeFor[Record]()
	fields := make([]reflect.StructField, typ.NumField())
	for i := range fields {
		fields[i] = typ.Field(i)
		if fields[i].Name == "Payload" {
			fields[i].Type = reflect.TypeFor[any]()
		}
	}
	record, err := schema.FromType(reflect.StructOf(fields), true)
	if err != nil {
		return nil, nil, nil, err
	}
	var node map[string]any
	if err := json.Unmarshal(record, &node); err != nil {
		return nil, nil, nil, err
	}
	list, err := json.Marshal(map[string]any{"type": "object", "additionalProperties": true, "required": []string{"schedules", "count"}, "properties": map[string]any{"schedules": map[string]any{"type": []string{"array", "null"}, "items": node}, "count": map[string]any{"type": "integer"}}})
	if err != nil {
		return nil, nil, nil, err
	}
	properties := node["properties"].(map[string]any)
	properties["updated"] = map[string]any{"type": "boolean"}
	edit, err := json.Marshal(map[string]any{"type": "object", "additionalProperties": true, "required": []string{"updated"}, "properties": properties})
	return record, list, edit, err
}
