// SPDX-License-Identifier: MIT

// Resume-on-boot helpers extracted from main_standing.go during Day 211
// god-file refactor (#51). Public API unchanged.
package main

import (
	"context"
	"fmt"
	"os"
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
)

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
