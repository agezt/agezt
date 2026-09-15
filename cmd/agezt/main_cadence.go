// SPDX-License-Identifier: MIT

// Cadence/scheduled-task helpers extracted from main.go during Day 211
// god-file refactor (#43). Public API unchanged.
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

func scheduledRunContext(runCtx context.Context, model string, prof *roster.Profile) context.Context {
	mctx := runCtx
	if prof != nil {
		mctx = kernelruntime.WithAgentProfile(mctx, *prof)
		if prof.MaxCostMc > 0 {
			mctx = kernelruntime.WithMaxCost(mctx, prof.MaxCostMc)
		}
	}
	model = strings.TrimSpace(model)
	if model != "" {
		mctx = kernelruntime.WithModel(mctx, model)
		mctx = kernelruntime.WithModelChain(mctx, []string{model})
	}
	return mctx
}

func scheduleFiredEventPayload(id, intent, model string, ent cadence.Entry, profs ...*roster.Profile) map[string]any {
	payload := map[string]any{
		"schedule_id": id,
		"intent":      intent,
		"model":       model,
		"target":      ent.Target,
		"agent":       ent.Agent,
	}
	if len(profs) > 0 && profs[0] != nil {
		payload["autonomy_runbook"] = agentAutonomyRunbookPayload(*profs[0])
	}
	switch ent.Target {
	case cadence.TargetWorkflow:
		payload["workflow"] = ent.Workflow
		payload["executor"] = "workflow"
		payload["uses_llm"] = true
	case cadence.TargetSystemTask:
		payload["system_task"] = ent.SystemTask
		if info, ok := systemtasks.Info(ent.SystemTask); ok {
			payload["executor"] = info.Executor
			payload["category"] = info.Category
			payload["effect_class"] = info.EffectClass
			payload["uses_llm"] = info.UsesLLM
		} else {
			payload["executor"] = "daemon"
			payload["uses_llm"] = false
		}
	case cadence.TargetTool:
		payload["tool"] = ent.Tool
		payload["executor"] = "tool"
		payload["uses_llm"] = false
	default:
		payload["executor"] = "agent"
		payload["uses_llm"] = true
	}
	return payload
}

// agentAutonomyRunbookPayload attaches the machine-readable wake contract to
// autonomous wake evidence (schedule.fired and standing.fired) when the firing
// resolves a concrete agent profile. It delegates to the canonical roster builder
// so operator, schedule, standing, and delegated wakes all carry an
// identically-shaped runbook through the journal.
func agentAutonomyRunbookPayload(p roster.Profile) map[string]any {
	return roster.AutonomyRunbook(p)
}

func runScheduledTrackedTarget(ctx context.Context, k *kernelruntime.Kernel, corr string, ent cadence.Entry, intent string, run func(context.Context) (string, error)) error {
	sf := schedulePayloadForEntry(ent, intent)
	action := controlplaneScheduleAction(sf)
	receivedPayload := map[string]any{
		"schedule_id":      ent.ID,
		"intent":           action,
		"scheduled_intent": intent,
		"target":           ent.Target,
		"agent":            ent.Agent,
		"workflow":         ent.Workflow,
		"system_task":      ent.SystemTask,
		"tool":             ent.Tool,
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.task",
		Kind:          event.KindTaskReceived,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload:       receivedPayload,
	})

	answer, err := run(ctx)
	if err != nil {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "schedule.task",
			Kind:          event.KindTaskFailed,
			Actor:         "schedule",
			CorrelationID: corr,
			Payload: map[string]any{
				"schedule_id": ent.ID,
				"target":      ent.Target,
				"reason":      "error",
				"error":       err.Error(),
			},
		})
		return err
	}
	k.CompleteAgentLifecycle(ctx, corr)
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.task",
		Kind:          event.KindTaskCompleted,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload: map[string]any{
			"schedule_id": ent.ID,
			"target":      ent.Target,
			"answer":      truncateScheduledAnswer(answer),
			"iters":       0,
		},
	})
	return nil
}

type scheduledTargetPayload struct {
	ScheduleID string
	Intent     string
	Target     string
	Agent      string
	Workflow   string
	SystemTask string
	Tool       string
}

func schedulePayloadForEntry(ent cadence.Entry, intent string) scheduledTargetPayload {
	return scheduledTargetPayload{
		ScheduleID: ent.ID,
		Intent:     intent,
		Target:     ent.Target,
		Agent:      ent.Agent,
		Workflow:   ent.Workflow,
		SystemTask: ent.SystemTask,
		Tool:       ent.Tool,
	}
}

func controlplaneScheduleAction(p scheduledTargetPayload) string {
	switch p.Target {
	case "workflow":
		if p.Workflow != "" {
			return "run workflow " + p.Workflow
		}
	case "system_task":
		if p.SystemTask != "" {
			return "run system task " + p.SystemTask
		}
	case "tool":
		if p.Tool != "" {
			return "run tool " + p.Tool
		}
	}
	if p.Agent != "" && p.Intent != "" {
		return "wake " + p.Agent + ": " + p.Intent
	}
	if p.Intent != "" {
		return p.Intent
	}
	return p.ScheduleID
}

func truncateScheduledAnswer(s string) string {
	const max = 4096
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}

// startUpdateChecker runs the background update checker goroutine (M860).
// It fires on the configured CheckInterval. When an update is found, it
// auto-applies after the daemon goes idle (drain). The journal receives an
// event so the update is auditable. The watchdog is signalled to restart
// with the new binary.
// startUpdateChecker → boot_ops.go

// bootStep is one step of runDaemon's tool late-bind + seed/banner phase
// (Phase 2.6 3a). run performs the step: it may print its own (multi-line or
// conditional) banner output directly, or return a single ready-to-print banner
// line as desc ("" = nothing to print). A returned error aborts the daemon only
// when fatal is set; best-effort steps surface their own failures inside run
// and return nil. The table keeps the sequence scannable and gives a single
// place to time steps later.
