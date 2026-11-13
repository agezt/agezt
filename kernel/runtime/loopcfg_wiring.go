// SPDX-License-Identifier: MIT

package runtime

// Provenance: SPDX-License-Identifier: MIT The daemon's WIRING for the runs
//             module's loop configuration (kernel/modules/runs/loopcfg,
//             W3.76g; the DOOR moved to the module in W3.77). The RULE — what a
//             governed run's loop looks like — is the module's; the daemon
//             keeps the STATE it reads: its effective configuration, its live
//             tool surface, its tool-phase service, its bus, its policy hook,
//             its artifact offload port and its model catalog.
//
// The daemon holds NO door over the rule any more: the run engine (which
// lives inside the module) and the delegation engine (which lives next door
// in the same module) hold the very same service and ask it directly. The
// daemon only WIRES it — it never calls the rule itself.
//
// The file is named loopcfg_wiring.go: it used to be loopcfg_doors.go, and the
// door it kept (BuildLoopConfig) left with the module. The runctx ratchet
// from W3.76c watches the names the run context left behind.

import (
	"context"

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
		// The knobs are the daemon's own state, asked PER RUN (W3.77): the
		// effective configuration is live, and a named agent's own overrides
		// apply to its run and to no other.
		Config: func(ctx context.Context) loopcfg.Config {
			return loopcfgConfig(k.effectiveConfig(ctx))
		},
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
