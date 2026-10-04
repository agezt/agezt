// SPDX-License-Identifier: MIT

package catalog_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	appcatalog "github.com/agezt/agezt/kernel/app/catalog"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

func TestCatalogSpecsDescribeActualTypedWire(t *testing.T) {
	operations, err := appcatalog.Operations(func(context.Context) *appcatalog.Service { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range operations {
		spec := operation.Spec()
		if spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput {
			t.Fatalf("catalog admission=%+v", spec)
		}
		if err := schema.ValidateJSON(spec.InputSchema, json.RawMessage(`{"ignored":"legacy"}`)); err != nil {
			t.Fatalf("ignored args rejected: %v", err)
		}
		if spec.Name == "catalog_sync" && schema.ValidateJSON(spec.InputSchema, json.RawMessage(`{"url":null}`)) == nil {
			t.Fatal("explicit null URL accepted")
		}
		if spec.Name == "catalog_discover" && schema.ValidateJSON(spec.InputSchema, json.RawMessage(`{"endpoint":true}`)) == nil {
			t.Fatal("non-string endpoint accepted")
		}
		zero := reflect.Zero(spec.Output).Interface()
		raw, err := json.Marshal(zero)
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatalf("zero typed output rejected: %v", err)
		}
		wire := catalogWire(t, zero)
		if spec.Name == "catalog_list" {
			wire["providers"] = []any{map[string]any{"id": "x", "name": "X", "family": "test", "api": "", "doc": "", "env": nil, "credentialed": false, "model_count": 1, "models": []any{map[string]any{"id": "m", "name": "M", "family": "test", "tool_call": true, "strict_tool_args": false, "schema_constrained_decoding": false, "grammar_constrained_decoding": false, "reasoning": false, "context": "wrong", "output": 1}}}}
		} else {
			wire["providers_reloaded"] = "wrong"
		}
		invalid, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		if schema.ValidateJSON(spec.OutputSchema, invalid) == nil {
			t.Fatalf("typed output field/schema erased: %s", spec.Name)
		}
	}
}

func TestCatalogTypedZeroPricesRemainPresent(t *testing.T) {
	zeroUSD := float64(0)
	zeroMC := int64(0)
	wire := catalogWire(t, appcatalog.ListOutput{Providers: []appcatalog.ProviderOutput{{Models: []appcatalog.ModelOutput{{ID: "zero", CostInputUSDPerMTok: &zeroUSD, CostOutputUSDPerMTok: &zeroUSD, CostInputMCPerMTok: &zeroMC, CostOutputMCPerMTok: &zeroMC}, {ID: "unknown"}}}}})
	models := catalogRows(t, catalogRows(t, wire["providers"])[0]["models"])
	for _, field := range []string{"cost_input_usd_per_mtok", "cost_output_usd_per_mtok", "cost_input_mc_per_mtok", "cost_output_mc_per_mtok"} {
		if models[0][field] != float64(0) {
			t.Fatalf("known zero price omitted: %s=%v", field, models[0])
		}
		if _, exists := models[1][field]; exists {
			t.Fatalf("unknown price fabricated: %s", field)
		}
	}
}
