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
//
// This file owns the data types + the Registry constructor; the
// Registry methods (Submit / Resolve / Pending) live in
// approval_methods.go. Carved out during the Day-211 god-file split.
// Public API unchanged.
package approval

import (
	"errors"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
)

// Decision is the operator's verdict on a pending Request.
type Decision string

const (
	DecisionGrant   Decision = "grant"
	DecisionDeny    Decision = "deny"
	DecisionTimeout Decision = "timeout" // synthesised when DefaultTimeout fires
	DecisionCancel  Decision = "cancel"  // synthesised when caller ctx is cancelled
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
	// EffectClass classifies the requested action by reversibility
	// (read_only/reversible/compensable/irreversible).
	EffectClass string
	// PredictedEffects summarizes what the action is expected to change.
	PredictedEffects []string
	// AffectedResources lists the concrete resources the operator should inspect
	// before approving.
	AffectedResources []string
	// RollbackNotes describes the available undo/compensation path, if any.
	RollbackNotes string
	// Confidence is the agent/runtime confidence in the prediction, 0..1 when
	// known. 0 means unspecified.
	Confidence float64
	// CanonicalIntent is the interpreter's compact reading of the user's request.
	CanonicalIntent string
	// HarmfulInterpretation is the most plausible costly wrong reading surfaced
	// by the intent gate.
	HarmfulInterpretation string
	// AmbiguityScore is the interpreter's uncertainty score, 0..1 when known.
	AmbiguityScore float64
	// RegretAxes estimates wrong-action cost by physical/informational/social/
	// identity axes.
	RegretAxes map[string]float64
	// ConfirmationPrompt is the targeted question the operator should answer.
	ConfirmationPrompt string
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
	// AutoRec is the auto-recommendation for the operator (e.g., "allow" or "deny").
	AutoRec string
	// ValuePreview is a masked preview of the value being accessed.
	ValuePreview          string
	EffectClass           string
	PredictedEffects      []string
	AffectedResources     []string
	RollbackNotes         string
	Confidence            float64
	CanonicalIntent       string
	HarmfulInterpretation string
	AmbiguityScore        float64
	RegretAxes            map[string]float64
	ConfirmationPrompt    string
}
