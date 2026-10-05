// SPDX-License-Identifier: MIT

package providers_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/app/providers"
	"github.com/agezt/agezt/kernel/platform/schema"
)

func TestProviderSpecsRetainTypedShapesAndRelevantInputFields(t *testing.T) {
	operations, err := providers.Operations(func(context.Context) *providers.Service { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range operations {
		spec := operation.Spec()
		value := reflect.Zero(spec.Output).Interface()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatalf("%s typed output rejected: %v", spec.Name, err)
		}
		var wire map[string]any
		if err := json.Unmarshal(raw, &wire); err != nil {
			t.Fatal(err)
		}
		if spec.Name == "provider_key_list" {
			wire["keys"] = []any{map[string]any{"label": "fixture", "active": "wrong", "last4": "1234"}}
		} else if spec.Name == "provider_reload" || spec.Name == "provider_connect" {
			wire["providers_reloaded"] = "wrong"
		} else {
			wire["label"] = 42
		}
		invalid, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		if schema.ValidateJSON(spec.OutputSchema, invalid) == nil {
			t.Fatalf("%s lost typed output schema", spec.Name)
		}
		if err := schema.ValidateJSON(spec.InputSchema, json.RawMessage(`{"ignored":true}`)); err != nil {
			t.Fatalf("unknown legacy args rejected: %v", err)
		}
		if spec.Name == "provider_key_list" && schema.ValidateJSON(spec.InputSchema, json.RawMessage(`{"value":42,"active":"ignored"}`)) != nil {
			t.Fatal("list validates unused add fields")
		}
		if spec.Name == "provider_key_add" && schema.ValidateJSON(spec.InputSchema, json.RawMessage(`{"active":"wrong"}`)) == nil {
			t.Fatal("active string accepted by typed add")
		}
	}
}
