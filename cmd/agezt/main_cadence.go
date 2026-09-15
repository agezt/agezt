// SPDX-License-Identifier: MIT

// Cadence/scheduled-task helpers extracted from main.go during Day 211
// god-file refactor (#43, #52). Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/cadence/systemtasks"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

func buildCadence(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer, onAnswer func(ctx context.Context, id, answer string)) string {
	store := k.Schedules()
	if store == nil {
		return ""
	}
	// Sync AGEZT_SCHEDULE env jobs into the store (idempotent: replaces the
	// previous env-sourced entries, leaves operator-managed ones untouched).
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SCHEDULE")); spec != "" {
		jobs, err := cadence.ParseJobs(spec)
		if err != nil {
			return "disabled (" + err.Error() + ")"
		}
		if err := store.SyncEnv(jobs, time.Now()); err != nil {
			return "disabled (" + err.Error() + ")"
		}
	} else {
		_ = store.SyncEnv(nil, time.Now()) // env cleared → drop stale env entries
	}
	// The engine always runs (so operator-added schedules fire even with no env
	// spec). With no entries it simply ticks idly.
	run := func(runCtx context.Context, id, intent, model string) error {
		corr := k.NewCorrelation()
		ent, ok := store.Get(id)
		if !ok {
			return fmt.Errorf("schedule %s: not found", id)
		}
		var prof *roster.Profile
		if slug := strings.TrimSpace(ent.Agent); slug != "" {
			p, ok := k.Roster().Get(slug)
			if !ok {
				return fmt.Errorf("schedule %s: unknown agent %s", id, slug)
			}
			if p.Retired {
				return fmt.Errorf("schedule %s: agent %s is retired — revive it first", id, p.Slug)
			}
			if !p.Enabled {
				return fmt.Errorf("schedule %s: agent %s is paused", id, p.Slug)
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
				return fmt.Errorf("schedule %s: agent %s is a managed sub-agent and cannot be scheduled directly; %s", id, p.Slug, hint)
			}
			prof = &p
		}
		mctx := scheduledRunContext(runCtx, model, prof)
		mctx = kernelruntime.WithWakeContext(mctx, kernelruntime.WakeContext{
			Source:     "schedule",
			Reason:     ent.Target,
			ScheduleID: id,
		})
		if ent.Target == cadence.TargetWorkflow {
			var payload any
			if len(ent.Payload) > 0 {
				if err := json.Unmarshal(ent.Payload, &payload); err != nil {
					return fmt.Errorf("schedule %s: workflow payload: %w", id, err)
				}
			}
			_, _ = k.Bus().Publish(event.Spec{
				Subject:       "schedule.fired",
				Kind:          event.KindScheduleFired,
				Actor:         "schedule",
				CorrelationID: corr,
				Payload:       scheduleFiredEventPayload(id, intent, model, ent, prof),
			})
			return runScheduledTrackedTarget(mctx, k, corr, ent, intent, func(ctx context.Context) (string, error) {
				res, err := k.RunWorkflow(ctx, corr, ent.Workflow, payload)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("workflow %s completed (%d nodes)", ent.Workflow, len(res.Executed)), nil
			})
		}
		if ent.Target == cadence.TargetSystemTask {
			_, _ = k.Bus().Publish(event.Spec{
				Subject:       "schedule.fired",
				Kind:          event.KindScheduleFired,
				Actor:         "schedule",
				CorrelationID: corr,
				Payload:       scheduleFiredEventPayload(id, intent, model, ent, prof),
			})
			return runScheduledTrackedTarget(mctx, k, corr, ent, intent, func(ctx context.Context) (string, error) {
				if err := systemtasks.Run(ctx, k, corr, id, ent.SystemTask); err != nil {
					return "", err
				}
				return "system task " + ent.SystemTask + " completed", nil
			})
		}
		if ent.Target == cadence.TargetTool {
			payload := ent.Payload
			if len(payload) == 0 {
				payload = json.RawMessage(`{}`)
			}
			_, _ = k.Bus().Publish(event.Spec{
				Subject:       "schedule.fired",
				Kind:          event.KindScheduleFired,
				Actor:         "schedule",
				CorrelationID: corr,
				Payload:       scheduleFiredEventPayload(id, intent, model, ent, prof),
			})
			return runScheduledTrackedTarget(mctx, k, corr, ent, intent, func(ctx context.Context) (string, error) {
				res, err := k.RunTool(ctx, corr, "schedule-"+id, ent.Tool, payload)
				if err != nil {
					return "", err
				}
				if res.IsError {
					return "", fmt.Errorf("tool %s failed: %s", ent.Tool, res.Output)
				}
				if strings.TrimSpace(res.Output) == "" {
					return "tool " + ent.Tool + " completed", nil
				}
				return res.Output, nil
			})
		}
		if ent.Target != cadence.TargetIntent {
			return fmt.Errorf("schedule %s: unknown target %q", id, ent.Target)
		}
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "schedule.fired",
			Kind:          event.KindScheduleFired,
			Actor:         "schedule",
			CorrelationID: corr,
			// schedule_id (M55) attributes the firing to its schedule entry, so
			// `agt schedule fires --id <sched>` can filter and `agt schedule list`
			// can show a schedule's last outcome.
			Payload: scheduleFiredEventPayload(id, intent, model, ent, prof),
		})
		// Do-it-for-sure firings (M654): when the entry carries an assure budget,
		// each firing runs-verifies-retries until the task is judged complete (or
		// the budget is spent), so an unattended schedule/continuous loop actually
		// gets its task done rather than firing once and hoping.
		var ans string
		var err error
		if ent.Assure > 0 {
			ans, _, err = k.RunAssured(mctx, corr, intent, ent.Assure)
		} else if prof != nil && prof.RetryPolicy != nil && prof.RetryPolicy.MaxAttempts > 1 {
			ans, err = k.RunWithRetry(mctx, corr, intent, *prof.RetryPolicy)
		} else {
			ans, err = k.RunWith(mctx, corr, intent)
		}
		// Deliver the scheduled run's answer to the operator's channels when
		// AGEZT_SCHEDULE_NOTIFY is on (M152): a proactive morning digest reaches
		// you instead of sitting silently in the journal. Only on success with a
		// non-empty answer; off entirely when onAnswer is nil.
		if err == nil && onAnswer != nil {
			onAnswer(runCtx, id, ans)
		}
		return err
	}
	eng := cadence.NewEngine(store, run, 0, stdout)
	// Injection tripwire (M886): a suspicious scheduled intent journals an
	// anomaly.detected warning on every firing (it still fires — default-allow).
	eng.Bus = k.Bus()
	// Backstop each firing with a deadline so a single hung run can't permanently
	// stall its schedule (its in-flight guard would never clear). Default 1h is
	// generous for any reasonable agentic run; AGEZT_SCHEDULE_RUN_TIMEOUT overrides
	// (a value of 0/"off" disables the backstop). Must be set before Start.
	eng.RunTimeout = time.Hour
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SCHEDULE_RUN_TIMEOUT")); v != "" {
		if strings.EqualFold(v, "off") || v == "0" {
			eng.RunTimeout = 0
		} else if d, err := time.ParseDuration(v); err == nil && d > 0 {
			eng.RunTimeout = d
		}
	}
	k.SetScheduleEngine(eng)
	eng.Start(ctx)

	entries := store.List()
	if len(entries) == 0 {
		return "active (no schedules yet — add with `agt schedule add`)"
	}
	return cadence.Describe(entries)
}
