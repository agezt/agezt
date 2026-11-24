// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

// requestPropertyTypes names the JSON types a request property may declare.
var requestPropertyTypes = map[string]bool{
	"string": true, "number": true, "integer": true, "boolean": true,
	"object": true, "array": true, "null": true,
}

// TestRequestSchemasNameEveryProperty is the guard for the request side of
// "the dictionary gets one home" (W5.8): every property of every console
// request body declares the type the handler reads it as.
//
// An empty property schema — `{}` — says "anything". The browser's request row
// then carries `any`, the browser's request bodies are unchecked, and a
// mistyped field reaches the handler as a silent default. A mistyped field is
// a usage error, so the request contract has to name the type: the registry
// declares it, the OpenAPI document publishes it, and the browser's generated
// request types carry it.
//
// The console's route catalogue is what the browser can reach, so the guard
// walks the routes the console binds, not the registry's whole surface: a
// request body nobody can send from the browser is not the browser's contract.
func TestRequestSchemasNameEveryProperty(t *testing.T) {
	specs := map[string]opapi.Spec{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		specs[spec.Name] = spec
	}
	for _, route := range opapi.ConsoleRoutes() {
		if route.Kind != opapi.HTTPRouteJSON && route.Kind != opapi.HTTPRouteStream {
			continue
		}
		spec, ok := specs[route.Op]
		if !ok {
			t.Fatalf("%s: the console route %s names an operation the registry does not carry", route.Op, route.Path)
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
			t.Fatalf("%s: the request schema is not valid JSON: %v", route.Op, err)
		}
		for name, raw := range schema.Properties {
			var property struct {
				Type json.RawMessage `json:"type"`
			}
			if err := json.Unmarshal(raw, &property); err != nil {
				t.Fatalf("%s: the request property %q is not valid JSON: %v", route.Op, name, err)
			}
			if len(property.Type) == 0 {
				t.Errorf("%s (%s): the request property %q declares no type: %s", route.Op, route.Path, name, raw)
				continue
			}
			var single string
			if err := json.Unmarshal(property.Type, &single); err == nil {
				if !requestPropertyTypes[single] {
					t.Errorf("%s (%s): the request property %q declares the unknown type %q", route.Op, route.Path, name, single)
				}
				continue
			}
			var many []string
			if err := json.Unmarshal(property.Type, &many); err != nil {
				t.Errorf("%s (%s): the request property %q declares an unreadable type: %s", route.Op, route.Path, name, property.Type)
				continue
			}
			if len(many) == 0 {
				t.Errorf("%s (%s): the request property %q declares an empty type list", route.Op, route.Path, name)
				continue
			}
			for _, name := range many {
				if !requestPropertyTypes[name] {
					t.Errorf("%s (%s): the request property %q declares the unknown type %q", route.Op, route.Path, name, name)
				}
			}
		}
	}
}
