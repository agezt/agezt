// SPDX-License-Identifier: MIT

package providers

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/platform/schema"
)

func TestOAuthSpecsDescribeTypedOutputAndPreserveModelAbsence(t *testing.T) {
	operations, err := OAuthOperations(func(context.Context) *OAuth { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range operations {
		spec := operation.Spec()
		raw, err := json.Marshal(reflect.Zero(spec.Output).Interface())
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatalf("%s zero output rejected: %v", spec.Name, err)
		}
		var wire map[string]any
		if err := json.Unmarshal(raw, &wire); err != nil {
			t.Fatal(err)
		}
		if spec.Name == "provider_oauth_start" {
			wire["state"] = 42
		} else {
			wire["connected"] = "wrong"
		}
		invalid, _ := json.Marshal(wire)
		if schema.ValidateJSON(spec.OutputSchema, invalid) == nil {
			t.Fatalf("%s output type erased", spec.Name)
		}
		if err := schema.ValidateJSON(spec.InputSchema, json.RawMessage(`{"ignored":true}`)); err != nil {
			t.Fatal(err)
		}
	}
	for _, models := range [][]string{nil, {}, {"fixture-current"}} {
		raw, err := json.Marshal(OAuthStatusOutput{Models: models})
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]any
		if err := json.Unmarshal(raw, &wire); err != nil {
			t.Fatal(err)
		}
		if models == nil && wire["models"] != nil {
			t.Fatal("unknown model surface changed")
		}
		if models != nil && len(wire["models"].([]any)) != len(models) {
			t.Fatal("authoritative model surface changed")
		}
	}
}
