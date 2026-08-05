// SPDX-License-Identifier: MIT

package runtime

import (
	"context"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/governor"
)

// buildLoopConfig assembles the parts of agent.LoopConfig that must be
// IDENTICAL for every governed run — root (RunWith) or delegated
// (executeSubAgent). Before this existed the two sites each hand-built the
// config and had silently diverged: delegated runs got no CostFn (which per
// the LoopConfig contract makes MaxRunCostMicrocents inert — a named agent's
// per-run spend ceiling was a no-op for delegations), no artifact offload, no
// context compaction, and no tool memo. Run-identity fields (Model chain,
// Agent, Actor, CorrelationID, System, Wake*, Steer, cost ceiling source) stay
// with the caller; everything governance- and capacity-shaped lives here.
func (k *Kernel) buildLoopConfig(runCtx context.Context, corr, model string) agent.LoopConfig {
	// Per-run tool set: forged script tools and live MCP attachments are merged
	// BEFORE the allowlist filter so a restricted run only sees the dynamic
	// tools its allowlist grants (M794/M796).
	runTools := k.mergeMCPTools(k.mergeScriptTools(k.tools))
	runTools = applyAgentToolPolicy(runTools, agentToolPolicyFromCtx(runCtx))
	runTools = applyAgentNoisePolicyToPromptTools(runTools, runCtx)
	if allow, ok := toolsFromCtx(runCtx); ok {
		runTools = filterTools(runTools, allow)
	}
	toolDiscoveryMax := k.cfg.ToolDiscoveryMax
	if v, ok := agentConfigIntOverride(runCtx, "AGEZT_TOOL_DISCOVERY_MAX"); ok {
		toolDiscoveryMax = v
	}
	if toolDiscoveryMax > 0 && len(runTools) > toolDiscoveryMax {
		runTools = withToolSearch(runTools)
	}
	toolSelector := agent.LexicalToolSelector(toolDiscoveryMax)
	if _, ok := runTools[toolSearchName]; ok && toolDiscoveryMax > 0 {
		toolSelector = agent.DeferredLexicalToolSelector(toolDiscoveryMax, []string{toolSearchName})
	}

	maxIter := k.cfg.MaxIter
	if v, ok := agentConfigIntOverride(runCtx, "AGEZT_MAX_ITER"); ok {
		maxIter = v
	}
	maxAutoContinue := k.cfg.MaxAutoContinue
	if v, ok := agentConfigIntOverride(runCtx, "AGEZT_MAX_AUTO_CONTINUE"); ok {
		maxAutoContinue = v
	}
	autoContinueWait := k.cfg.AutoContinueWait
	if v, ok := agentConfigDurationOverride(runCtx, "AGEZT_AUTO_CONTINUE_WAIT"); ok {
		autoContinueWait = v
	}
	maxParallelTools := k.cfg.MaxParallelTools
	if v, ok := agentConfigIntOverride(runCtx, "AGEZT_PARALLEL_TOOLS"); ok {
		maxParallelTools = v
	}
	observationDeltas := k.cfg.ObservationDeltas
	if v, ok := agentConfigBoolOverride(runCtx, "AGEZT_OBSERVATION_DELTAS"); ok {
		observationDeltas = v
	}

	// Context budget (SPEC-10 §3): an explicit budget wins; otherwise, in auto
	// mode, derive one from the resolved model's catalog context window. An
	// unknown/empty model leaves compaction off (0). Catalog reads go through
	// the locked accessor — ReloadCatalog swaps the field concurrently (M477).
	ctxBudget := k.cfg.ContextBudget
	if v, ok := agentConfigIntOverride(runCtx, "AGEZT_CONTEXT_BUDGET"); ok {
		ctxBudget = v
	}
	if ctxBudget == 0 && k.cfg.ContextBudgetAuto {
		if cat := k.Catalog(); cat != nil {
			if _, m := cat.FindModel(model); m != nil {
				ctxBudget = agent.AutoContextBudgetChars(m.Limit.Context)
			}
		}
	}

	// Abstractive summary of elided tool outputs (M398): opt-in, and only worth
	// wiring when compaction is actually active for this run.
	var summarizeElided func(context.Context, string) (string, error)
	if k.cfg.ContextSummarize && (ctxBudget > 0 || k.cfg.ContextBudgetAuto) {
		maxTok := elidedSummaryMaxTokens
		// A reasoning model needs chain-of-thought headroom or the summary
		// comes back empty (M926).
		if cat := k.Catalog(); cat != nil {
			if _, m := cat.FindModel(model); m != nil && m.Reasoning {
				maxTok = elidedSummaryReasoningMaxTokens
			}
		}
		summarizeElided = makeElidedSummarizer(k.cfg.Provider, model, corr, maxTok)
	}

	return agent.LoopConfig{
		Provider:             k.cfg.Provider,
		Tools:                runTools,
		Bus:                  k.bus,
		Model:                model,
		MaxIter:              maxIter,
		MaxAutoContinue:      maxAutoContinue,  // M833: autonomous continue past MaxIter
		AutoContinueWait:     autoContinueWait, // M833
		ToolTimeout:          k.cfg.ToolTimeout,
		MaxParallelTools:     maxParallelTools, // M880: in-turn parallel tool dispatch
		Policy:               k.policyHook,
		ToolSelector:         toolSelector,
		ToolResultHook:       k.completeAgentNoiseNotify,
		ObservationDeltas:    observationDeltas,
		ToolMemo:             agent.NewToolMemo(agent.DefaultToolMemoTTL, agent.DefaultToolMemoMaxEntries),
		CostFn:               governor.CostMicrocents, // required or MaxRunCostMicrocents is inert
		Artifacts:            k.artifacts,             // M390: offload oversized tool outputs
		ArtifactThreshold:    k.cfg.ArtifactThreshold,
		ContextBudget:        ctxBudget,                 // M393/M394 (SPEC-10 §3)
		ContextProtectFirst:  k.cfg.ContextProtectFirst, // M395
		SummarizeElided:      summarizeElided,           // M398
		ContextRescueMarkers: []string{agent.DefaultContextRescueMarker},
	}
}
