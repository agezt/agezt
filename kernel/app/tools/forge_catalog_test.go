// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/toolforge"
	"reflect"
	"testing"
)

type forgeReaderProbe struct {
	rows                []toolforge.ScriptTool
	found               bool
	getRef              string
	listCalls, getCalls int
}

func (p *forgeReaderProbe) List() []toolforge.ScriptTool { p.listCalls++; return p.rows }
func (p *forgeReaderProbe) Get(ref string) (toolforge.ScriptTool, bool) {
	p.getCalls++
	p.getRef = ref
	return p.rows[0], p.found
}
func forgeJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err = json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}
func TestForgeCatalogListKeepsStoreOrderCountsAndLightViews(t *testing.T) {
	rows := []toolforge.ScriptTool{
		{ID: "z", Name: "zeta", Status: toolforge.StatusActive, Code: "secret code", TestedOK: true, TestedMS: 123, InputSchema: `{"type":"object"}`, CreatedMS: 10, UpdatedMS: 20},
		{ID: "b", Name: "beta", Status: toolforge.StatusDraft, Code: "draft code"},
		{ID: "a", Name: "alpha", Status: toolforge.StatusQuarantined, Code: "pulled code"},
		{ID: "f", Name: "future", Status: toolforge.Status("future")},
		{ID: "a2", Name: "active2", Status: toolforge.StatusActive},
	}
	p := &forgeReaderProbe{rows: rows}
	service := NewForgeCatalog(p)
	out, err := service.List(context.Background(), ForgeListInput{})
	if err != nil || out.Count != 5 || out.ActiveCount != 2 || p.listCalls != 1 || p.getCalls != 0 {
		t.Fatal(out, err, p)
	}
	for i, row := range out.Tools {
		m := forgeJSON(t, row)
		if m["name"] != rows[i].Name {
			t.Fatal(i, m)
		}
		if _, ok := m["code"]; ok {
			t.Fatal("light view code", m)
		}
		callable, ok := m["callable_as"]
		if rows[i].Status == toolforge.StatusActive {
			if !ok || callable != "forge_"+rows[i].Name {
				t.Fatal(m)
			}
		} else if ok {
			t.Fatal(m)
		}
	}
	if !reflect.DeepEqual(p.rows, rows) {
		t.Fatal("reader rows mutated")
	}
	p.rows = nil
	empty, err := service.List(context.Background(), ForgeListInput{})
	if err != nil || empty.Tools == nil || empty.Count != 0 || empty.ActiveCount != 0 {
		t.Fatal(empty, err)
	}
	m := forgeJSON(t, empty)
	if !reflect.DeepEqual(m["tools"], []any{}) || m["count"] != float64(0) || m["active_count"] != float64(0) {
		t.Fatal(m)
	}
	p.rows = []toolforge.ScriptTool{{Name: "fresh", Status: toolforge.StatusActive}}
	fresh, _ := service.List(context.Background(), ForgeListInput{})
	if fresh.Count != 1 || fresh.ActiveCount != 1 || forgeJSON(t, fresh.Tools[0])["name"] != "fresh" || p.listCalls != 3 {
		t.Fatal(fresh, p)
	}
}
func TestForgeCatalogShowTrimsLookupPreservesRawUnknownAndFullView(t *testing.T) {
	row := toolforge.ScriptTool{ID: "id", Name: "named", Description: " raw ", Language: "python", Code: "\n owned code \n", InputSchema: `{"type":"object"}`, Status: toolforge.StatusActive, TestedOK: true, TestedMS: 9, CreatedMS: 12, UpdatedMS: 13}
	p := &forgeReaderProbe{rows: []toolforge.ScriptTool{row}, found: true}
	out, err := NewForgeCatalog(p).Show(context.Background(), ForgeShowInput{Ref: " \t id \n"})
	if err != nil || p.getRef != "id" || p.getCalls != 1 || p.listCalls != 0 || out.Tool.Code != row.Code || out.Tool.CallableAs != "forge_named" {
		t.Fatal(out, err, p)
	}
	want := forgeJSON(t, row)
	want["callable_as"] = "forge_named"
	if !reflect.DeepEqual(forgeJSON(t, out.Tool), want) {
		t.Fatal(out.Tool, want)
	}
	p.found = false
	out, err = NewForgeCatalog(p).Show(context.Background(), ForgeShowInput{Ref: "  missing  "})
	if err == nil || err.Error() != "unknown script tool:   missing  " || out.Tool != (ForgeDetail{}) || p.getRef != "missing" {
		t.Fatal(out, err, p)
	}
}
func TestForgeToolViewRequiredOptionalPresenceAndExactTimestamps(t *testing.T) {
	light := forgeJSON(t, ForgeToolView(toolforge.ScriptTool{}))
	required := []string{"id", "name", "description", "language", "status", "tested_ok", "created_ms", "updated_ms"}
	if len(light) != len(required) {
		t.Fatal(light)
	}
	for _, key := range required {
		if _, ok := light[key]; !ok {
			t.Fatal(key, light)
		}
	}
	if light["tested_ok"] != false || light["created_ms"] != float64(0) || light["updated_ms"] != float64(0) {
		t.Fatal(light)
	}
	full := forgeJSON(t, ForgeDetail{})
	if code, ok := full["code"]; !ok || code != "" {
		t.Fatal(full)
	}
	row := toolforge.ScriptTool{TestedMS: -2, CreatedMS: 9007199254740993, UpdatedMS: 9223372036854775807, InputSchema: " raw schema ", Status: toolforge.StatusActive, Name: "unicode_ö"}
	m := forgeJSON(t, ForgeToolView(row))
	if m["tested_ms"] != float64(-2) || m["input_schema"] != row.InputSchema || m["created_ms"] != float64(row.CreatedMS) || m["updated_ms"] != float64(row.UpdatedMS) || m["callable_as"] != "forge_unicode_ö" {
		t.Fatal(m)
	}
	delete(m, "name")
	if row.Name != "unicode_ö" {
		t.Fatal(row)
	}
	again := forgeJSON(t, ForgeToolView(row))
	if again["name"] != row.Name {
		t.Fatal(again)
	}
}

func TestForgeViewTimestampProjectionKeepsInt64Values(t *testing.T) {
	row := toolforge.ScriptTool{CreatedMS: 9007199254740993, UpdatedMS: 9223372036854775807, TestedMS: 9007199254740993}
	view := ForgeToolView(row)
	if view.CreatedMS != row.CreatedMS || view.UpdatedMS != row.UpdatedMS || view.TestedMS != row.TestedMS {
		t.Fatal(view)
	}
	raw, _ := json.Marshal(view)
	var numbers map[string]json.RawMessage
	_ = json.Unmarshal(raw, &numbers)
	for key, want := range map[string]string{"created_ms": "9007199254740993", "tested_ms": "9007199254740993", "updated_ms": "9223372036854775807"} {
		if string(numbers[key]) != want {
			t.Fatal(key, string(numbers[key]), want)
		}
	}
}
