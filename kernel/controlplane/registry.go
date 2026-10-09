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
	registerDaemonOpsCommands()
	registerDatalakeCommands()
	registerFileCommands()
	registerJournalLogCommands()
	registerMiscSmallCommands()
	registerProviderConfigCommands()
	registerTenantCommands()
}

// registerJournalLogCommands registers Read-only journal projections and log/stat folds from small single-purpose files.
func registerJournalLogCommands() {
	register(
		commandSpec{Cmd: CmdApprovalsLog, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleApprovalsLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdApprovalsStats, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleApprovalsStats(dc.Conn, dc.Req) }},
	)
}

// registerProviderConfigCommands registers Provider credentials/OAuth, model routing/chains, budgets, execution profiles, config.
func registerProviderConfigCommands() {
	register(
		commandSpec{Cmd: CmdBudget, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleBudget(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdBudgetSet, Handler: func(dc *DispatchCtx) { dc.S.handleBudgetSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChainsGet, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleChainsGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChainsSet, Handler: func(dc *DispatchCtx) { dc.S.handleChainsSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdExecutionProfiles, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleExecutionProfiles(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdExecutionProfileShow, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleExecutionProfileShow(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdExecutionProfileCheck, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleExecutionProfileCheck(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRoutingGet, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleRoutingGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRoutingSet, Handler: func(dc *DispatchCtx) { dc.S.handleRoutingSet(dc.Conn, dc.Req) }},
	)
}

// registerDaemonOpsCommands registers Daemon operations: runs listing, state/status, disk, storage, sandbox, updates, shutdown.
func registerDaemonOpsCommands() {
	register(
		// Mission Control "Spend today" tile (Day 28+1). Slim counterpart of
		// CmdBudget: just { total: int microcents } â€” no per-task breakdown. Lives
		// here next to CmdBudget/CmdStatus rather than a new register func since
		// it's a one-off and the route count stays readable.
		commandSpec{Cmd: CmdSpendToday, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleSpendToday(dc.Conn, dc.Req) }},
		// Mission Control "Needs your attention" panel (Day 28+1). Cross-cuts
		// pending approvals + recent pulse asks into one time-sorted, capped
		// feed. Same rationale for placement as CmdSpendToday.
		commandSpec{Cmd: CmdAttention, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleAttention(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdDiskStats, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleDiskStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdReaperScan, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleReaperScan(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRedactTest, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleRedactTest(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSandboxList, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleSandboxList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSandboxFile, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleSandboxFile(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSandboxDelete, Handler: func(dc *DispatchCtx) { dc.S.handleSandboxDelete(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdShutdown, Handler: func(dc *DispatchCtx) { dc.S.handleShutdown(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStateList, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleStateList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStateGet, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleStateGet(dc.Conn, dc.Req) }},
	)
}

// registerCognitionCommands registers Cognition surfaces: planner, conductor/council, research, reflection, persona, prompts, taste, seats.
func registerCognitionCommands() {
	register(
		commandSpec{Cmd: CmdChatSuggestions, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleChatSuggestions(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChatSummarize, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handleChatSummarize(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConductorRoles, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleConductorRoles(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConductorAsk, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handleConductorAsk(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdCouncilMembers, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleCouncilMembers(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdCouncilAsk, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handleCouncilAsk(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdCouncilSet, Handler: func(dc *DispatchCtx) { dc.S.handleCouncilSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdNodeRegistry, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleNodeRegistry(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPersonaGet, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handlePersonaGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPersonaSet, Handler: func(dc *DispatchCtx) { dc.S.handlePersonaSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPlanHistory, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handlePlanHistory(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPlanStats, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handlePlanStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPlanGenerate, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handlePlanGenerate(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPlanRefine, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handlePlanRefine(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPromptsGet, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handlePromptsGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPromptsSet, Handler: func(dc *DispatchCtx) { dc.S.handlePromptsSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdReflectRun, Handler: func(dc *DispatchCtx) { dc.S.handleReflectRun(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdReflectShow, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleReflectShow(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdResearchAsk, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handleResearchAsk(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSeatList, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleSeatList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSeatCreate, Handler: func(dc *DispatchCtx) { dc.S.handleSeatCreate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSeatDelete, Handler: func(dc *DispatchCtx) { dc.S.handleSeatDelete(dc.Conn, dc.Req) }},
	)
}

// registerMiscSmallCommands registers Remaining small subsystems: artifacts, plugins, tools, toolbox, edict overlay.
func registerMiscSmallCommands() {
	register(
		commandSpec{Cmd: CmdEdictOverlay, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictOverlay(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictCompact, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictCompact(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentPermissions, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleAgentPermissions(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentCapabilities, Handler: func(dc *DispatchCtx) { dc.S.handleAgentCapabilities(dc.Conn, dc.Req) }},
	)
}
