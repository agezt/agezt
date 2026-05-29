// SPDX-License-Identifier: MIT

// Package approval is the human-in-the-loop pause point. When Edict's
// trust ladder lands on an Ask-class level (L1..L3) and the engine is
// configured to actually prompt, the agent tool-loop suspends, the
// Registry emits an `approval.requested` event, and the caller blocks
// on Submit until either:
//
//   - an out-of-band caller (agt approve/deny over the control plane,
//     or — later — Telegram, web UI, Pulse) calls Resolve, or
//   - the per-request timeout fires (auto-deny with Reason=timeout).
//
// All four outcomes (granted / denied / timeout / cancelled) are
// journaled with the original CorrelationID so `agt why` walks the
// chain from the originating task to the approval verdict.
//
// SPEC-06 §3.4 defines this surface. M1.d ships the kernel + control-
// plane path; channel-routed prompts (Telegram, in-IDE) land later by
// implementing the same Resolve API.
package approval

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ersinkoc/agezt/kernel/bus"
	"github.com/ersinkoc/agezt/kernel/event"
	"github.com/ersinkoc/agezt/kernel/ulid"
)

// Decision is the operator's verdict on a pending Request.
type Decision string

const (
	DecisionGrant   Decision = "grant"
	DecisionDeny    Decision = "deny"
	DecisionTimeout Decision = "timeout"  // synthesised when DefaultTimeout fires
	DecisionCancel  Decision = "cancel"   // synthesised when caller ctx is cancelled
)

// IsTerminal reports whether d is a final outcome (any of the four).
func (d Decision) IsTerminal() bool {
	switch d {
	case DecisionGrant, DecisionDeny, DecisionTimeout, DecisionCancel:
		return true
	}
	return false
}

// DefaultTimeout caps how long a Submit blocks waiting for a Resolve.
// SPEC-06 §3.4: "Time-outs default to deny."
const DefaultTimeout = 5 * time.Minute

// Request is the data the operator sees to make a decision.
type Request struct {
	// ID is a ULID minted at Submit time; the operator uses it for
	// `agt approve <id>`.
	ID string
	// Capability is the Edict capability the tool wants (e.g. "shell",
	// "file.delete").
	Capability string
	// ToolName is the agent.Tool's Name() (e.g. "shell", "file").
	ToolName string
	// Input is the JSON the model passed to the tool. Kept verbatim so
	// the operator can inspect *exactly* what was about to run.
	Input string
	// Reason is the human-readable rationale from Edict (e.g.
	// "level ask-first; AskPolicy=AskPrompt").
	Reason string
	// Actor is the originating agent (e.g. "agent-run-…").
	Actor string
	// CorrelationID ties this approval to the originating task; the
	// resulting events all carry the same ID so `agt why` works.
	CorrelationID string
	// CreatedAt is when Submit was called (UTC).
	CreatedAt time.Time
	// Timeout is when DefaultTimeout would synthesise a deny (UTC).
	Timeout time.Time
}

// Outcome carries the decision plus a human reason for the journal.
type Outcome struct {
	Decision Decision
	Reason   string
	// ResolvedBy is "operator" (Resolve called) or "system" (timeout /
	// cancel). Future channel sources will set their own identity.
	ResolvedBy string
}

// pending is the internal in-memory entry awaiting a decision.
type pending struct {
	req  Request
	done chan Outcome
}

// Registry is the in-process approval queue. Safe for concurrent use.
//
// Lifecycle: Submit adds a pending entry and blocks; Resolve removes
// it and unblocks the waiter. Pending lists what is currently waiting.
type Registry struct {
	bus *bus.Bus
	now func() time.Time

	mu      sync.Mutex
	entries map[string]*pending // ID → pending

	timeout time.Duration
}

// Config tunes a Registry.
type Config struct {
	// Bus receives all four approval.* events. May be nil for tests;
	// events are silently dropped.
	Bus *bus.Bus
	// Timeout overrides DefaultTimeout when > 0.
	Timeout time.Duration
	// Now overrides time.Now for tests (esp. timeout assertions).
	Now func() time.Time
}

// New constructs an empty Registry.
func New(cfg Config) *Registry {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Registry{
		bus:     cfg.Bus,
		now:     now,
		entries: map[string]*pending{},
		timeout: timeout,
	}
}

// ErrUnknownApproval is returned by Resolve for an ID that has no
// pending entry (either never created, already resolved, or expired).
var ErrUnknownApproval = errors.New("approval: unknown or already-resolved request")

// SubmitSpec is the call-site data Submit needs; ID/CreatedAt/Timeout
// are filled in by the Registry so callers can't fabricate them.
type SubmitSpec struct {
	Capability    string
	ToolName      string
	Input         string
	Reason        string
	Actor         string
	CorrelationID string
}

// Submit registers a pending request and blocks until Resolve is
// called for it, the per-request timeout fires (DecisionTimeout), or
// ctx is cancelled (DecisionCancel). The returned Outcome is also
// journaled as approval.{granted,denied,timeout} so the trace is
// auditable.
func (r *Registry) Submit(ctx context.Context, spec SubmitSpec) Outcome {
	now := r.now()
	req := Request{
		ID:            "appr-" + ulid.New(),
		Capability:    spec.Capability,
		ToolName:      spec.ToolName,
		Input:         spec.Input,
		Reason:        spec.Reason,
		Actor:         spec.Actor,
		CorrelationID: spec.CorrelationID,
		CreatedAt:     now,
		Timeout:       now.Add(r.timeout),
	}
	entry := &pending{req: req, done: make(chan Outcome, 1)}

	r.mu.Lock()
	r.entries[req.ID] = entry
	r.mu.Unlock()

	r.publishRequested(req)

	// Detach from the entry on every exit so a Resolve after exit is
	// a clean no-op rather than blocking on the buffered channel.
	defer r.detach(req.ID)

	timer := time.NewTimer(r.timeout)
	defer timer.Stop()

	select {
	case out := <-entry.done:
		r.publishResolved(req, out)
		return out
	case <-timer.C:
		out := Outcome{Decision: DecisionTimeout, Reason: "no response within timeout", ResolvedBy: "system"}
		r.publishResolved(req, out)
		return out
	case <-ctx.Done():
		out := Outcome{Decision: DecisionCancel, Reason: ctx.Err().Error(), ResolvedBy: "system"}
		r.publishResolved(req, out)
		return out
	}
}

// Resolve records a decision for the named ID. Returns
// ErrUnknownApproval if no pending entry exists. Idempotent for the
// caller — a second Resolve for the same ID returns ErrUnknownApproval
// because the first one already detached the entry.
func (r *Registry) Resolve(id string, decision Decision, reason, resolvedBy string) error {
	if decision != DecisionGrant && decision != DecisionDeny {
		return fmt.Errorf("approval: Resolve accepts only grant/deny, got %q", decision)
	}
	r.mu.Lock()
	entry, ok := r.entries[id]
	if ok {
		delete(r.entries, id)
	}
	r.mu.Unlock()
	if !ok {
		return ErrUnknownApproval
	}
	if resolvedBy == "" {
		resolvedBy = "operator"
	}
	// Non-blocking send — buffered channel of size 1 means this never
	// blocks, but in the unlikely race where the waiter already exited
	// (ctx-cancel / timeout) we just drop the outcome.
	select {
	case entry.done <- Outcome{Decision: decision, Reason: reason, ResolvedBy: resolvedBy}:
	default:
	}
	return nil
}

// Pending returns a snapshot of currently-pending requests, sorted by
// CreatedAt ascending. Safe to read concurrently with Submit/Resolve.
func (r *Registry) Pending() []Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Request, 0, len(r.entries))
	for _, p := range r.entries {
		out = append(out, p.req)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// PendingCount is a cheap len() over the queue, useful for tests.
func (r *Registry) PendingCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

func (r *Registry) detach(id string) {
	r.mu.Lock()
	delete(r.entries, id)
	r.mu.Unlock()
}

func (r *Registry) publishRequested(req Request) {
	if r.bus == nil {
		return
	}
	_, _ = r.bus.Publish(event.Spec{
		Subject:       "approval.request",
		Kind:          event.KindApprovalRequested,
		Actor:         actorOr(req.Actor, "approval"),
		CorrelationID: req.CorrelationID,
		Payload: map[string]any{
			"approval_id":   req.ID,
			"capability":    req.Capability,
			"tool_name":     req.ToolName,
			"input":         req.Input,
			"reason":        req.Reason,
			"timeout_unix":  req.Timeout.Unix(),
			"created_unix":  req.CreatedAt.Unix(),
		},
	})
}

func (r *Registry) publishResolved(req Request, out Outcome) {
	if r.bus == nil {
		return
	}
	var kind event.Kind
	switch out.Decision {
	case DecisionGrant:
		kind = event.KindApprovalGranted
	case DecisionDeny:
		kind = event.KindApprovalDenied
	case DecisionTimeout:
		kind = event.KindApprovalTimeout
	case DecisionCancel:
		// Cancellation is a system event; reuse the denied kind with
		// a clear reason so consumers only have to know about the
		// three terminal kinds.
		kind = event.KindApprovalDenied
	}
	_, _ = r.bus.Publish(event.Spec{
		Subject:       "approval.resolve",
		Kind:          kind,
		Actor:         actorOr(req.Actor, "approval"),
		CorrelationID: req.CorrelationID,
		Payload: map[string]any{
			"approval_id": req.ID,
			"decision":    string(out.Decision),
			"reason":      out.Reason,
			"resolved_by": out.ResolvedBy,
		},
	})
}

func actorOr(a, fallback string) string {
	if a == "" {
		return fallback
	}
	return a
}
