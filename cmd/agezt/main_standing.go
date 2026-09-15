// SPDX-License-Identifier: MIT

// Standing-orders helpers extracted from main.go during Day 211
// god-file refactor (#43, #51). Public API unchanged.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
)

func buildStandingRunner(ctx context.Context, k *kernelruntime.Kernel, brief func(ctx context.Context, kind, text string)) (string, func(id string) bool) {
	fire := func(fctx context.Context, o standing.Order, subject string, triggerPayload map[string]any) {
		// A fired order launches a full governed run (provider/tool/plugin code) and
		// then briefs over the network — any of which can panic. This goroutine is
		// dispatched with a bare `go fire(...)` by the runner/cron loop, so its own
		// recover() does NOT cover us; without this defer a single bad run would take
		// down the whole daemon. Contain the panic to this order and journal it as a
		// standing.error so it stays diagnosable (`agt journal`).
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "standing order %q panicked: %v\n", o.Name, r)
				_, _ = k.Bus().Publish(event.Spec{
					Subject: "standing." + o.ID,
					Kind:    event.KindStandingError,
					Actor:   "standing",
					Payload: map[string]any{"id": o.ID, "name": o.Name, "trigger_subject": subject, "panic": fmt.Sprintf("%v", r)},
				})
			}
		}()
		corr := k.NewCorrelation()
		intent := strings.TrimSpace(o.Plan)
		if intent == "" {
			intent = o.Name
		}
		// Run AS a named agent (M790): resolve the order's roster profile up
		// front — an unknown or paused agent journals a standing.error instead
		// of silently running as the default identity (mirrors `agt run --agent`).
		var prof *roster.Profile
		if slug := strings.TrimSpace(o.Agent); slug != "" {
			p, ok := k.Roster().Get(slug)
			if !ok || p.Retired || !p.Enabled {
				reason := "unknown agent " + slug
				if ok {
					reason = "agent " + p.Slug + " is paused"
					if p.Retired {
						reason = "agent " + p.Slug + " is retired — revive it first"
					}
				}
				_, _ = k.Bus().Publish(event.Spec{
					Subject: "standing." + o.ID,
					Kind:    event.KindStandingError,
					Actor:   "standing",
					Payload: map[string]any{"id": o.ID, "name": o.Name, "trigger_subject": subject, "agent": slug, "reason": reason},
				})
				return
			}
			if !p.AllowsDirectCall() {
				manager := strings.TrimSpace(p.ParentAgent)
				if manager == "" {
					manager = strings.TrimSpace(p.OwnerAgent)
				}
				hint := "route the work through its parent/owner agent"
				if manager != "" {
					hint = "wake " + manager + " or delegate through it"
				}
				_, _ = k.Bus().Publish(event.Spec{
					Subject: "standing." + o.ID,
					Kind:    event.KindStandingError,
					Actor:   "standing",
					Payload: map[string]any{
						"id":              o.ID,
						"name":            o.Name,
						"trigger_subject": subject,
						"agent":           p.Slug,
						"reason":          "agent " + p.Slug + " is a managed sub-agent and cannot be fired directly by a standing order; " + hint,
					},
				})
				return
			}
			prof = &p
		}
		// Ground the run in the order's scope (SPEC-16 §4): the agent is told what
		// this standing order watches.
		intent = standing.ScopedIntent(o, intent)
		intent = standing.TriggeredIntent(intent, subject, triggerPayload)
		firedPayload := map[string]any{"id": o.ID, "name": o.Name, "trigger_subject": subject, "intent": intent}
		if len(triggerPayload) > 0 {
			firedPayload["trigger_payload"] = triggerPayload
		}
		if prof != nil {
			firedPayload["agent"] = prof.Slug // who this firing runs AS (M790)
			// Carry the same autonomy runbook schedule.fired does, so a standing
			// wake is traceable as event payload -> status -> detail -> activity.
			firedPayload["autonomy_runbook"] = agentAutonomyRunbookPayload(*prof)
		}
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "standing." + o.ID,
			Kind:          event.KindStandingFired,
			Actor:         "standing",
			CorrelationID: corr,
			Payload:       firedPayload,
		})
		rctx := fctx
		if prof != nil {
			// Soul → system, model + fallbacks → chain, memory scope (M790).
			rctx = kernelruntime.WithAgentProfile(rctx, *prof)
			// The profile's per-run ceiling is the DEFAULT; the order's own wins.
			if o.Initiative.BudgetPerRunMc <= 0 && prof.MaxCostMc > 0 {
				rctx = kernelruntime.WithMaxCost(rctx, prof.MaxCostMc)
			}
		}
		rctx = kernelruntime.WithWakeContext(rctx, kernelruntime.WakeContext{
			Source:         "standing",
			Reason:         "event",
			StandingID:     o.ID,
			StandingName:   o.Name,
			TriggerSubject: subject,
		})
		if o.Initiative.BudgetPerRunMc > 0 {
			rctx = kernelruntime.WithMaxCost(rctx, o.Initiative.BudgetPerRunMc)
		}
		// Cap autonomous action (SPEC-16 §4, M999): the effective ceiling is the MORE
		// restrictive of the order's max_trust and the trust implied by its initiative
		// MODE (inform_only→L0/no-tools, ask→L1/approval-each). A normally auto-allowed
		// tool is downgraded to Ask/Deny within this run. Before M999 the mode was
		// stored but never enforced — only max_trust gated; now the mode is a real dial.
		if lvl, ok := standingTrustCeiling(o.Initiative); ok {
			rctx = kernelruntime.WithTrustCeiling(rctx, lvl)
		}
		// Do-it-for-sure firings (M655): when the order carries an assure budget,
		// each firing runs-verifies-retries until the plan is judged complete (or
		// the budget is spent) — symmetric with assured schedules, so an
		// event/cron-triggered order actually gets its task done.
		var answer string
		if o.Assure > 0 {
			answer, _, _ = k.RunAssured(rctx, corr, intent, o.Assure)
		} else if prof != nil && prof.RetryPolicy != nil && prof.RetryPolicy.MaxAttempts > 1 {
			answer, _ = k.RunWithRetry(rctx, corr, intent, *prof.RetryPolicy)
		} else {
			answer, _ = k.RunWith(rctx, corr, intent)
		}
		// Brief the result to the order's configured channel (SPEC-16 §4 briefing).
		if text, ok := standing.BriefText(o, answer); ok && brief != nil {
			brief(fctx, o.BriefingChan, text)
		}
	}
	// fireNow launches an order on demand (M765), through the same governed fire path
	// the triggers use — so "run now" from the console/CLI behaves exactly like a real
	// firing. Returns false for an unknown id. Works even if auto-triggers are off.
	fireNow := func(id string) bool {
		o, ok := k.Standing().Get(id)
		if !ok {
			return false
		}
		go fire(ctx, o, "manual", nil)
		return true
	}

	evOK := standing.StartRunner(ctx, k.Bus(), k.Standing(), standing.RunnerConfig{}, fire)
	cronOK := standing.StartCron(ctx, k.Standing(), nil, fire)
	if !evOK && !cronOK {
		return "disabled", fireNow
	}
	return fmt.Sprintf("on (event + cron triggers; %d order(s) defined)", k.Standing().Count()), fireNow
}
