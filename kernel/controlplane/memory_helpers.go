// SPDX-License-Identifier: MIT

package controlplane

// Provenance: Memory helpers: jsonMap + registerMemoryCommands. Code
//             extracted from memory.go during the Day-61 god-file split. Public API
//             unchanged.

import (
	"encoding/json"
)

func jsonMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = json.Unmarshal(b, &out)
	return out, err
}

// registerMemoryCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerMemoryCommands() {
	register(
		commandSpec{Cmd: CmdMemoryAdd, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryAdd(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemorySupersede, Handler: func(dc *DispatchCtx) { dc.S.handleMemorySupersede(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryList, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryGet, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemorySearch, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleMemorySearch(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryConsolidate, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryConsolidate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProfileRebuild, Handler: func(dc *DispatchCtx) { dc.S.handleProfileRebuild(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryForget, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryForget(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryPromote, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryPromote(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryPrune, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryPrune(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryTidy, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryTidy(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryBulkForget, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryBulkForget(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryFindRelated, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryFindRelated(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryAudit, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryAudit(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryClean, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryClean(dc.Conn, dc.Req) }},
	)
}
