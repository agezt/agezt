// SPDX-License-Identifier: MIT

package loopcfg

import (
	"context"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolphaseapi"
)

// Config is the run's EFFECTIVE configuration, projected into the knobs the
// loop needs. The daemon reads its own effective configuration (the
// daemon-wide config plus the operator's live edits plus this agent's own
// overrides) and projects it here for every run — the projection is a
// PARAMETER of the build, not a snapshot taken once, because a named agent's
// overrides apply per run.
type Config struct {
	// Provider is the run's model provider.
	Provider llm.Provider
	// MaxIter is the iteration ceiling.
	MaxIter int
	// MaxAutoContinue is how many times the loop may continue past MaxIter.
	MaxAutoContinue int
	// AutoContinueWait is the breather between auto-continues.
	AutoContinueWait time.Duration
	// ToolTimeout bounds one tool call.
	ToolTimeout time.Duration
	// MaxParallelTools is the in-turn parallel tool dispatch bound.
	MaxParallelTools int
	// ObservationDeltas turns on observation deltas.
	ObservationDeltas bool
	// ToolDiscoveryMax is the tool count above which the catalog is deferred
	// behind a search. 0 disables discovery.
	ToolDiscoveryMax int
	// ContextBudget is the explicit context budget in characters. 0 with
	// ContextBudgetAuto derives one from the model's catalog entry.
	ContextBudget int
	// ContextBudgetAuto derives the budget from the model's catalog entry.
	ContextBudgetAuto bool
	// ContextProtectFirst is the number of leading characters protected from
	// compaction.
	ContextProtectFirst int
	// ContextSummarize enables the abstractive summary of elided tool
	// outputs.
	ContextSummarize bool
	// ArtifactThreshold is the tool-output size above which the output is
	// offloaded to the artifact store.
	ArtifactThreshold int
}

// Deps carries the module's collaborators: the daemon's live state, which the
// rule needs but cannot own.
type Deps struct {
	// Tools returns the run's tool surface, already merged with the daemon's
	// live MCP attachments and forged script tools. The module shapes it
	// further (the allowlist, the agent policy, the noise policy, the
	// deferred discovery).
	Tools func(ctx context.Context) map[string]toolapi.Tool
	// Phases is the batch-aware tool invocation service.
	Phases toolphaseapi.Phases
	// Bus is the daemon's event bus.
	Bus *bus.Bus
	// Policy is the daemon's tool-call policy hook.
	Policy agent.Policy
	// Notify is the daemon's noise-policy result hook.
	Notify func(context.Context, llm.ToolCall, toolapi.Result)
	// Artifacts is the daemon's artifact offload port.
	Artifacts agent.ArtifactPutter
	// Catalog is the daemon's live model catalog accessor: the context window
	// and the reasoning flag come from it. It is a FUNCTION, not a snapshot,
	// because the daemon swaps the catalog on reload (M477) and a captured
	// pointer would freeze the context window of the model that was current
	// when the service was built.
	Catalog func() *catalogapi.Catalog
}

// Service builds the loop configuration.
type Service struct {
	deps Deps
}

// New builds the loop-configuration service.
func New(deps Deps) *Service { return &Service{deps: deps} }

// findModel resolves the model's catalog entry, if the catalog knows it.
func (s *Service) findModel(model string) (*catalogapi.Model, bool) {
	if s.deps.Catalog == nil {
		return nil, false
	}
	cat := s.deps.Catalog()
	if cat == nil {
		return nil, false
	}
	_, m := cat.FindModel(model)
	if m == nil {
		return nil, false
	}
	return m, true
}
