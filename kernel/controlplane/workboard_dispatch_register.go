// SPDX-License-Identifier: MIT

package controlplane

// Workboard command registration: registerWorkboardCommands.
// Carved out of workboard_dispatch.go during the Day 195 god-file
// split so the main file can stay focused on the dispatch +
// watch internals and the args file can stay focused on
// arg-parsing + retry helpers.
// Public API unchanged.

import ()

func registerWorkboardCommands() {
	register(
		commandSpec{Cmd: CmdWorkboardList, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardLanes, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardLanes(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardShow, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardShow(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardCreate, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardCreate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardClaim, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardClaim(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardHeartbeat, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardHeartbeat(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardComment, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardComment(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardBlock, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardBlock(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardFail, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardFail(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardUnblock, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardUnblock(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardComplete, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardComplete(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardProve, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardProve(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardSeat, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardSeat(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardArchive, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardArchive(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardLink, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardLink(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardPolicy, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardPolicy(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardDepend, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardDepend(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardReclaim, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardReclaim(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardSweep, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardSweep(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardDispatch, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardDispatch(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardWatch, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardWatch(dc.Conn, dc.Req) }},
	)
}

