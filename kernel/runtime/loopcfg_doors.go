// SPDX-License-Identifier: MIT

package runtime

// Provenance: SPDX-License-Identifier: MIT The daemon's doors for the runs
//             module's loop configuration (kernel/modules/runs/loopcfg,
//             W3.76g). The RULE — what a governed run's loop looks like —
//             moved to the module in W3.76g; the daemon keeps the STATE it
//             reads: its effective configuration, its live tool surface, its
//             tool-phase service, its bus, its policy hook, its artifact
//             offload port and its model catalog.
//
// The file is named loopcfg_doors.go: the daemon's loopconfig.go is gone with
// the assembler it held, and the runctx ratchet from W3.76c watches the
// names the run context left behind.

import (
	"context"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/modules/runs/loopcfg"
)

// loopcfgConfig projects the daemon's EFFECTIVE configuration into the knobs
// the loop configuration needs.
//
// The projection is a PARAMETER of the build, not a snapshot taken once: the
// daemon's effective configuration is per-run (the daemon-wide config plus the
// operator's live edits plus this agent's own overrides), so a named agent
// retuning e.g. AGEZT_MAX_ITER is honoured without the module knowing which
// keys exist.
func loopcfgConfig(cfg Config) loopcfg.Config {
	return loopcfg.Config{
		Provider:            cfg.Provider,
		MaxIter:             cfg.MaxIter,
		MaxAutoContinue:     cfg.MaxAutoContinue,
		AutoContinueWait:    cfg.AutoContinueWait,
		ToolTimeout:         cfg.ToolTimeout,
		MaxParallelTools:    cfg.MaxParallelTools,
		ObservationDeltas:   cfg.ObservationDeltas,
		ToolDiscoveryMax:    cfg.ToolDiscoveryMax,
		ContextBudget:       cfg.ContextBudget,
		ContextBudgetAuto:   cfg.ContextBudgetAuto,
		ContextProtectFirst: cfg.ContextProtectFirst,
		ContextSummarize:    cfg.ContextSummarize,
		ArtifactThreshold:   cfg.ArtifactThreshold,
	}
}

// loopcfgDeps wires the module's collaborators: the daemon's live state.
func (k *Kernel) loopcfgDeps() loopcfg.Deps {
	return loopcfg.Deps{
		// Forged script tools and live MCP attachments are merged BEFORE the
		// module's allowlist filter, so a restricted run only sees the
		// dynamic tools its allowlist grants (M794/M796).
		Tools: func(ctx context.Context) map[string]toolapi.Tool {
			return k.mergeMCPTools(k.mergeScriptTools(k.tools))
		},
		Phases:    k.toolPhases,
		Bus:       k.bus,
		Policy:    k.policyHook,
		Notify:    k.completeAgentNoiseNotify,
		Artifacts: k.artifacts,
		Catalog:   k.Catalog,
	}
}

// BuildLoopConfig assembles the parts of agent.LoopConfig that must be
// IDENTICAL for every governed run — root (RunWith) or delegated
// (executeSubAgent).
//
// It is the daemon's DOOR over the runs module's rule (W3.76g): the module
// owns the shape, the daemon owns the state the shape is built from. The
// public surface is unchanged — the runexec sub-package's KernelAPI and the
// delegation engine's BaseLoopConfig door both call this.
func (k *Kernel) BuildLoopConfig(runCtx context.Context, corr, model string) agent.LoopConfig {
	// The service is wired at Open(); the nil fallback keeps the door safe
	// for a caller that reaches it before the daemon was opened.
	svc := k.loopcfg
	if svc == nil {
		svc = loopcfg.New(k.loopcfgDeps())
	}
	return svc.Build(runCtx, corr, model, loopcfgConfig(k.effectiveConfig(runCtx)))
}
