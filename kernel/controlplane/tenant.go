// SPDX-License-Identifier: MIT
//
// kernel/controlplane tenant command registrar (registerTenantCommands).
// Extracted from tenant.go during Day 211 god-file refactor (#98).
// Public API unchanged.
package controlplane

func registerTenantCommands() {
	register(
		commandSpec{Cmd: CmdTenantCreate, Handler: func(dc *DispatchCtx) { dc.S.handleTenantCreate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdTenantList, Handler: func(dc *DispatchCtx) { dc.S.handleTenantList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdTenantRelease, Handler: func(dc *DispatchCtx) { dc.S.handleTenantRelease(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdTenantRemove, Handler: func(dc *DispatchCtx) { dc.S.handleTenantRemove(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdTenantToken, Handler: func(dc *DispatchCtx) { dc.S.handleTenantToken(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdTenantStats, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleTenantStats(dc.Conn, dc.Req) }},
	)
}
