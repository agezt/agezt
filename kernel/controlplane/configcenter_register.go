// SPDX-License-Identifier: MIT

// Control-plane configcenter command registry (registerConfigCenterCommands).
// Code extracted from configcenter_handler.go during the Day-114 god-file split.
// Public API unchanged.
package controlplane



func registerConfigCenterCommands() {
	register(
		commandSpec{Cmd: CmdConfigCenterSet, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigCenterGet, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigCenterList, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigCenterDelete, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterDelete(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigCenterSetRating, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterSetRating(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigCenterSetAccess, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterSetAccess(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigCenterAccessLog, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterAccessLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigCenterAudit, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterAudit(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigCenterHealth, Handler: func(dc *DispatchCtx) { dc.S.handleConfigCenterHealth(dc.Conn, dc.Req) }},
	)
}
