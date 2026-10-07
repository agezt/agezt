// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"sort"
)

// catalogProbe holds a representative input per input-branching tool so the
// catalog can report the tool's PRIMARY governed capability — the higher-risk
// axis an operator most wants to see the policy for. Tools that don't branch on
// input map to one capability regardless, so they're absent here (nil input).
var catalogProbe = map[string]json.RawMessage{
	"file":          json.RawMessage(`{"op":"write"}`),
	"http":          json.RawMessage(`{"method":"POST"}`),
	"homeassistant": json.RawMessage(`{"operation":"call_service"}`),
}

// PrimaryCapability retains the legacy inventory/permission representative axis.
// Registration name is authoritative here; this does not change runtime invocation.
func PrimaryCapability(name string) edict.Capability {
	return edict.CapabilityForToolCall(name, catalogProbe[name])
}

type InventoryReader interface {
	Tools() map[string]toolapi.Tool
}
type Inventory struct{ reader InventoryReader }

func NewInventory(reader InventoryReader) *Inventory { return &Inventory{reader: reader} }

type InventoryInput struct{}
type InventoryItem struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Capability    string `json:"capability"`
	EffectClass   string `json:"effect_class"`
	RollbackMode  string `json:"rollback_mode"`
	RollbackNotes string `json:"rollback_notes"`
}
type InventoryOutput struct {
	Tools []InventoryItem `json:"tools"`
	Count int             `json:"count"`
}

// List reads definitions without invoking any tool. Names sort by the advertised
// definition name; equal-name tie order retains the original map traversal policy.
func (s *Inventory) List(_ context.Context, _ InventoryInput) (InventoryOutput, error) {
	registered := s.reader.Tools()
	rows := make([]InventoryItem, 0, len(registered))
	for name, tool := range registered {
		def := tool.Definition()
		rows = append(rows, InventoryItem{Name: def.Name, Description: def.Description, Capability: string(PrimaryCapability(name)), EffectClass: string(def.Effect.Class), RollbackMode: toolRollbackMode(def.Effect.Class), RollbackNotes: def.Effect.RollbackNotes})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return InventoryOutput{Tools: rows, Count: len(rows)}, nil
}

func toolRollbackMode(class toolapi.EffectClass) string {
	switch class {
	case toolapi.EffectReadOnly:
		return "none_needed"
	case toolapi.EffectReversible:
		return "rollbackable"
	case toolapi.EffectCompensable:
		return "compensate"
	case toolapi.EffectIrreversible:
		return "audit_only"
	default:
		return "unknown"
	}
}
