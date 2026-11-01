// SPDX-License-Identifier: MIT

package loopcfg

import (
	"context"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
)

// costFn is the governor's price function. The loop's per-run spend ceiling
// (MaxRunCostMicrocents) is INERT without it, so every governed run — root or
// delegated — carries it.
var costFn = governor.CostMicrocents

// rescueMarkers are the markers the loop uses to escape a compacted context.
var rescueMarkers = []string{toolapi.DefaultContextRescueMarker}

// Build assembles the parts of agent.LoopConfig that must be IDENTICAL for
// every governed run — root (RunWith) or delegated (executeSubAgent).
//
// Before this rule existed the two sites each hand-built the config and had
// silently diverged: delegated runs got no CostFn (which per the LoopConfig
// contract makes MaxRunCostMicrocents inert — a named agent's per-run spend
// ceiling was a no-op for delegations), no artifact offload, no context
// compaction, and no tool memo.
//
// Run-identity fields (Model chain, Agent, Actor, CorrelationID, System,
// Wake*, Steer, cost ceiling source) stay with the caller; everything
// governance- and capacity-shaped lives here.
func (s *Service) Build(runCtx context.Context, corr, model string, cfg Config) agent.LoopConfig {
	// Per-run tool set: forged script tools and live MCP attachments are
	// merged by the daemon BEFORE the allowlist filter so a restricted run
	// only sees the dynamic tools its allowlist grants (M794/M796).
	runTools := s.deps.Tools(runCtx)
	runTools = AgentTools(runCtx, runTools)
	runTools = applyAgentNoisePolicyToPromptTools(runTools, runCtx)
	if allow, ok := runctx.ToolsFromCtx(runCtx); ok {
		runTools = filterTools(runTools, allow)
	}
	if cfg.ToolDiscoveryMax > 0 && len(runTools) > cfg.ToolDiscoveryMax {
		runTools = withToolSearch(runTools)
	}
	toolSelector := agent.LexicalToolSelector(cfg.ToolDiscoveryMax)
	if _, ok := runTools[toolSearchName]; ok && cfg.ToolDiscoveryMax > 0 {
		toolSelector = agent.DeferredLexicalToolSelector(cfg.ToolDiscoveryMax, []string{toolSearchName})
	}

	// Context budget (SPEC-10 §3): an explicit budget wins; otherwise, in auto
	// mode, derive one from the resolved model's catalog context window. An
	// unknown/empty model leaves compaction off (0).
	ctxBudget := cfg.ContextBudget
	if ctxBudget == 0 && cfg.ContextBudgetAuto {
		if m, ok := s.findModel(model); ok {
			ctxBudget = agent.AutoContextBudgetChars(m.Limit.Context)
		}
	}

	// Abstractive summary of elided tool outputs (M398): opt-in, and only worth
	// wiring when compaction is actually active for this run.
	var summarizeElided func(context.Context, string) (string, error)
	if cfg.ContextSummarize && (ctxBudget > 0 || cfg.ContextBudgetAuto) {
		maxTok := elidedSummaryMaxTokens
		// A reasoning model needs chain-of-thought headroom or the summary
		// comes back empty (M926).
		if m, ok := s.findModel(model); ok && m.Reasoning {
			maxTok = elidedSummaryReasoningMaxTokens
		}
		summarizeElided = makeElidedSummarizer(cfg.Provider, model, corr, maxTok)
	}

	return agent.LoopConfig{
		Provider:             cfg.Provider,
		Tools:                runTools,
		ToolPhases:           s.deps.Phases,
		Bus:                  s.deps.Bus,
		Model:                model,
		MaxIter:              cfg.MaxIter,
		MaxAutoContinue:      cfg.MaxAutoContinue,  // M833: autonomous continue past MaxIter
		AutoContinueWait:     cfg.AutoContinueWait, // M833
		ToolTimeout:          cfg.ToolTimeout,
		MaxParallelTools:     cfg.MaxParallelTools, // M880: in-turn parallel tool dispatch
		Policy:               s.deps.Policy,
		ToolSelector:         toolSelector,
		ToolResultHook:       s.deps.Notify,
		ObservationDeltas:    cfg.ObservationDeltas,
		ToolMemo:             agent.NewToolMemo(agent.DefaultToolMemoTTL, agent.DefaultToolMemoMaxEntries),
		CostFn:               costFn,           // required or MaxRunCostMicrocents is inert
		Artifacts:            s.deps.Artifacts, // M390: offload oversized tool outputs
		ArtifactThreshold:    cfg.ArtifactThreshold,
		ContextBudget:        ctxBudget,               // M393/M394 (SPEC-10 §3)
		ContextProtectFirst:  cfg.ContextProtectFirst, // M395
		SummarizeElided:      summarizeElided,         // M398
		ContextRescueMarkers: rescueMarkers,
	}
}
