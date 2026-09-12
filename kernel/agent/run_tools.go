// SPDX-License-Identifier: MIT

// Agent run-tools: toolJob + runState types + newRunState + executeToolJobs + invokeToolJob + finalizeToolJobs + policyDecisionPayload.
// Code extracted from run_tools.go during the Day-127 god-file split.
// Public API unchanged.
package agent


import (
	"github.com/agezt/agezt/kernel/event"
)


// The loop's tool turn, extracted from Run (refactor Phase 3.2).
//
// A model turn that requests tools moves through three phases (M880):
//
//	gate     — sequentially, in call order: availability, schema, loop guard,
//	           policy, memo. Journals policy.decision / tool.invoked, so the
//	           audit trail and any HITL approval prompts stay deterministic.
//	execute  — the allowed invocations run, concurrently when the turn carries
//	           more than one (bounded by MaxParallelTools).
//	finalize — classify, journal tool.result, and append the tool messages in
//	           the ORIGINAL call order, so the conversation the model sees is
//	           identical to what a fully sequential build would have produced.
//
// The phases share mutable per-run state (the loop guard's call counts, the
// policy denial tallies, the observation cache, and the untrusted-observation
// taint) which lives in runState below — deliberately together, because the
// prompt-injection causal window is a two-sided invariant (finalize records WHEN
// a directive-like observation arrived; gate decides whether a proposed action is
// still inside the window) and reviewing it used to mean reading 300 interleaved
// lines of the loop body.

// maxToolDenials is how many policy refusals a tool gets before it stops being
// offered to the model at all (M605). A hard-deny counts as all of them at once.
const maxToolDenials = 2

// toolJob is one requested tool call as it moves through the three phases.
// A nil `tool` after gating means the call was refused/short-circuited and its
// synthesized `result` is already final — execute skips it, finalize journals it.
type toolJob struct {
	tc           ToolCall
	tool         Tool // non-nil ⇒ execute runs it
	result       Result
	invokeErr    error
	toolTimedOut bool
	panicked     bool
	memoEligible bool
	memoHit      bool
	memoSource   *toolJob
}

// runState is the mutable state one run carries across its iterations. Hoisted
// out of Run's body so each piece has a name and a documented invariant instead
// of being one of a dozen loop-locals captured by closures.
type runState struct {
	cfg     LoopConfig
	publish func(kind event.Kind, suffix string, payload any) (*event.Event, error)

	// The conversation is deliberately NOT held here: the loop appends to it from
	// several places (steering, the model turn, auto-continue) and compaction
	// rewrites it wholesale, so a second copy in this struct would be a
	// dual-source-of-truth waiting to silently drop a turn. finalizeToolJobs
	// takes it and returns it instead.

	// callCounts counts how many times each exact (tool, input) has been
	// requested in this run, for the M116 loop guard.
	callCounts map[string]int

	// observations holds the last successful output per exact (tool, input) so a
	// repeated observation can be delivered to the model as a delta (CH-04).
	observations map[string]string

	// toolDenials counts policy refusals per tool this run (M605). Once a tool
	// reaches maxToolDenials — or is hard-denied even once — it is dropped from
	// the set offered to the model, so it stops burning iterations (and tokens)
	// requesting a call the policy will always refuse. The M116 guard only
	// catches an identical (tool,input) repeat; this catches the same tool
	// retried with new inputs.
	toolDenials map[string]int

	// --- prompt-injection causal window (M-injection) -----------------------
	// untrustedTaint carries external-observation provenance forward to policy.
	// It is set by the tool-result boundary, NOT by the model, so a hostile web
	// page cannot erase that it was the source of a downstream proposal.
	untrustedTaint UntrustedObservationTaint
	// directiveObsIter is the iteration at which the most recent directive-like
	// untrusted observation arrived (-1 = none yet).
	directiveObsIter int
	// directiveWindow is how many iterations after that observation the gate
	// still treats a proposed action as potentially caused by it.
	directiveWindow int
}

func newRunState(cfg LoopConfig, publish func(event.Kind, string, any) (*event.Event, error)) *runState {
	return &runState{
		cfg:              cfg,
		publish:          publish,
		callCounts:       map[string]int{},
		observations:     map[string]string{},
		toolDenials:      map[string]int{},
		directiveObsIter: -1,
		directiveWindow:  resolveDirectiveWindow(cfg),
	}
}

// directiveActive reports whether a directive-like untrusted observation is
// still close enough (in iterations) to have plausibly caused an action proposed
// at `iter`. Provenance (Sources/Matches) is ALWAYS carried for audit; only this
// gating flag decays — so one suspicious search early in a run stops forcing
// approval on every later action (the run-wide-sticky-taint fix), while an
// action taken right after the injection is still gated.
func (s *runState) directiveActive(iter int) bool {
	return s.directiveObsIter >= 0 && iter-s.directiveObsIter <= s.directiveWindow
}

// offeredTools drops the tools the policy has repeatedly refused this run
// (M605). A no-op until a tool crosses the threshold, so the common case
// allocates nothing and returns the caller's slice unchanged.
func (s *runState) offeredTools(tools []ToolDef) []ToolDef {
	if len(s.toolDenials) == 0 {
		return tools
	}
	filtered := make([]ToolDef, 0, len(tools))
	for _, t := range tools {
		if s.toolDenials[t.Name] >= maxToolDenials {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}

// gateToolCalls is phase 1: decide, in call order, which of this turn's calls
// may run. Every rejection path synthesizes the Result the model will see and
// leaves job.tool nil. Returns an error only for run-terminal conditions (a
// cancelled context, a failed journal publish) — a refused call is not an error.
