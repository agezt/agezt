// SPDX-License-Identifier: MIT

// Package guards is the platform's POLICY GUARD set: the fail-closed rules
// that decide whether an otherwise-allowed tool call must see a human.
//
// The rules are the platform's: they own no store, they publish nothing, and
// every host asks the same ones — the shape the epistemic guard took in
// W3.73. The host keeps the doors: it reads the run's context, it publishes
// the events, and it submits the approval.
//
// The three guards are armed one by one, each by its own operator setting:
//
//   - the epistemic escalation (the platform's own guard, W3.73);
//   - the intent/regret gate (the operator's frame says the call's regret
//     axes need a confirmation);
//   - the prompt-injection guard (an effectful call downstream of a
//     directive-like untrusted observation).
//
// They are evaluated in that order, and the FIRST one that raises wins: a
// guard that has already stopped the call cannot be answered by a later one,
// and the operator is shown one reason — the guard's own.
//
// The session's blanket capability grant answers the Edict Ask axis only. It
// is not an answer to a question a guard asked for a different reason, so a
// guard the operator armed deliberately is never satisfied by it.
package guards

import (
	"strings"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/policy/epistemic"
)

// The guard names. They are the strings the daemon journals and the operator
// reads, and they travel inside the verdict's reason.
const (
	// GuardEpistemic names the epistemic escalation guard.
	GuardEpistemic = "epistemic escalation"
	// GuardIntentRegret names the intent/regret gate.
	GuardIntentRegret = "intent/regret gating"
	// GuardPromptInjection names the prompt-injection guard.
	GuardPromptInjection = "prompt-injection guard"
)

// PromptInjectionMode selects how the prompt-injection guard handles an
// effectful action downstream of directive-like untrusted content.
type PromptInjectionMode int

const (
	// PromptInjectionOn routes the action to HITL approval.
	PromptInjectionOn PromptInjectionMode = iota
	// PromptInjectionWarn allows the action but journals a warning (default).
	PromptInjectionWarn
	// PromptInjectionOff disables the active intervention entirely.
	PromptInjectionOff
)

// ParseMode maps an operator string (AGEZT_PROMPT_INJECTION_GUARD) to a mode.
// "off"/"0"/"false" -> Off, "warn"/"warning"/"audit" -> Warn, anything else
// (including empty) -> Warn. Operators who want a live approval stop on
// directive-like untrusted content set "on"/"block"/"prompt".
func ParseMode(s string) PromptInjectionMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "off", "0", "false", "no":
		return PromptInjectionOff
	case "on", "1", "true", "yes", "block", "prompt":
		return PromptInjectionOn
	case "", "warn", "warning", "audit":
		return PromptInjectionWarn
	default:
		return PromptInjectionWarn
	}
}

// Input is what the host knows when it asks the platform's guards.
//
// The host fills it in; the platform does not read anything itself. Every
// field is either the call's own verdict, the evidence a guard reasons over,
// or the setting the operator armed.
type Input struct {
	// Allow is the call's verdict so far. A guard only ever raises on a call
	// the normal policy ALLOWED: a denied call is not a guard's business.
	Allow bool

	// AskRequired and AskReason are the Edict's Ask axis — the call may run
	// only with the operator's approval.
	AskRequired bool
	AskReason   string

	// EffectClass is the call's effect class. A read-only call is not
	// downstream of an untrusted observation in any way that matters.
	EffectClass string

	// Epistemic is the platform's epistemic guard's decision (W3.73).
	Epistemic epistemic.Decision

	// RequiresConfirmation and ConfirmationPrompt are the intent/regret
	// gate's evidence: the operator's frame says the call's regret axes need
	// a confirmation, and the prompt is the question to ask.
	RequiresConfirmation bool
	ConfirmationPrompt   string

	// DirectiveLike says an untrusted observation carried directive-like
	// text, and TaintSources names where it came from.
	DirectiveLike bool
	TaintSources  []string

	// TrustedObservations says the operator chose to trust this run's
	// untrusted content, which downgrades the prompt-injection guard from
	// blocking to warning FOR THIS RUN.
	TrustedObservations bool

	// EpistemicEscalation arms the epistemic escalation guard.
	EpistemicEscalation bool

	// IntentRegretGating arms the intent/regret gate.
	IntentRegretGating bool

	// PromptInjection arms the prompt-injection guard.
	PromptInjection PromptInjectionMode

	// AutoApprove is the session's blanket capability grant.
	AutoApprove bool
}

// Outcome is the platform's answer: does the call need a human, why, and
// which guard asked.
type Outcome struct {
	// RequiresApproval says the call must not run unattended.
	RequiresApproval bool

	// Reason is what the operator is shown — the guard's own reason when a
	// guard raised, the Edict's reason otherwise.
	Reason string

	// Guard names the guard that raised, or is empty when none did.
	Guard string

	// Warn says the prompt-injection guard observed the call and did NOT
	// stop it. The host journals the warning.
	Warn bool

	// AutoApproved says the session's blanket grant answered the call. It
	// never answers a guard: only the Edict Ask axis.
	AutoApproved bool
}

// Evaluate applies the guards, in order, and arbitrates the result.
//
// The order is the contract: the epistemic escalation, then the intent/regret
// gate, then the prompt-injection guard. The first guard that raises ends
// the evaluation — a guard that has already stopped the call cannot be
// answered by a later one.
func Evaluate(in Input) Outcome {
	out := Outcome{RequiresApproval: in.AskRequired, Reason: in.AskReason}
	if in.Allow {
		switch {
		case in.EpistemicEscalation && in.Epistemic.Escalates():
			out.RequiresApproval = true
			out.Reason = in.Epistemic.Reason
			out.Guard = GuardEpistemic
			return out
		case in.IntentRegretGating && in.RequiresConfirmation:
			out.RequiresApproval = true
			out.Reason = in.ConfirmationPrompt
			out.Guard = GuardIntentRegret
			return out
		case in.PromptInjection != PromptInjectionOff && in.DirectiveLike && in.EffectClass != string(toolapi.EffectReadOnly):
			if in.PromptInjection == PromptInjectionOn && !in.TrustedObservations {
				out.RequiresApproval = true
				out.Reason = "prompt-injection guard: effectful action is downstream of directive-like untrusted observation from " + strings.Join(in.TaintSources, ", ")
				out.Guard = GuardPromptInjection
				return out
			}
			// Warn mode, and a run the operator chose to trust, audit the
			// call without interrupting it.
			out.Warn = true
			return out
		}
	}
	// The session's blanket grant answers the Ask axis only: a guard the
	// operator armed deliberately asked a different question, and a blanket
	// capability grant is not an answer to it. A guard that raised has already
	// returned, so the grant never sees one.
	if out.RequiresApproval && in.AutoApprove {
		out.AutoApproved = true
		out.RequiresApproval = false
	}
	return out
}
