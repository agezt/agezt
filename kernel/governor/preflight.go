// SPDX-License-Identifier: MIT

// The pre-dispatch cascade: everything that happens to a completion request
// between "the loop asked for it" and "a provider chain is picked". Split from
// governor.go (refactor Phase 3.6).

package governor

import (
	"fmt"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)

// preflightStep is one step of the pre-dispatch cascade. A step may rewrite the
// request (routing overrides), journal an observation about it (capability
// degradation), or refuse the call by returning an error.
//
// The ORDER of the table below is load-bearing and was previously discoverable
// only by reading a 245-line function top to bottom. Two orderings in particular
// are decisions, not accidents:
//
//   - Down-routing runs BEFORE the strict capability gate, so a request that can
//     be remapped to a tool-capable model is remapped rather than rejected.
//   - The rate gate runs BEFORE the budget gates, so a call blocked on frequency
//     never touches the spend ledger.
type preflightStep struct {
	// Name identifies the step in test failures. It is not journaled — each
	// step publishes its own event with its own subject and kind.
	Name string
	Run  func(g *Governor, req *agent.CompletionRequest) error
}

// preflightSteps is the cascade, in the order it runs.
var preflightSteps = []preflightStep{
	{"task-model-override", (*Governor).applyTaskModelOverride},
	{"capability-down-route", (*Governor).downRouteToolModel},
	{"capability-gate", (*Governor).gateModelToolUse},
	{"strict-tool-args-degradation", (*Governor).noteStrictToolArgsDegradation},
	{"json-mode-degradation", (*Governor).noteJSONModeDegradation},
	{"rate-limit", (*Governor).gateRate},
	{"budgets", (*Governor).gateBudgets},
	{"strict-pricing", (*Governor).gateStrictPricing},
}

// runPreflight walks the cascade, stopping at the first refusal.
func (g *Governor) runPreflight(req *agent.CompletionRequest) error {
	for _, step := range preflightSteps {
		if err := step.Run(g, req); err != nil {
			return err
		}
	}
	return nil
}

// applyTaskModelOverride applies the per-task-type model override (M1.ll). It
// mutates req.Model before anything downstream sees it — providers, audit
// events, and usage accounting all observe the overridden id. The original is
// recoverable from the operator's config (a static mapping), so it is not
// stored.
//
// A configured model CHAIN (M703) supersedes the single override for that task
// type: completeChained has already set req.Model to the chain's current model,
// so this must not clobber it.
func (g *Governor) applyTaskModelOverride(req *agent.CompletionRequest) error {
	if len(g.cfg.TaskModelOverrides) == 0 || req.TaskType == "" {
		return nil
	}
	g.mu.Lock()
	_, hasChain := g.taskModelChains[req.TaskType]
	g.mu.Unlock()
	if hasChain {
		return nil
	}
	if newModel, ok := g.cfg.TaskModelOverrides[req.TaskType]; ok {
		req.Model = newModel
	}
	return nil
}

// downRouteToolModel remaps a tools-bearing request away from a tool-incapable
// model (M37), BEFORE the strict gate would reject it. A miss leaves req.Model
// unchanged so the gate (if on) still rejects. Journaled, so `agt why` explains
// why the served model differs from the requested one.
func (g *Governor) downRouteToolModel(req *agent.CompletionRequest) error {
	if !g.cfg.DownRouteToolModels || g.cfg.ToolCapableAlternative == nil ||
		g.cfg.ModelToolCapable == nil || len(req.Tools) == 0 {
		return nil
	}
	capable, known := g.cfg.ModelToolCapable(req.Model)
	if !known || capable {
		return nil
	}
	alt, found := g.cfg.ToolCapableAlternative(req.Model)
	if !found || alt == req.Model {
		return nil
	}
	g.publishCapability(req, event.KindCapabilityRerouted, map[string]any{
		"from_model":      req.Model,
		"to_model":        alt,
		"capability":      "tool_call",
		"tools_requested": len(req.Tools),
	})
	req.Model = alt
	return nil
}

// gateModelToolUse rejects a tools-bearing request to a model the catalog KNOWS
// lacks tool use (M25), turning a confusing deep upstream failure into a clear,
// journaled error. Checked against the FINAL req.Model, after any override or
// down-route. Unknown models are never blocked; non-tool requests pass.
func (g *Governor) gateModelToolUse(req *agent.CompletionRequest) error {
	if !g.cfg.StrictModelCapabilities || g.cfg.ModelToolCapable == nil || len(req.Tools) == 0 {
		return nil
	}
	if capable, known := g.cfg.ModelToolCapable(req.Model); known && !capable {
		g.publishCapability(req, event.KindCapabilityRejected, map[string]any{
			"model":           req.Model,
			"capability":      "tool_call",
			"tools_requested": len(req.Tools),
		})
		return fmt.Errorf("%w: model %q (request carries %d tool(s))",
			ErrModelLacksToolUse, req.Model, len(req.Tools))
	}
	return nil
}

// noteStrictToolArgsDegradation records that the final model lacks a native
// schema-constrained tool-argument path (CH-02). Tool schemas are always
// validated at the kernel boundary; this notes that invalid tool calls will be
// caught AFTER sampling rather than made impossible by the sampler. Not fatal.
func (g *Governor) noteStrictToolArgsDegradation(req *agent.CompletionRequest) error {
	if len(req.Tools) == 0 || g.cfg.ModelStrictToolArgsNative == nil {
		return nil
	}
	if native, known := g.cfg.ModelStrictToolArgsNative(req.Model); known && !native {
		g.publishCapability(req, event.KindCapabilityDegraded, map[string]any{
			"model":           req.Model,
			"capability":      "strict_tool_args",
			"tools_requested": len(req.Tools),
			"reason":          "model does not advertise schema-constrained tool arguments; relying on kernel validation fallback",
		})
	}
	return nil
}

// noteJSONModeDegradation records that a JSON-mode request is being served by a
// provider family with no native structured-output switch (M381). Unlike tool
// use this is neither fatal nor rerouted — the provider honours it via
// prompt-instructed JSON — but nothing used to record that the native path was
// skipped. Only flagged when the catalog KNOWS the model is non-native.
func (g *Governor) noteJSONModeDegradation(req *agent.CompletionRequest) error {
	if !req.JSONMode || g.cfg.ModelJSONNative == nil {
		return nil
	}
	if native, known := g.cfg.ModelJSONNative(req.Model); known && !native {
		g.publishCapability(req, event.KindCapabilityDegraded, map[string]any{
			"model":      req.Model,
			"capability": "json_mode",
			"reason":     "model family has no native JSON mode; relying on prompt-instructed JSON",
		})
	}
	return nil
}

// gateRate is the frequency check, deliberately ahead of spend: a call blocked
// here never reaches a provider or the budget ledger. Admitted calls are
// counted.
func (g *Governor) gateRate(req *agent.CompletionRequest) error {
	admitted, used, limit := g.admitRate()
	if admitted {
		return nil
	}
	g.publish(event.Spec{
		Subject:       "governor.rate",
		Kind:          event.KindRateLimited,
		Actor:         "governor",
		CorrelationID: req.CorrelationID,
		Payload:       map[string]any{"used": used, "limit_per_min": limit},
	})
	return fmt.Errorf("%w (used=%d, limit=%d/min)", ErrRateLimited, used, limit)
}

// gateStrictPricing refuses a model the governor cannot price, BEFORE spending
// real money on it (M193). Off by default; when on, an unpriced model (catalog
// and fallback table both miss) would otherwise be charged $0 and silently
// bypass every budget. Known-free models (in the table at price 0) pass, and an
// empty req.Model (provider default) is not gated.
func (g *Governor) gateStrictPricing(req *agent.CompletionRequest) error {
	if !g.cfg.StrictPricing || req.Model == "" || modelIsPriced(req.Model) {
		return nil
	}
	g.publish(event.Spec{
		Subject:       "governor.budget",
		Kind:          event.KindBudgetUnpriced,
		Actor:         "governor",
		CorrelationID: req.CorrelationID,
		Payload:       map[string]any{"model": req.Model},
	})
	return fmt.Errorf("%w: %q", ErrUnpricedModel, req.Model)
}

func (g *Governor) publishCapability(req *agent.CompletionRequest, kind event.Kind, payload map[string]any) {
	g.publish(event.Spec{
		Subject:       "governor.capability",
		Kind:          kind,
		Actor:         "governor",
		CorrelationID: req.CorrelationID,
		Payload:       payload,
	})
}
