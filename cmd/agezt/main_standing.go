// SPDX-License-Identifier: MIT

// Standing-orders helpers extracted from main.go during Day 211
// god-file refactor (#43). Public API unchanged.
package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/resume"
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

// buildResumer re-dispatches runs that were interrupted by a restart (M1002). On
// boot, any ticket left in the resume store is a run that did not finish cleanly
// (a clean finish deletes its ticket) — self-update, stop/start, or hard kill.
// Each resumable ticket is re-dispatched through the SAME governed entry point it
// used, seeded with its saved conversation so a Kind=run continues where it left
// off. The resumer owns the ticket lifecycle for the runs it launches: it marks
// them owned (so the inner RunWith/wrapper neither recreates the ticket — which
// would reset the crash-loop counter — nor deletes it) and finalizes on return.
//
// Safety rails: a ticket that used an un-reconstructable per-run override, whose
// agent is gone, or that has exceeded AGEZT_RESUME_MAX_ATTEMPTS is quarantined
// (moved aside for postmortem) rather than re-dispatched, so a poison run can
// never wedge boot. The attempt counter is incremented and fsynced BEFORE
// dispatch, so a resume that hard-crashes the daemon still records the attempt.
func buildResumer(ctx context.Context, k *kernelruntime.Kernel) string {
	store := k.ResumeStore()
	if store == nil {
		return "disabled"
	}
	tickets, err := store.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resume: list tickets: %v\n", err)
		return "error"
	}
	if len(tickets) == 0 {
		return "none pending"
	}
	maxAttempts := 3
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "RESUME_MAX_ATTEMPTS")); v != "" {
		if n, aerr := strconv.Atoi(v); aerr == nil && n > 0 {
			maxAttempts = n
		}
	}

	quarantine := func(t *resume.Ticket, reason string) {
		_ = store.Quarantine(t.Corr)
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "run.resume.quarantined",
			Kind:          event.KindAnomalyDetected,
			Actor:         "resume",
			CorrelationID: t.Corr,
			Payload:       map[string]any{"corr": t.Corr, "agent": t.AgentSlug, "attempts": t.Attempts, "reason": reason, "severity": "warning"},
		})
	}

	resumed := 0
	for _, t := range tickets {
		if !t.Resumable {
			quarantine(t, "non-resumable (per-run override cannot be reconstructed)")
			continue
		}
		if t.Attempts >= maxAttempts {
			quarantine(t, fmt.Sprintf("exceeded resume attempt cap (%d)", maxAttempts))
			continue
		}
		// Resolve the agent this run ran AS. If it named an agent that is now gone
		// or disabled, quarantine rather than silently resume under the default
		// identity (which would run with the wrong persona/tools/ceilings).
		var prof *roster.Profile
		if slug := strings.TrimSpace(t.AgentSlug); slug != "" {
			p, ok := k.Roster().Get(slug)
			if !ok || p.Retired || !p.Enabled {
				quarantine(t, "agent "+slug+" is gone, retired, or disabled")
				continue
			}
			prof = &p
		}
		// Record the attempt durably BEFORE dispatch: a resume that hard-crashes the
		// daemon must still have counted, or the crash-loop guard never trips.
		if _, aerr := store.IncrementAttempt(t.Corr); aerr != nil {
			fmt.Fprintf(os.Stderr, "resume: increment attempt for %s: %v\n", t.Corr, aerr)
			continue
		}

		// Rebuild the run context from the ticket's resolved fields. The resumer
		// OWNS the ticket, so mark it owned to stop the inner RunWith recreating it.
		rctx := kernelruntime.WithResumeOwned(ctx, t.Kind)
		if prof != nil {
			rctx = kernelruntime.WithAgentProfile(rctx, *prof)
		}
		rctx = kernelruntime.WithWakeContext(rctx, kernelruntime.WakeContext{
			Source:         strutil.FirstNonEmpty(t.WakeSource, "resume"),
			Reason:         "resumed",
			ScheduleID:     t.WakeScheduleID,
			StandingID:     t.WakeStandingID,
			StandingName:   t.WakeStandingName,
			TriggerSubject: t.WakeTriggerSubject,
		})
		// Governance invariant (M1002): a tightened trust ceiling MUST be re-applied
		// so a resumed run never silently regains authority.
		if t.TrustCeiling != nil {
			rctx = kernelruntime.WithTrustCeiling(rctx, edict.TrustLevel(*t.TrustCeiling))
		}
		if t.MaxCostMc > 0 {
			rctx = kernelruntime.WithMaxCost(rctx, t.MaxCostMc)
		}
		if t.RunTimeoutMs > 0 {
			rctx = kernelruntime.WithRunTimeout(rctx, time.Duration(t.RunTimeoutMs)*time.Millisecond)
		}
		// Continue the interrupted conversation for a message-bearing run; assured/
		// retry re-run from the top (cheap re-verification), so they carry no seed.
		if t.Kind == resume.KindRun && len(t.Messages) > 0 {
			rctx = kernelruntime.WithResumeSeed(rctx, t.Messages, t.Iter)
		}

		resumed++
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "run.resumed",
			Kind:          event.KindInfo,
			Actor:         "resume",
			CorrelationID: t.Corr,
			Payload:       map[string]any{"corr": t.Corr, "agent": t.AgentSlug, "kind": t.Kind, "iter": t.Iter, "attempt": t.Attempts + 1, "seeded": len(t.Messages) > 0},
		})
		go func() {
			// Contain a panic to this run — a bad resumed run must not take down boot.
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(os.Stderr, "resume: run %s panicked: %v\n", t.Corr, r)
					k.ResumeFinalize(t.Corr, fmt.Errorf("resume panic: %v", r))
				}
			}()
			var rerr error
			switch t.Kind {
			case resume.KindAssured:
				budget := t.AssureBudget
				if budget <= 0 {
					budget = 1
				}
				_, _, rerr = k.RunAssured(rctx, t.Corr, t.Intent, budget)
			case resume.KindRetry:
				if prof != nil && prof.RetryPolicy != nil && prof.RetryPolicy.MaxAttempts > 1 {
					_, rerr = k.RunWithRetry(rctx, t.Corr, t.Intent, *prof.RetryPolicy)
				} else {
					_, rerr = k.RunWith(rctx, t.Corr, t.Intent)
				}
			default:
				_, rerr = k.RunWith(rctx, t.Corr, t.Intent)
			}
			// The resumer owns the ticket: clear it on a clean/failed terminal, keep
			// it if a NEW shutdown interrupted the resumed run.
			k.ResumeFinalize(t.Corr, rerr)
		}()
	}
	quarantined := len(tickets) - resumed
	if resumed == 0 {
		return fmt.Sprintf("none resumable (%d quarantined)", quarantined)
	}
	if quarantined > 0 {
		return fmt.Sprintf("%d run(s) resumed, %d quarantined", resumed, quarantined)
	}
	return fmt.Sprintf("%d run(s) resumed", resumed)
}

// delegationBanner renders the active multi-agent delegation ceilings (M58) for
// the boot banner — the same effective caps `agt status` reports (M49), so the
// governance is visible at startup, not only on demand. "off" when the delegate
// tool is disabled; 0 fan-out / spend render as "unbounded".
func delegationBanner(k *kernelruntime.Kernel) string {
	l := k.SubAgentLimits()
	if !l.Enabled {
		return "off (AGEZT_SUBAGENT=off)"
	}
	fanout := "unbounded"
	if l.MaxFanout > 0 {
		fanout = fmt.Sprintf("≤%d", l.MaxFanout)
	}
	spend := "unbounded"
	if l.MaxSpendMicrocents > 0 {
		spend = fmt.Sprintf("$%.4f", float64(l.MaxSpendMicrocents)/1e9)
	}
	total := "unbounded"
	if l.MaxTotal > 0 {
		total = fmt.Sprintf("≤%d", l.MaxTotal)
	}
	return fmt.Sprintf("depth≤%d, fan-out %s, total %s, spend %s", l.MaxDepth, fanout, total, spend)
}

// buildCadence starts the scheduled-intents resident when AGEZT_SCHEDULE is set.
// Each firing journals a schedule.fired event (carrying the run's correlation so
// `agt why` links the schedule to the run) and then runs the intent through the
// normal governed loop. Returns the banner description; "" only when the env var
// is unset and the store is empty.
// deliverScheduled sends a scheduled run's answer to every configured channel
// recipient (M152), prefixed with the schedule id so the operator knows which job
// produced it. Empty answers are skipped. Returns the number of successful
// deliveries (for testing). Channel kinds are iterated in sorted order for
// deterministic delivery.
func deliverScheduled(ctx context.Context, send func(context.Context, string, string, string) error, targets map[string][]string, id, answer string) int {
	if strings.TrimSpace(answer) == "" || send == nil {
		return 0
	}
	text := "[scheduled: " + id + "]\n" + answer
	kinds := make([]string, 0, len(targets))
	for k := range targets {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	sent := 0
	for _, kind := range kinds {
		for _, recip := range targets[kind] {
			if send(ctx, kind, recip, text) == nil {
				sent++
			}
		}
	}
	return sent
}
