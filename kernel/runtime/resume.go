// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"errors"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/resume"
)

// Durable run resume (M1002). See package kernel/resume for the on-disk ticket
// format and the daemon's boot-time resumer for the replay side. This file is
// the kernel-side integration: it decides which runs get a ticket, keeps each
// run's conversation snapshot fresh, and — when the daemon is going down — flips
// tickets to "suspended" so the resumer re-dispatches them on the next boot
// instead of the work being cancelled and lost.

// ResumeStore returns the durable ticket store (nil when resume is disabled) so
// the daemon's boot-time resumer can enumerate suspended runs. (Named to avoid
// clashing with Resume(), which clears the halt flag.)
func (k *Kernel) ResumeStore() *resume.Store { return k.resume }

// WithResumeSeed carries a suspended run's prior conversation and iteration into
// a resume dispatch, so RunWith continues the interrupted loop instead of
// starting fresh. Set by the daemon's resumer; read only by RunWith.
func WithResumeSeed(ctx context.Context, messages []agent.Message, iter int) context.Context {
	if len(messages) == 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyResumeSeed, resumeSeed{messages: messages, iter: iter})
}

// WithResumeOwned marks that a ticket for this corr already exists and is owned
// by an outer frame — a governed wrapper (RunAssured/RunWithRetry) or the
// resumer. RunWith then neither creates nor deletes the ticket; it only refreshes
// the snapshot for a message-bearing (Kind=run) resume.
func WithResumeOwned(ctx context.Context, kind string) context.Context {
	return context.WithValue(ctx, ctxKeyResumeOwned, kind)
}

type resumeSeed struct {
	messages []agent.Message
	iter     int
}

func resumeSeedFromCtx(ctx context.Context) ([]agent.Message, int, bool) {
	s, ok := ctx.Value(ctxKeyResumeSeed).(resumeSeed)
	if !ok || len(s.messages) == 0 {
		return nil, 0, false
	}
	return s.messages, s.iter, true
}

func resumeOwnedKind(ctx context.Context) (string, bool) {
	kind, ok := ctx.Value(ctxKeyResumeOwned).(string)
	return kind, ok
}

// resumeActive reports whether this ctx describes a run eligible to carry a
// resume ticket: the feature is on and it is a ROOT run. Sub-agents carry a
// ParentCorrelation (and never reach RunWith anyway), so their work is re-driven
// by resuming the root rather than persisted separately.
func (k *Kernel) resumeActive(ctx context.Context) bool {
	return k.resume != nil && k.cfg.ResumeEnabled && wakeContextFromCtx(ctx).ParentCorrelation == ""
}

// claimResumeTicket writes a fresh ticket for a root run and returns a context
// carrying the owned marker plus owns=true — UNLESS an outer frame already owns a
// ticket for this corr, resume is off, or this isn't a root run, in which case it
// returns the ctx unchanged with owns=false. Whoever receives owns=true must call
// finalizeResumeTicket when the run terminates.
func (k *Kernel) claimResumeTicket(ctx context.Context, corr, intent, kind string, assureBudget int) (context.Context, bool) {
	if !k.resumeActive(ctx) {
		return ctx, false
	}
	if _, already := resumeOwnedKind(ctx); already {
		return ctx, false
	}
	t := k.buildResumeTicket(ctx, corr, intent, kind, assureBudget)
	if err := k.resume.Put(t); err != nil {
		// Best-effort: a failed ticket write must never block or fail the run.
		k.publishResumeAnomaly("ticket_write_failed", corr, err)
		return ctx, false
	}
	return WithResumeOwned(ctx, kind), true
}

// buildResumeTicket captures the RESOLVED run context (not the recipe) so a
// resumed run neither loses a tightened trust ceiling nor guesses a cost cap. A
// run carrying a per-run override this can't faithfully reconstruct (ad-hoc
// system prompt, tool allowlist, or model pick) is marked non-resumable — the
// resumer cleans it up rather than re-running it under the wrong constraints.
func (k *Kernel) buildResumeTicket(ctx context.Context, corr, intent, kind string, assureBudget int) *resume.Ticket {
	t := &resume.Ticket{
		Corr:         corr,
		Intent:       intent,
		Kind:         kind,
		AssureBudget: assureBudget,
		AgentSlug:    agentSlugFromCtx(ctx),
		MaxCostMc:    maxCostFromCtx(ctx),
		Resumable:    true,
		Status:       resume.StatusActive,
	}
	if lvl, ok := trustCeilingFromCtx(ctx); ok {
		v := int(lvl)
		t.TrustCeiling = &v
	}
	if d := runTimeoutFromCtx(ctx); d > 0 {
		t.RunTimeoutMs = d.Milliseconds()
	}
	w := wakeContextFromCtx(ctx)
	t.WakeSource = w.Source
	t.WakeReason = w.Reason
	t.WakeScheduleID = w.ScheduleID
	t.WakeStandingID = w.StandingID
	t.WakeStandingName = w.StandingName
	t.WakeTriggerSubject = w.TriggerSubject
	if systemFromCtx(ctx) != "" || modelFromCtx(ctx) != "" {
		t.Resumable = false
	}
	if _, ok := toolsFromCtx(ctx); ok {
		t.Resumable = false
	}
	return t
}

// resumeCheckpointFn returns the closure the agent loop calls at each safe
// iteration boundary to refresh the conversation snapshot. It copies the slice
// (the loop keeps appending after this returns) and is a no-op once the ticket is
// gone (clean completion deleted it). Returns nil when resume is disabled, so the
// loop pays zero overhead.
func (k *Kernel) resumeCheckpointFn(corr string) func(int, []agent.Message) {
	if k.resume == nil {
		return nil
	}
	return func(iter int, messages []agent.Message) {
		snap := append([]agent.Message(nil), messages...)
		if err := k.resume.Snapshot(corr, snap, iter); err != nil {
			k.publishResumeAnomaly("snapshot_failed", corr, err)
		}
	}
}

// finalizeResumeTicket deletes a run's ticket on any terminal outcome EXCEPT an
// interruption by shutdown: a run cancelled while the daemon is suspending is a
// resume candidate, so its ticket is kept for the restart. A clean finish, a
// genuine failure, an operator cancel, or a per-run timeout all delete it — none
// of those should re-run on the next boot.
func (k *Kernel) finalizeResumeTicket(corr string, runErr error) {
	if k.resume == nil {
		return
	}
	if k.suspending.Load() && (errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded)) {
		return // interrupted by shutdown → keep for resume
	}
	if err := k.resume.Delete(corr); err != nil {
		k.publishResumeAnomaly("ticket_delete_failed", corr, err)
	}
}

// ResumeFinalize clears a resumed run's ticket after it terminates, keeping it
// only if the run was interrupted by a subsequent shutdown. The daemon's resumer
// owns the ticket lifecycle for runs it re-dispatches (it marks them owned so the
// inner RunWith/wrapper neither recreates the ticket — which would reset the
// crash-loop attempt counter — nor deletes it), so it calls this when the run
// returns. Same keep/delete rule as finalizeResumeTicket.
func (k *Kernel) ResumeFinalize(corr string, runErr error) { k.finalizeResumeTicket(corr, runErr) }

// Suspend flips every live run's ticket to "suspended" — the durable signal that
// these runs were interrupted by a restart, not completed — and notifies the
// running agents. It latches k.suspending so finalizeResumeTicket keeps (rather
// than deletes) any ticket whose run is then cancelled. Idempotent: a second call
// is a no-op. Returns the number of tickets marked.
//
// Correctness does NOT depend on the agent obeying the notice — a SIGKILL gives
// no grace at all. The continuous per-iteration checkpoint is what actually
// preserves the work; Suspend classifies the shutdown and lets a cooperating loop
// wrap up its current step. Call it BEFORE any cancel on the shutdown path, or a
// cancelled run's ticket is misclassified as operator-cancelled and deleted.
func (k *Kernel) Suspend(reason string) int {
	if k.resume == nil {
		return 0
	}
	if !k.suspending.CompareAndSwap(false, true) {
		return 0
	}
	// Notify each running agent at its next boundary (best-effort).
	k.steersMu.Lock()
	for _, rc := range k.steers {
		rc.inject("The daemon is shutting down and will resume this run on restart. Wrap up the current step if you can; do not start new long-running work.", true)
	}
	k.steersMu.Unlock()

	n, err := k.resume.MarkSuspendedAll()
	if err != nil {
		k.publishResumeAnomaly("mark_suspended_failed", "", err)
	}
	payload := map[string]any{"suspended_runs": n}
	if reason != "" {
		payload["reason"] = reason
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "run.suspending",
		Kind:    event.KindInfo,
		Actor:   "kernel",
		Payload: payload,
	})
	return n
}

func (k *Kernel) publishResumeAnomaly(kind, corr string, err error) {
	payload := map[string]any{"anomaly": kind, "severity": "warning"}
	if corr != "" {
		payload["correlation_id"] = corr
	}
	if err != nil {
		payload["error"] = err.Error()
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "run.resume.anomaly",
		Kind:    event.KindAnomalyDetected,
		Actor:   "kernel",
		Payload: payload,
	})
}
