// SPDX-License-Identifier: MIT

package controlplane

// Phase 2.3 (commit A): explicit registration of every protocol command.
// Each subsystem file with 5+ handlers hosts its own registerXCommands() at
// the bottom of that file; the ~35 small single-handler files are grouped
// into the themed register funcs below. dispatch_registry_test.go asserts
// 1:1 coverage against protocol.go, the legacy handleConn switch, and the
// legacy tenantTokenAllows allowlist.

func init() { registerAllCommands() }

// registerAllCommands populates commandRegistry with all protocol commands,
// one register func per subsystem. Explicit (not per-file init) so the full
// registration order is readable in one place.
func registerAllCommands() {
	registerAppSystemCommands()
	registerCognitionCommands()
	registerCoreCommands()
	registerFileCommands()
}

func registerCognitionCommands() {
	register(
		commandSpec{Cmd: CmdChatSuggestions, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleChatSuggestions(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChatSummarize, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handleChatSummarize(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConductorRoles, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleConductorRoles(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConductorAsk, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handleConductorAsk(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdCouncilAsk, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handleCouncilAsk(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdNodeRegistry, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleNodeRegistry(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPlanGenerate, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handlePlanGenerate(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPlanRefine, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handlePlanRefine(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdReflectRun, Handler: func(dc *DispatchCtx) { dc.S.handleReflectRun(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdReflectShow, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleReflectShow(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdResearchAsk, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handleResearchAsk(dc.Ctx, dc.Conn, dc.Req) }},
	)
}
