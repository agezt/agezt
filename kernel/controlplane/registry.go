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
	registerChannelCommands()
	registerCognitionCommands()
	registerConfigCenterCommands()
	registerCoreCommands()
	registerDaemonOpsCommands()
	registerDatalakeCommands()
	registerEdictCommands()
	registerFileCommands()
	registerJournalLogCommands()
	registerMCPCommands()
	registerMarketCommands()
	registerMiscSmallCommands()
	registerOKRCommands()
	registerProviderConfigCommands()
	registerPulseControlCommands()
	registerRosterCommands()
	registerScheduleCommands()
	registerSettingsCommands()
	registerStandingCommands()
	registerSteerCommands()
	registerTenantCommands()
	registerToolforgeCommands()
	registerWorkboardCommands()
	registerWorkflowCommands()
}

// registerJournalLogCommands registers Read-only journal projections and log/stat folds from small single-purpose files.
func registerJournalLogCommands() {
	register(
		commandSpec{Cmd: CmdApprovalsLog, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleApprovalsLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdApprovalsStats, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleApprovalsStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdCacheStats, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleCacheStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChangelog, ReadOnly: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleChangelog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdJournalTail, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleJournalTail(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdJournalHead, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleJournalHead(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdJournalExport, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleJournalExport(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdJournalGrep, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleJournalGrep(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdJournalStats, ReadOnly: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleJournalStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdNetguardLog, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleNetguardLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictLog, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictStats, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRateLimitLog, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleRateLimitLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRateLimitStats, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleRateLimitStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdScheduleFires, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleScheduleFires(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdScheduleStats, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleScheduleStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdToolLog, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleToolLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdToolStats, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleToolStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWardenLog, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleWardenLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWardenStats, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleWardenStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWebhookLog, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleWebhookLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWebhookStats, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleWebhookStats(dc.Conn, dc.Req) }},
	)
}

// registerProviderConfigCommands registers Provider credentials/OAuth, model routing/chains, budgets, execution profiles, config.
func registerProviderConfigCommands() {
	register(
		commandSpec{Cmd: CmdBudget, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleBudget(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdBudgetSet, Handler: func(dc *DispatchCtx) { dc.S.handleBudgetSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChainsGet, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleChainsGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChainsSet, Handler: func(dc *DispatchCtx) { dc.S.handleChainsSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfig, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleConfig(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdExecutionProfiles, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleExecutionProfiles(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdExecutionProfileShow, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleExecutionProfileShow(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdExecutionProfileCheck, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleExecutionProfileCheck(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRoutingGet, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleRoutingGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRoutingSet, Handler: func(dc *DispatchCtx) { dc.S.handleRoutingSet(dc.Conn, dc.Req) }},
	)
}

// registerChannelCommands registers Communication channels: accounts, OAuth, sessions, inbox, outbound send, ACP.
func registerChannelCommands() {
	register(
		commandSpec{Cmd: CmdACPAgents, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleACPAgents(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChannelAccountSet, Handler: func(dc *DispatchCtx) { dc.S.handleChannelAccountSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChannelAccountRemove, Handler: func(dc *DispatchCtx) { dc.S.handleChannelAccountRemove(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChannelOAuthStart, Handler: func(dc *DispatchCtx) { dc.S.handleChannelOAuthStart(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChannelOAuthCallback, Handler: func(dc *DispatchCtx) { dc.S.handleChannelOAuthCallback(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChannelOAuthStatus, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleChannelOAuthStatus(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWhatsAppGatewayStatus, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleWhatsAppGatewayStatus(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWhatsAppGatewayQR, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleWhatsAppGatewayQR(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChannelList, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleChannelList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdInbox, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleInbox(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSend, Handler: func(dc *DispatchCtx) { dc.S.handleSend(dc.Conn, dc.Req) }},
	)
}

// registerDaemonOpsCommands registers Daemon operations: runs listing, state/status, disk, storage, sandbox, updates, shutdown.
func registerDaemonOpsCommands() {
	register(
		// Mission Control "Spend today" tile (Day 28+1). Slim counterpart of
		// CmdBudget: just { total: int microcents } — no per-task breakdown. Lives
		// here next to CmdBudget/CmdStatus rather than a new register func since
		// it's a one-off and the route count stays readable.
		commandSpec{Cmd: CmdSpendToday, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleSpendToday(dc.Conn, dc.Req) }},
		// Mission Control "Needs your attention" panel (Day 28+1). Cross-cuts
		// pending approvals + recent pulse asks into one time-sorted, capped
		// feed. Same rationale for placement as CmdSpendToday.
		commandSpec{Cmd: CmdAttention, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleAttention(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAutonomyFeed, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleAutonomyFeed(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdDiskStats, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleDiskStats(dc.Conn, dc.Req) }},
		// pulse_subscribe is a long-lived stream but deliberately NOT StreamLive:
		// it manages its own connection lifecycle (handlePulseSubscribe clears the
		// read deadline itself and runs a bespoke disconnect watcher that re-arms
		// 500ms read deadlines and treats read timeouts as "idle, not gone").
		// Dispatch's cancelOnConnClose wrapper would add a SECOND goroutine
		// reading the same conn (a race) and its blocking Read would trip on the
		// watcher's 500ms deadlines, cancelling a healthy idle stream.
		commandSpec{Cmd: CmdPulseSubscribe, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handlePulseSubscribe(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdReaperScan, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleReaperScan(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRedactTest, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleRedactTest(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRunsList, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleRunsList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRunsStats, ReadOnly: true, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleRunsStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSandboxList, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleSandboxList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSandboxFile, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleSandboxFile(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSandboxDelete, Handler: func(dc *DispatchCtx) { dc.S.handleSandboxDelete(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdShutdown, Handler: func(dc *DispatchCtx) { dc.S.handleShutdown(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStateList, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleStateList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStateGet, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleStateGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStorageStats, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleStorageStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdUpdateCheck, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleUpdateCheck(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdUpdateApply, Handler: func(dc *DispatchCtx) { dc.S.handleUpdateApply(dc.Conn, dc.Req) }},
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
		commandSpec{Cmd: CmdArtifactGet, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleArtifactGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdArtifactList, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleArtifactList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdArtifactDelete, Handler: func(dc *DispatchCtx) { dc.S.handleArtifactDelete(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdArtifactCollect, Handler: func(dc *DispatchCtx) { dc.S.handleArtifactCollect(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictOverlay, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictOverlay(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictCompact, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictCompact(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPluginList, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handlePluginList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdToolList, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleToolList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentPermissions, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleAgentPermissions(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentCapabilities, Handler: func(dc *DispatchCtx) { dc.S.handleAgentCapabilities(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdToolboxDetect, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleToolboxDetect(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdToolboxOutdated, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleToolboxOutdated(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdToolboxInstall, Streaming: StreamEvents, Handler: func(dc *DispatchCtx) { dc.S.handleToolboxInstall(dc.Ctx, dc.Conn, dc.Req) }},
	)
}
