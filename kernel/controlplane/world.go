// SPDX-License-Identifier: MIT
//
// kernel/controlplane world-model registrar (registerWorldCommands)
// + entity-view helper (entityView).
// Extracted from world.go during Day 211 god-file refactor (#79).
// Public API unchanged.
package controlplane

import (
	"github.com/agezt/agezt/kernel/worldmodel"
)

func entityView(e worldmodel.Entity) map[string]any {
	v := map[string]any{
		"id":           e.ID,
		"kind":         string(e.Kind),
		"name":         e.Name,
		"weight":       e.Weight,
		"created_ms":   e.CreatedMS,
		"last_seen_ms": e.LastSeenMS,
	}
	if len(e.Aliases) > 0 {
		v["aliases"] = e.Aliases
	}
	if len(e.Attrs) > 0 {
		v["attrs"] = e.Attrs
	}
	if e.SourceEvent != "" {
		v["source_event"] = e.SourceEvent
	}
	if e.SupersededBy != "" {
		v["superseded_by"] = e.SupersededBy
	}
	if e.Tombstoned {
		v["tombstoned"] = true
	}
	return v
}
func registerWorldCommands() {
	register(
		commandSpec{Cmd: CmdWorldAdd, Handler: func(dc *DispatchCtx) { dc.S.handleWorldAdd(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorldEdit, Handler: func(dc *DispatchCtx) { dc.S.handleWorldEdit(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorldRelate, Handler: func(dc *DispatchCtx) { dc.S.handleWorldRelate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorldResolve, Handler: func(dc *DispatchCtx) { dc.S.handleWorldResolve(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorldNeighbors, Handler: func(dc *DispatchCtx) { dc.S.handleWorldNeighbors(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorldList, Handler: func(dc *DispatchCtx) { dc.S.handleWorldList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorldGet, Handler: func(dc *DispatchCtx) { dc.S.handleWorldGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorldForget, Handler: func(dc *DispatchCtx) { dc.S.handleWorldForget(dc.Conn, dc.Req) }},
	)
}
