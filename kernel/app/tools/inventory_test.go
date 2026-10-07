// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"reflect"
	"testing"
)

type inventoryReader struct {
	registered map[string]toolapi.Tool
	reads      int
}

func (p *inventoryReader) Tools() map[string]toolapi.Tool { p.reads++; return p.registered }

type inventoryTool struct {
	def                      toolapi.ToolDef
	definitions, invocations int
}

func (p *inventoryTool) Definition() toolapi.ToolDef { p.definitions++; return p.def }
func (p *inventoryTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	p.invocations++
	panic("inventory must not invoke tools")
}
func TestInventoryDefinitionNamesRegistrationAxesAndFreshRead(t *testing.T) {
	file := &inventoryTool{def: toolapi.ToolDef{Name: "z-defined", Description: " raw description ", Capability: toolapi.ToolCapability{Name: "declared-other-axis"}, Effect: toolapi.ToolEffect{Class: toolapi.EffectReversible, RollbackNotes: " raw rollback "}}}
	unknown := &inventoryTool{def: toolapi.ToolDef{Name: "a-defined"}}
	port := &inventoryReader{registered: map[string]toolapi.Tool{"file": file, "unknown": unknown}}
	service := NewInventory(port)
	out, err := service.List(context.Background(), InventoryInput{})
	want := InventoryOutput{Tools: []InventoryItem{{Name: "a-defined", Capability: "unknown", RollbackMode: "unknown"}, {Name: "z-defined", Description: " raw description ", Capability: string(edict.CapFileWrite), EffectClass: "reversible", RollbackMode: "rollbackable", RollbackNotes: " raw rollback "}}, Count: 2}
	if err != nil || !reflect.DeepEqual(out, want) || port.reads != 1 || file.definitions != 1 || unknown.definitions != 1 || file.invocations != 0 || unknown.invocations != 0 {
		t.Fatal(out, want, err, port, file, unknown)
	}
	delete(port.registered, "unknown")
	again, err := service.List(context.Background(), InventoryInput{})
	if err != nil || again.Count != 1 || again.Tools[0].Name != "z-defined" || port.reads != 2 || file.definitions != 2 || unknown.definitions != 1 {
		t.Fatal(again, err, port, file, unknown)
	}
}
func TestInventoryPrimaryAxesAndRollbackContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		want edict.Capability
	}{{"file", edict.CapFileWrite}, {"http", edict.CapHTTPPost}, {"homeassistant", edict.CapHomeAssistantCall}, {"shell", edict.CapShell}, {"forge_owned", edict.CapCodeExec}, {"mcp_owned_echo", edict.CapMCP}, {"unknown", edict.Capability("unknown")}, {"FILE", edict.Capability("FILE")}} {
		if got := PrimaryCapability(tc.name); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	for _, tc := range []struct {
		class toolapi.EffectClass
		mode  string
	}{{toolapi.EffectReadOnly, "none_needed"}, {toolapi.EffectReversible, "rollbackable"}, {toolapi.EffectCompensable, "compensate"}, {toolapi.EffectIrreversible, "audit_only"}, {toolapi.EffectUnknown, "unknown"}, {toolapi.EffectClass("future"), "unknown"}} {
		if got := toolRollbackMode(tc.class); got != tc.mode {
			t.Fatal(tc, got)
		}
	}
}
func TestInventoryEmptyAndRequiredWireFields(t *testing.T) {
	out, err := NewInventory(&inventoryReader{}).List(context.Background(), InventoryInput{})
	raw, _ := json.Marshal(out)
	if err != nil || string(raw) != `{"tools":[],"count":0}` {
		t.Fatal(string(raw), err)
	}
	item := InventoryItem{}
	encoded, _ := json.Marshal(item)
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire) != 6 {
		t.Fatal(wire)
	}
	for _, key := range []string{"name", "description", "capability", "effect_class", "rollback_mode", "rollback_notes"} {
		if value, ok := wire[key]; !ok || value != "" {
			t.Fatal(key, wire)
		}
	}
}
func TestInventoryDuplicateDefinitionNamesRetainBothEntries(t *testing.T) {
	port := &inventoryReader{registered: map[string]toolapi.Tool{"one": &inventoryTool{def: toolapi.ToolDef{Name: "same"}}, "two": &inventoryTool{def: toolapi.ToolDef{Name: "same"}}}}
	out, err := NewInventory(port).List(context.Background(), InventoryInput{})
	if err != nil || out.Count != 2 || len(out.Tools) != 2 || out.Tools[0].Name != "same" || out.Tools[1].Name != "same" {
		t.Fatal(out, err)
	}
	seen := map[string]bool{}
	for _, row := range out.Tools {
		seen[row.Capability] = true
	}
	if !seen["one"] || !seen["two"] {
		t.Fatal(out)
	}
}
