package controlplane

// Provenance: SPDX-License-Identifier: MIT kernel/controlplane world-model registrar
//             (registerWorldCommands). Extracted
//             from world.go during Day 211 god-file refactor (#79). Public API
//             unchanged.

func registerWorldCommands() {
	register(
		commandSpec{Cmd: CmdWorldAdd, Handler: func(dc *DispatchCtx) { dc.S.handleWorldAdd(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorldEdit, Handler: func(dc *DispatchCtx) { dc.S.handleWorldEdit(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorldRelate, Handler: func(dc *DispatchCtx) { dc.S.handleWorldRelate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorldResolve, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleWorldResolve(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorldNeighbors, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleWorldNeighbors(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorldList, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleWorldList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorldGet, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleWorldGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorldForget, Handler: func(dc *DispatchCtx) { dc.S.handleWorldForget(dc.Conn, dc.Req) }},
	)
}
