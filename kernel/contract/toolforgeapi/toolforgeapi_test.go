// SPDX-License-Identifier: MIT

package toolforgeapi

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestTheScriptToolWireShape: the row is the contract between the forge and
// everything that reads it, so its wire shape is fixed — the key set, and the
// fields that fall out of the wire when they are empty.
func TestTheScriptToolWireShape(t *testing.T) {
	st := ScriptTool{
		ID:          "t1",
		Name:        "fetch_weather",
		Description: "fetches the weather",
		Language:    "python",
		Code:        "print(1)",
		Status:      StatusActive,
		TestedOK:    true,
		TestedMS:    42,
		CreatedMS:   10,
		UpdatedMS:   20,
	}
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"id", "name", "description", "language", "code", "status", "tested_ok", "tested_ms", "created_ms", "updated_ms"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("the wire must carry %q: %s", key, raw)
		}
	}
	if got["status"] != "active" {
		t.Fatalf("status = %v, want active", got["status"])
	}
	if got["tested_ms"] != float64(42) {
		t.Fatalf("tested_ms = %v", got["tested_ms"])
	}

	// The optional fields fall out of the wire when empty.
	empty, err := json.Marshal(ScriptTool{})
	if err != nil {
		t.Fatalf("Marshal empty: %v", err)
	}
	var gotEmpty map[string]any
	if err := json.Unmarshal(empty, &gotEmpty); err != nil {
		t.Fatalf("Unmarshal empty: %v", err)
	}
	for _, key := range []string{"input_schema", "tested_ms"} {
		if _, ok := gotEmpty[key]; ok {
			t.Fatalf("an empty %q must fall out of the wire: %s", key, empty)
		}
	}
	for _, key := range []string{"id", "name", "description", "language", "code", "status", "tested_ok", "created_ms", "updated_ms"} {
		if _, ok := gotEmpty[key]; !ok {
			t.Fatalf("the wire must always carry %q: %s", key, empty)
		}
	}
}

// TestTheLifecycleStatusValues: the status values are wire too — the agent
// reads them back as strings.
func TestTheLifecycleStatusValues(t *testing.T) {
	if StatusDraft != "draft" || StatusActive != "active" || StatusQuarantined != "quarantined" {
		t.Fatalf("status values drifted: %q %q %q", StatusDraft, StatusActive, StatusQuarantined)
	}
}

// TestValidateJudgesEachRule: every rule of the draft gate has its own
// verdict AND its own message — a rule that fails for the wrong reason is a
// rule nobody can debug.
func TestValidateJudgesEachRule(t *testing.T) {
	ok := ScriptTool{Name: "fetch_weather", Description: "fetches it", Language: "python", Code: "print(1)"}
	if err := Validate(ok); err != nil {
		t.Fatalf("a valid tool must pass: %v", err)
	}

	cases := []struct {
		name string
		st   ScriptTool
		want string
	}{
		{"empty name", ScriptTool{Description: "d", Language: "python", Code: "c"}, "name must match"},
		{"name starts upper", ScriptTool{Name: "Fetch", Description: "d", Language: "python", Code: "c"}, "name must match"},
		{"name starts with a digit", ScriptTool{Name: "1fetch", Description: "d", Language: "python", Code: "c"}, "name must match"},
		{"name has a dash", ScriptTool{Name: "fetch-weather", Description: "d", Language: "python", Code: "c"}, "name must match"},
		{"name too long", ScriptTool{Name: strings.Repeat("a", 41), Description: "d", Language: "python", Code: "c"}, "name must match"},
		{"empty description", ScriptTool{Name: "a", Language: "python", Code: "c"}, "description is required"},
		{"blank description", ScriptTool{Name: "a", Description: "   ", Language: "python", Code: "c"}, "description is required"},
		{"empty language", ScriptTool{Name: "a", Description: "d", Code: "c"}, "language is required"},
		{"language with a dash", ScriptTool{Name: "a", Description: "d", Language: "py-thon", Code: "c"}, "language is required"},
		{"empty code", ScriptTool{Name: "a", Description: "d", Language: "python"}, "code is required"},
		{"blank code", ScriptTool{Name: "a", Description: "d", Language: "python", Code: "  \n "}, "code is required"},
		{"schema is not json", ScriptTool{Name: "a", Description: "d", Language: "python", Code: "c", InputSchema: "{oops"}, "must be a JSON object"},
		{"schema is not an object", ScriptTool{Name: "a", Description: "d", Language: "python", Code: "c", InputSchema: "[1,2]"}, "must be a JSON object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.st)
			if err == nil {
				t.Fatalf("%s must be refused", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s: error = %q, want it to mention %q", tc.name, err, tc.want)
			}
		})
	}
}

// TestTheCapsAreTheValues: the caps are numbers the daemon promises, and a
// test that measures them against the constant itself would pass any value at
// all — so the numbers themselves are pinned here.
func TestTheCapsAreTheValues(t *testing.T) {
	if MaxCodeBytes != 128*1024 {
		t.Errorf("MaxCodeBytes = %d, want %d", MaxCodeBytes, 128*1024)
	}
	if MaxDescBytes != 2*1024 {
		t.Errorf("MaxDescBytes = %d, want %d", MaxDescBytes, 2*1024)
	}
	if MaxSchemaBytes != 16*1024 {
		t.Errorf("MaxSchemaBytes = %d, want %d", MaxSchemaBytes, 16*1024)
	}
}

// TestValidateJudgesTheNameLengthAtItsBoundary: the name pattern's length
// limit is a limit — a name of exactly 40 characters must pass, one more must
// not. A pattern that is only measured at its overflow would also pass every
// implementation that had no limit at all.
func TestValidateJudgesTheNameLengthAtItsBoundary(t *testing.T) {
	atLimit := ScriptTool{
		Name:        strings.Repeat("a", 40),
		Description: "d",
		Language:    "python",
		Code:        "c",
	}
	if err := Validate(atLimit); err != nil {
		t.Fatalf("a name of exactly 40 characters must pass: %v", err)
	}
	overLimit := atLimit
	overLimit.Name = strings.Repeat("a", 41)
	if err := Validate(overLimit); err == nil || !strings.Contains(err.Error(), "name must match") {
		t.Fatalf("a name of 41 characters must be refused, got %v", err)
	}
}

// TestValidateJudgesTheSizeCaps: the caps are limits, so the boundary is
// the contract — a cap that is only measured at cap+1 would pass every
// implementation that has no cap at all.
func TestValidateJudgesTheSizeCaps(t *testing.T) {
	base := ScriptTool{Name: "a", Description: "d", Language: "python", Code: "c"}

	atCap := base
	atCap.Description = strings.Repeat("x", MaxDescBytes)
	if err := Validate(atCap); err != nil {
		t.Fatalf("a description of exactly the cap must pass: %v", err)
	}
	overCap := base
	overCap.Description = strings.Repeat("x", MaxDescBytes+1)
	if err := Validate(overCap); err == nil || !strings.Contains(err.Error(), "description exceeds") {
		t.Fatalf("a description over the cap must be refused, got %v", err)
	}

	atCap = base
	atCap.Code = strings.Repeat("x", MaxCodeBytes)
	if err := Validate(atCap); err != nil {
		t.Fatalf("code of exactly the cap must pass: %v", err)
	}
	overCap = base
	overCap.Code = strings.Repeat("x", MaxCodeBytes+1)
	if err := Validate(overCap); err == nil || !strings.Contains(err.Error(), "code exceeds") {
		t.Fatalf("code over the cap must be refused, got %v", err)
	}

	// The schema cap counts the whole schema, JSON wrapper included.
	const wrapper = len(`{"k":""}`)
	atCap = base
	atCap.InputSchema = `{"k":"` + strings.Repeat("x", MaxSchemaBytes-wrapper) + `"}`
	if len(atCap.InputSchema) != MaxSchemaBytes {
		t.Fatalf("the fixture must be exactly at the cap, got %d", len(atCap.InputSchema))
	}
	if err := Validate(atCap); err != nil {
		t.Fatalf("a schema of exactly the cap must pass: %v", err)
	}
	overCap = base
	overCap.InputSchema = `{"k":"` + strings.Repeat("x", MaxSchemaBytes-wrapper+1) + `"}`
	if err := Validate(overCap); err == nil || !strings.Contains(err.Error(), "input_schema exceeds") {
		t.Fatalf("a schema over the cap must be refused, got %v", err)
	}
}

// TestValidateAcceptsAnEmptySchemaAndAJSONObject: the optional schema is
// either absent or a JSON OBJECT — a valid JSON array is not a schema.
func TestValidateAcceptsAnEmptySchemaAndAJSONObject(t *testing.T) {
	st := ScriptTool{Name: "a", Description: "d", Language: "python", Code: "c", InputSchema: "   "}
	if err := Validate(st); err != nil {
		t.Fatalf("a blank schema must pass: %v", err)
	}
	st.InputSchema = `{"type":"object"}`
	if err := Validate(st); err != nil {
		t.Fatalf("a JSON object schema must pass: %v", err)
	}
}

// TestTheStorePortShape: the port is the contract the agent-facing tool
// reads through, so its method set is fixed — an extra method would be a
// capability the plugin could reach without the daemon's consent.
func TestTheStorePortShape(t *testing.T) {
	typ := reflect.TypeOf((*Store)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Store must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 2 {
		names := make([]string, 0, typ.NumMethod())
		for i := 0; i < typ.NumMethod(); i++ {
			names = append(names, typ.Method(i).Name)
		}
		t.Fatalf("the port must have exactly Get and List, got %v", names)
	}
	get, ok := typ.MethodByName("Get")
	if !ok {
		t.Fatal("the port must have Get")
	}
	if get.Type.NumOut() != 2 || get.Type.Out(0) != reflect.TypeOf(ScriptTool{}) || get.Type.Out(1).Kind() != reflect.Bool {
		t.Fatalf("Get must return (ScriptTool, bool), got %v", get.Type)
	}
	list, ok := typ.MethodByName("List")
	if !ok {
		t.Fatal("the port must have List")
	}
	if list.Type.NumOut() != 1 || list.Type.Out(0) != reflect.TypeOf([]ScriptTool{}) {
		t.Fatalf("List must return []ScriptTool, got %v", list.Type)
	}
}
