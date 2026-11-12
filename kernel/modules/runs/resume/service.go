// SPDX-License-Identifier: MIT

package resume

// The ticket LIFECYCLE: who owns a ticket, when it is written, when it is
// refreshed and when it is deleted.
//
// The rows, the store and the rules that judge a ticket live in this package
// since W3.76a — but the OPERATIONS over them stayed in the daemon
// (kernel/runtime/resume.go). The daemon held three public verbs for them
// (ClaimResumeTicket, FinalizeResumeTicket, ResumeCheckpointFn) and the run
// engine — which lives INSIDE this module — asked the host for them through
// its port: a round trip inside one module, three times.
//
// The operations belong here for the same reason the rows do: a ticket is a
// RUN's own durable state, and the run engine is the one that claims it,
// refreshes it and finalizes it. The module keeps the lifecycle; the host
// keeps its own two duties — the boot-time resumer (which enumerates the
// suspended tickets and re-dispatches them) and the shutdown protocol (which
// flips the live tickets to "suspended").

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
)

// Dependencies are the narrow ports the ticket lifecycle needs. Everything
// else it touches is the module's own: the store, the rules, the handshake's
// vocabulary and the run context.
type Dependencies struct {
	// Store is the module's own durable ticket store. A nil store means
	// resume is off: no ticket is ever written, and every operation is a
	// no-op.
	Store *Store

	// Enabled reports whether the HOST's configuration turns resume on. The
	// daemon's config is the host's own state, and it is read per run — the
	// operator may turn resume off while the daemon runs.
	Enabled func() bool

	// Publish is the HOST's bus: the lifecycle's anomalies travel on it.
	Publish func(spec event.Spec) (*event.Event, error)

	// Suspending reports whether the daemon is going down. A run cancelled
	// while the daemon suspends is a resume candidate, so its ticket is kept
	// rather than deleted.
	Suspending func() bool
}

// Service owns the ticket lifecycle: the claim, the per-iteration snapshot
// and the finalization.
//
// nil ports are safe: a service without an Enabled port never writes a
// ticket, one without a Publish port stays quiet about its anomalies, and one
// without a Suspending port treats every terminal outcome as final.
type Service struct {
	store      *Store
	enabled    func() bool
	publish    func(spec event.Spec) (*event.Event, error)
	suspending func() bool
}

// New builds the ticket lifecycle service.
func New(deps Dependencies) *Service {
	return &Service{
		store:      deps.Store,
		enabled:    deps.Enabled,
		publish:    deps.Publish,
		suspending: deps.Suspending,
	}
}

// Store returns the module's durable ticket store — the handle the host's
// boot-time resumer enumerates. It is the module's object; the host asks the
// module for it rather than holding its own copy.
func (s *Service) Store() *Store { return s.store }

// Claim writes a fresh ticket for a root run and returns a context carrying
// the owned marker plus owns=true — UNLESS an outer frame already owns a
// ticket for this corr, resume is off, or this isn't a root run, in which case
// it returns the ctx unchanged with owns=false. Whoever receives owns=true
// must call Finalize when the run terminates.
func (s *Service) Claim(ctx context.Context, corr, intent, kind string, assureBudget int) (context.Context, bool) {
	if !s.active(ctx) {
		return ctx, false
	}
	if _, already := OwnedKindFromCtx(ctx); already {
		return ctx, false
	}
	t := s.buildTicket(ctx, corr, intent, kind, assureBudget)
	if err := s.store.Put(t); err != nil {
		// Best-effort: a failed ticket write must never block or fail the run.
		s.publishAnomaly("ticket_write_failed", corr, err)
		return ctx, false
	}
	return WithOwned(ctx, kind), true
}

// Finalize deletes a run's ticket on any terminal outcome EXCEPT an
// interruption by shutdown: a run cancelled while the daemon is suspending is
// a resume candidate, so its ticket is kept for the restart. A clean finish, a
// genuine failure, an operator cancel, or a per-run timeout all delete it —
// none of those should re-run on the next boot.
//
// The keep/delete rule is the module's (TerminalOutcome); the lifecycle
// applies it.
func (s *Service) Finalize(corr string, runErr error) {
	if s.store == nil {
		return
	}
	if TerminalOutcome(runErr, s.suspendingNow()) == DispositionKeep {
		return // interrupted by shutdown → keep for resume
	}
	if err := s.store.Delete(corr); err != nil {
		s.publishAnomaly("ticket_delete_failed", corr, err)
	}
}

// CheckpointFn returns the closure the agent loop calls at each safe
// iteration boundary to refresh the conversation snapshot. It copies the slice
// (the loop keeps appending after this returns) and is a no-op once the ticket
// is gone (clean completion deleted it). Returns nil when resume is
// disabled, so the loop pays zero overhead.
func (s *Service) CheckpointFn(corr string) func(int, []llm.Message) {
	if s.store == nil {
		return nil
	}
	return func(iter int, messages []llm.Message) {
		snap := append([]llm.Message(nil), messages...)
		if err := s.store.Snapshot(corr, snap, iter); err != nil {
			s.publishAnomaly("snapshot_failed", corr, err)
		}
	}
}

// SuspendAll flips every live ticket to "suspended" — the durable signal that
// these runs were interrupted by a restart, not completed. It returns the
// number of tickets marked; a failure is published as an anomaly and
// returned, because the caller (the host's shutdown path) must not be
// blocked by it.
func (s *Service) SuspendAll() (int, error) {
	if s.store == nil {
		return 0, nil
	}
	n, err := s.store.MarkSuspendedAll()
	if err != nil {
		s.publishAnomaly("mark_suspended_failed", "", err)
	}
	return n, err
}

// active reports whether this ctx describes a run eligible to carry a
// resume ticket: the feature is on and it is a ROOT run. Sub-agents carry a
// ParentCorrelation (and never reach the governed entry points anyway), so
// their work is re-driven by resuming the root rather than persisted
// separately.
func (s *Service) active(ctx context.Context) bool {
	return s.store != nil && s.enabledNow() &&
		CarriesTicket(runctx.WakeContextFromCtx(ctx).ParentCorrelation)
}

// buildTicket stores the agent slug (the profile is reloaded on boot) and
// resolved governance ceilings, so resume cannot silently regain authority.
//
// Explicit model/system/tools overrides are not stored yet and remain
// non-resumable. Profile-sourced defaults are reconstructible, even when set.
func (s *Service) buildTicket(ctx context.Context, corr, intent, kind string, assureBudget int) *Ticket {
	facts := Facts{
		Corr:         corr,
		Intent:       intent,
		Kind:         kind,
		AssureBudget: assureBudget,
		AgentSlug:    runctx.AgentSlugFromCtx(ctx),
		MaxCostMc:    runctx.MaxCostFromCtx(ctx),
		Wake:         runctx.WakeContextFromCtx(ctx),
	}
	if lvl, ok := runctx.TrustCeilingFromCtx(ctx); ok {
		v := int(lvl)
		facts.TrustCeiling = &v
	}
	if d := runctx.RunTimeoutFromCtx(ctx); d > 0 {
		facts.RunTimeoutMs = d.Milliseconds()
	}
	facts.System = setting(runctx.SystemSetting(ctx))
	facts.Model = setting(runctx.ModelSetting(ctx))
	if _, ok := runctx.ToolsFromCtx(ctx); ok {
		facts.ToolsOverridden = true
	}
	return Build(facts)
}

// setting reads one resolved run setting (the system prompt or the model) and
// reports where it came from: the agent profile that supplied it
// (reconstructible on boot) or an explicit per-run override (not).
func setting(s runctx.RunStringSetting) Setting {
	return Setting{Set: s.Value != "", SourceProfile: s.ProfileSlug}
}

// enabledNow reads the host's resume switch. A host that wired no switch has
// resume off.
func (s *Service) enabledNow() bool { return s.enabled != nil && s.enabled() }

// suspendingNow reads the host's shutdown latch. A host that wired no latch
// is not suspending, so every terminal outcome is final.
func (s *Service) suspendingNow() bool { return s.suspending != nil && s.suspending() }

// publishAnomaly journals a resume anomaly on the host's bus.
func (s *Service) publishAnomaly(kind, corr string, err error) {
	if s.publish == nil {
		return
	}
	payload := map[string]any{"anomaly": kind, "severity": "warning"}
	if corr != "" {
		payload["correlation_id"] = corr
	}
	if err != nil {
		payload["error"] = err.Error()
	}
	_, _ = s.publish(event.Spec{
		Subject: "run.resume.anomaly",
		Kind:    event.KindAnomalyDetected,
		Actor:   "kernel",
		Payload: payload,
	})
}
