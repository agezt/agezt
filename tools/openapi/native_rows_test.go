// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/controlplane"
)

// The console's OWN rows reach the browser's catalogue.
//
// A native route that names the row it answers with, and a browser that reads
// `any` for that path, is the old drift waiting to happen: the browser writes
// its own view by hand, and nothing says the two are the same row.
//
// So the catalogue must carry every named row, and the map must bind the path
// to it.

// catalogue renders the browser's payload types the way the tool does.
func catalogue(t *testing.T) string {
	t.Helper()
	doc, err := opapi.BuildOpenAPI(func(op string) (opapi.OperationSchema, bool) {
		schema, ok := controlplane.AppOperationSchemas()[op]
		return schema, ok
	}, opapi.OpenAPIInfo{Title: "AGEZT Console API", Version: brand.Version})
	if err != nil {
		t.Fatalf("build the API document: %v", err)
	}
	out, err := renderTypes(opapi.ConsoleRoutes(), opapi.ConsoleNativeRoutes(), doc)
	if err != nil {
		t.Fatalf("render the types: %v", err)
	}
	return out
}

// TestEveryNativeRowReachesTheBrowserCatalogue: every native route that
// names a row is rendered, and its path is bound to that row's type.
func TestEveryNativeRowReachesTheBrowserCatalogue(t *testing.T) {
	out := catalogue(t)
	named := 0
	for _, route := range opapi.ConsoleNativeRoutes() {
		if route.Answer == nil {
			continue
		}
		named++
		if !strings.Contains(out, "export interface "+pascalName(string(route.Name))+"Response {") {
			t.Errorf("the native route %q names the row it answers with, but the browser's "+
				"catalogue declares no type for it", route.Name)
		}
		if !strings.Contains(out, `"`+route.Path+`": `) {
			t.Errorf("the native route %q names the row it answers with, but the catalogue's "+
				"native response map binds no path to it", route.Name)
		}
	}
	if named == 0 {
		t.Fatal("no native route names a row — the catalogue has nothing to render")
	}
	if !strings.Contains(out, "export interface NativeResponseMap {") {
		t.Error("the catalogue has no native response map")
	}
	if !strings.Contains(out, "P extends keyof NativeResponseMap") {
		t.Error("the browser's ResponseOf does not consult the native response map")
	}
}

// TestTheSchemaOfARowFollowsTheContract: the struct-to-schema step keeps the
// contract's own rules — a member without `omitempty` is required, one with it
// may be absent, and a POINTER says "may be null".
func TestTheSchemaOfARowFollowsTheContract(t *testing.T) {
	raw, err := schemaOf(rollbackRow{})
	if err != nil {
		t.Fatalf("render the schema: %v", err)
	}
	var node schemaNode
	if err := json.Unmarshal(raw, &node); err != nil {
		t.Fatalf("parse the schema: %v", err)
	}
	if node.hasType("object") == false {
		t.Fatalf("the row's schema is not an object: %s", raw)
	}
	for _, name := range []string{"id", "kind", "created_ms"} {
		if _, ok := node.Properties[name]; !ok {
			t.Errorf("the row's schema does not carry the member %q: %s", name, raw)
		}
	}
	if _, ok := node.Properties["run_id"]; !ok {
		t.Errorf("the row's schema does not carry the optional member run_id: %s", raw)
	}
	required := map[string]bool{}
	for _, name := range node.Required {
		required[name] = true
	}
	if !required["id"] || !required["kind"] || !required["created_ms"] {
		t.Errorf("the row's schema does not require the members the contract always carries: %s", raw)
	}
	if required["run_id"] {
		t.Errorf("the row's schema requires a member the contract may omit: %s", raw)
	}
}

// rollbackRow is a row shaped like the contract's own: required members, an
// optional member, and a pointer that says "may be null".
type rollbackRow struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	RunID     string  `json:"run_id,omitempty"`
	Subject   *string `json:"subject"`
	CreatedMS int64   `json:"created_ms"`
}
