// SPDX-License-Identifier: MIT

// Tool inventory: catalogProbe + handleToolList + toolRollbackMode.
// Code extracted from tool.go during the Day-59 god-file split. Public API unchanged.
package controlplane


import (
	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
	"net"
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

// handleToolList serves CmdToolList. Returns a deterministic,
// name-sorted list so two consecutive calls produce identical
// output (Go map iteration is randomized, so we sort here rather
// than make the client do it). Each row carries the tool's governing
// Edict capability (its primary axis) so the operator can see the full
// agent capability surface and cross-reference it with the policy levels.
func (s *Server) handleToolList(conn net.Conn, req Request) {
	tools := s.k.Tools()
	rows := make([]map[string]any, 0, len(tools))
	for name, t := range tools {
		def := t.Definition()
		cap := edict.CapabilityForToolCall(name, catalogProbe[name])
		rows = append(rows, map[string]any{
			"name":           def.Name,
			"description":    def.Description,
			"capability":     string(cap),
			"effect_class":   string(def.Effect.Class),
			"rollback_mode":  toolRollbackMode(def.Effect.Class),
			"rollback_notes": def.Effect.RollbackNotes,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		ni, _ := rows[i]["name"].(string)
		nj, _ := rows[j]["name"].(string)
		return ni < nj
	})
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"tools": rows,
			"count": len(rows),
		},
	})
}

func toolRollbackMode(class agent.EffectClass) string {
	switch class {
	case agent.EffectReadOnly:
		return "none_needed"
	case agent.EffectReversible:
		return "rollbackable"
	case agent.EffectCompensable:
		return "compensate"
	case agent.EffectIrreversible:
		return "audit_only"
	default:
		return "unknown"
	}
}
