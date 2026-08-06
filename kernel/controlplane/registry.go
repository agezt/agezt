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
	registerBoardCommands()
	registerCatalogCommands()
	registerChannelCommands()
	registerCognitionCommands()
	registerConfigCenterCommands()
	registerCoreCommands()
	registerDaemonOpsCommands()
	registerDatalakeCommands()
	registerEdictCommands()
	registerJournalLogCommands()
	registerMCPCommands()
	registerMarketCommands()
	registerMemoryCommands()
	registerMiscSmallCommands()
	registerOKRCommands()
	registerProviderConfigCommands()
	registerPulseControlCommands()
	registerRosterCommands()
	registerScheduleCommands()
	registerSettingsCommands()
	registerSkillCommands()
	registerStandingCommands()
	registerSteerCommands()
	registerTenantCommands()
	registerToolforgeCommands()
	registerWorkboardCommands()
	registerWorkflowCommands()
	registerWorldCommands()
}

// registerJournalLogCommands registers Read-only journal projections and log/stat folds from small single-purpose files.
func registerJournalLogCommands() {
	register(
		commandSpec{Cmd: CmdApprovalsLog, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleApprovalsLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdApprovalsStats, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleApprovalsStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdCacheStats, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleCacheStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChangelog, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleChangelog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdJournalTail, Handler: func(dc *DispatchCtx) { dc.S.handleJournalTail(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdJournalHead, Handler: func(dc *DispatchCtx) { dc.S.handleJournalHead(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdJournalExport, Handler: func(dc *DispatchCtx) { dc.S.handleJournalExport(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdJournalGrep, Handler: func(dc *DispatchCtx) { dc.S.handleJournalGrep(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdJournalStats, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleJournalStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryLog, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdNetguardLog, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleNetguardLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictLog, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictStats, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderLog, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleProviderLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderStats, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleProviderStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderRejections, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleProviderRejections(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRateLimitLog, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleRateLimitLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRateLimitStats, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleRateLimitStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdScheduleFires, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleScheduleFires(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdScheduleStats, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleScheduleStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdToolLog, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleToolLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdToolStats, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleToolStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWardenLog, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleWardenLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWardenStats, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleWardenStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWebhookLog, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleWebhookLog(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWebhookStats, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleWebhookStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorldLog, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleWorldLog(dc.Conn, dc.Req) }},
	)
}

// registerProviderConfigCommands registers Provider credentials/OAuth, model routing/chains, budgets, execution profiles, config.
func registerProviderConfigCommands() {
	register(
		commandSpec{Cmd: CmdBudget, Handler: func(dc *DispatchCtx) { dc.S.handleBudget(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdBudgetSet, Handler: func(dc *DispatchCtx) { dc.S.handleBudgetSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChainsGet, Handler: func(dc *DispatchCtx) { dc.S.handleChainsGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChainsSet, Handler: func(dc *DispatchCtx) { dc.S.handleChainsSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfig, Handler: func(dc *DispatchCtx) { dc.S.handleConfig(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdExecutionProfiles, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleExecutionProfiles(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdExecutionProfileShow, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleExecutionProfileShow(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdExecutionProfileCheck, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleExecutionProfileCheck(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderKeyList, Handler: func(dc *DispatchCtx) { dc.S.handleProviderKeyList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderKeyAdd, Handler: func(dc *DispatchCtx) { dc.S.handleProviderKeyAdd(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderKeyActivate, Handler: func(dc *DispatchCtx) { dc.S.handleProviderKeyActivate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderKeyRemove, Handler: func(dc *DispatchCtx) { dc.S.handleProviderKeyRemove(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderOAuthStart, Handler: func(dc *DispatchCtx) { dc.S.handleProviderOAuthStart(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderOAuthStatus, Handler: func(dc *DispatchCtx) { dc.S.handleProviderOAuthStatus(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderOAuthImport, Handler: func(dc *DispatchCtx) { dc.S.handleProviderOAuthImport(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderOAuthLogout, Handler: func(dc *DispatchCtx) { dc.S.handleProviderOAuthLogout(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRoutingGet, Handler: func(dc *DispatchCtx) { dc.S.handleRoutingGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRoutingSet, Handler: func(dc *DispatchCtx) { dc.S.handleRoutingSet(dc.Conn, dc.Req) }},
	)
}

// registerChannelCommands registers Communication channels: accounts, OAuth, sessions, inbox, outbound send, ACP.
func registerChannelCommands() {
	register(
		commandSpec{Cmd: CmdACPAgents, Handler: func(dc *DispatchCtx) { dc.S.handleACPAgents(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChannelAccountSet, Handler: func(dc *DispatchCtx) { dc.S.handleChannelAccountSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChannelAccountRemove, Handler: func(dc *DispatchCtx) { dc.S.handleChannelAccountRemove(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChannelOAuthStart, Handler: func(dc *DispatchCtx) { dc.S.handleChannelOAuthStart(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChannelOAuthCallback, Handler: func(dc *DispatchCtx) { dc.S.handleChannelOAuthCallback(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChannelOAuthStatus, Handler: func(dc *DispatchCtx) { dc.S.handleChannelOAuthStatus(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderProbe, Handler: func(dc *DispatchCtx) { dc.S.handleProviderProbe(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWhatsAppGatewayStatus, Handler: func(dc *DispatchCtx) { dc.S.handleWhatsAppGatewayStatus(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWhatsAppGatewayQR, Handler: func(dc *DispatchCtx) { dc.S.handleWhatsAppGatewayQR(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChannelList, Handler: func(dc *DispatchCtx) { dc.S.handleChannelList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdInbox, Handler: func(dc *DispatchCtx) { dc.S.handleInbox(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSend, Handler: func(dc *DispatchCtx) { dc.S.handleSend(dc.Conn, dc.Req) }},
	)
}

// registerDaemonOpsCommands registers Daemon operations: runs listing, state/status, disk, storage, sandbox, updates, shutdown.
func registerDaemonOpsCommands() {
	register(
		commandSpec{Cmd: CmdAutonomyFeed, Handler: func(dc *DispatchCtx) { dc.S.handleAutonomyFeed(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdDiskStats, Handler: func(dc *DispatchCtx) { dc.S.handleDiskStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPulseSubscribe, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handlePulseSubscribe(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdReaperScan, Handler: func(dc *DispatchCtx) { dc.S.handleReaperScan(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRedactTest, Handler: func(dc *DispatchCtx) { dc.S.handleRedactTest(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRunsList, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleRunsList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRunsStats, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleRunsStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSandboxList, Handler: func(dc *DispatchCtx) { dc.S.handleSandboxList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSandboxFile, Handler: func(dc *DispatchCtx) { dc.S.handleSandboxFile(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSandboxDelete, Handler: func(dc *DispatchCtx) { dc.S.handleSandboxDelete(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdShutdown, Handler: func(dc *DispatchCtx) { dc.S.handleShutdown(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStateList, Handler: func(dc *DispatchCtx) { dc.S.handleStateList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStateGet, Handler: func(dc *DispatchCtx) { dc.S.handleStateGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStatus, Handler: func(dc *DispatchCtx) { dc.S.handleStatus(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStorageStats, Handler: func(dc *DispatchCtx) { dc.S.handleStorageStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdUpdateCheck, Handler: func(dc *DispatchCtx) { dc.S.handleUpdateCheck(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdUpdateApply, Handler: func(dc *DispatchCtx) { dc.S.handleUpdateApply(dc.Conn, dc.Req) }},
	)
}

// registerCognitionCommands registers Cognition surfaces: planner, conductor/council, research, reflection, persona, prompts, taste, seats.
func registerCognitionCommands() {
	register(
		commandSpec{Cmd: CmdChatSuggestions, Handler: func(dc *DispatchCtx) { dc.S.handleChatSuggestions(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdChatSummarize, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handleChatSummarize(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConductorRoles, Handler: func(dc *DispatchCtx) { dc.S.handleConductorRoles(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConductorAsk, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handleConductorAsk(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdCouncilMembers, Handler: func(dc *DispatchCtx) { dc.S.handleCouncilMembers(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdCouncilAsk, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handleCouncilAsk(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdCouncilSet, Handler: func(dc *DispatchCtx) { dc.S.handleCouncilSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdNodeRegistry, Handler: func(dc *DispatchCtx) { dc.S.handleNodeRegistry(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPersonaGet, Handler: func(dc *DispatchCtx) { dc.S.handlePersonaGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPersonaSet, Handler: func(dc *DispatchCtx) { dc.S.handlePersonaSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPlanHistory, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handlePlanHistory(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPlanStats, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handlePlanStats(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPlanGenerate, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handlePlanGenerate(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPlanRefine, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handlePlanRefine(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPromptsGet, Handler: func(dc *DispatchCtx) { dc.S.handlePromptsGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPromptsSet, Handler: func(dc *DispatchCtx) { dc.S.handlePromptsSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdReflectRun, Handler: func(dc *DispatchCtx) { dc.S.handleReflectRun(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdReflectShow, Handler: func(dc *DispatchCtx) { dc.S.handleReflectShow(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdResearchAsk, Streaming: StreamLive, Handler: func(dc *DispatchCtx) { dc.S.handleResearchAsk(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSeatList, Handler: func(dc *DispatchCtx) { dc.S.handleSeatList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSeatCreate, Handler: func(dc *DispatchCtx) { dc.S.handleSeatCreate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSeatDelete, Handler: func(dc *DispatchCtx) { dc.S.handleSeatDelete(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdTasteList, Handler: func(dc *DispatchCtx) { dc.S.handleTasteList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdTasteCreate, Handler: func(dc *DispatchCtx) { dc.S.handleTasteCreate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdTasteDelete, Handler: func(dc *DispatchCtx) { dc.S.handleTasteDelete(dc.Conn, dc.Req) }},
	)
}

// registerMiscSmallCommands registers Remaining small subsystems: artifacts, plugins, tools, toolbox, edict overlay.
func registerMiscSmallCommands() {
	register(
		commandSpec{Cmd: CmdArtifactGet, Handler: func(dc *DispatchCtx) { dc.S.handleArtifactGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdArtifactList, Handler: func(dc *DispatchCtx) { dc.S.handleArtifactList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdArtifactDelete, Handler: func(dc *DispatchCtx) { dc.S.handleArtifactDelete(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdArtifactCollect, Handler: func(dc *DispatchCtx) { dc.S.handleArtifactCollect(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictOverlay, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictOverlay(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictCompact, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictCompact(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPluginList, Handler: func(dc *DispatchCtx) { dc.S.handlePluginList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdToolList, Handler: func(dc *DispatchCtx) { dc.S.handleToolList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentPermissions, Handler: func(dc *DispatchCtx) { dc.S.handleAgentPermissions(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentCapabilities, Handler: func(dc *DispatchCtx) { dc.S.handleAgentCapabilities(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdToolboxDetect, Handler: func(dc *DispatchCtx) { dc.S.handleToolboxDetect(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdToolboxOutdated, Handler: func(dc *DispatchCtx) { dc.S.handleToolboxOutdated(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdToolboxInstall, Streaming: StreamEvents, Handler: func(dc *DispatchCtx) { dc.S.handleToolboxInstall(dc.Ctx, dc.Conn, dc.Req) }},
	)
}
