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

// assertRequestPropertyTypes fails for every property of a request body that
// declares no type, or a type the validator does not know.
func assertRequestPropertyTypes(t *testing.T, label string, spec opapi.Spec) {
	t.Helper()
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
		t.Fatalf("%s: the request schema is not valid JSON: %v", label, err)
	}
	for name, raw := range schema.Properties {
		var property struct {
			Type json.RawMessage `json:"type"`
		}
		if err := json.Unmarshal(raw, &property); err != nil {
			t.Fatalf("%s: the request property %q is not valid JSON: %v", label, name, err)
		}
		if len(property.Type) == 0 {
			t.Errorf("%s: the request property %q declares no type: %s", label, name, raw)
			continue
		}
		var single string
		if err := json.Unmarshal(property.Type, &single); err == nil {
			if !requestPropertyTypes[single] {
				t.Errorf("%s: the request property %q declares the unknown type %q", label, name, single)
			}
			continue
		}
		var many []string
		if err := json.Unmarshal(property.Type, &many); err != nil {
			t.Errorf("%s: the request property %q declares an unreadable type: %s", label, name, property.Type)
			continue
		}
		if len(many) == 0 {
			t.Errorf("%s: the request property %q declares an empty type list", label, name)
			continue
		}
		for _, member := range many {
			if !requestPropertyTypes[member] {
				t.Errorf("%s: the request property %q declares the unknown type %q", label, name, member)
			}
		}
	}
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
		assertRequestPropertyTypes(t, spec.Name+" ("+route.Path+")", spec)
	}
}

// TestRequestSchemasNameEveryPropertyBeyondTheConsole is the same guard for the
// request surface the console cannot reach (W5.9): the CLI and the SDK drive
// operations that carry no console route, and those bodies were the last ones
// still reading "anything goes".
//
// The console routes are a subset of the registry, so the whole registry is the
// stronger claim: after this guard, every request body the daemon accepts
// names the type its handler reads, and the door can refuse a mistyped field
// before the handler turns it into a silent default.
func TestRequestSchemasNameEveryPropertyBeyondTheConsole(t *testing.T) {
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		assertRequestPropertyTypes(t, spec.Name, spec)
	}
}
