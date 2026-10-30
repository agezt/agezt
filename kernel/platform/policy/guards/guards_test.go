// SPDX-License-Identifier: MIT

package guards

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/policy/epistemic"
)

// The GUARDS' own tests (W3.74).
//
// The three fail-closed guards and the session grant's arbitration lived in
// the daemon (kernel/runtime/policy.go). These tests pin the behaviour where
// it now lives — the platform's rules, asked with the evidence the host read.

func escalating() epistemic.Decision {
	return epistemic.Decision{
		Action: epistemic.ActionEscalate,
		Reason: "epistemic policy requires human review: novel_tool_conditions",
	}
}

func allowing() epistemic.Decision {
	return epistemic.Decision{Action: epistemic.ActionAllow, Reason: "epistemic policy allowed"}
}

// TestTheEpistemicGuardRaisesWhenArmed: the guard the platform already keeps
// routes the call to a human when the operator armed it.
func TestTheEpistemicGuardRaisesWhenArmed(t *testing.T) {
	out := Evaluate(Input{
		Allow:               true,
		EpistemicEscalation: true,
		Epistemic:           escalating(),
	})
	if !out.RequiresApproval {
		t.Fatalf("the call must need a human: %+v", out)
	}
	if out.Guard != GuardEpistemic {
		t.Fatalf("guard = %q, want %q", out.Guard, GuardEpistemic)
	}
	if !strings.Contains(out.Reason, "novel_tool_conditions") {
		t.Fatalf("the reason must be the guard's own: %q", out.Reason)
	}
}

// TestTheEpistemicGuardStaysSilentWhenNotArmed: the same decision without the
// operator's setting is advisory only.
func TestTheEpistemicGuardStaysSilentWhenNotArmed(t *testing.T) {
	out := Evaluate(Input{Allow: true, Epistemic: escalating()})
	if out.RequiresApproval || out.Guard != "" {
		t.Fatalf("an unarmed guard must not raise: %+v", out)
	}
}

// TestTheIntentRegretGateRaisesWithItsOwnPrompt: the operator's frame asked
// a question, and the operator is shown that question.
func TestTheIntentRegretGateRaisesWithItsOwnPrompt(t *testing.T) {
	out := Evaluate(Input{
		Allow:                true,
		IntentRegretGating:   true,
		RequiresConfirmation: true,
		ConfirmationPrompt:   "delete the customer's record?",
	})
	if !out.RequiresApproval {
		t.Fatalf("the call must need a human: %+v", out)
	}
	if out.Guard != GuardIntentRegret {
		t.Fatalf("guard = %q, want %q", out.Guard, GuardIntentRegret)
	}
	if out.Reason != "delete the customer's record?" {
		t.Fatalf("reason = %q, want the frame's prompt", out.Reason)
	}
}

// TestTheFirstGuardWins: a guard that has already stopped the call cannot be
// answered by a later one, and the operator sees one reason.
func TestTheFirstGuardWins(t *testing.T) {
	out := Evaluate(Input{
		Allow:                true,
		EpistemicEscalation:  true,
		Epistemic:            escalating(),
		IntentRegretGating:   true,
		RequiresConfirmation: true,
		ConfirmationPrompt:   "the intent question",
		PromptInjection:      PromptInjectionOn,
		DirectiveLike:        true,
		TaintSources:         []string{"https://example.test"},
		EffectClass:          string(toolapi.EffectCompensable),
	})
	if out.Guard != GuardEpistemic {
		t.Fatalf("guard = %q, want the first one (%q)", out.Guard, GuardEpistemic)
	}
	if out.Reason == "the intent question" {
		t.Fatal("the later guard's reason must not replace the first guard's")
	}
}

// TestThePromptInjectionGuardBlocksAnEffectfulCall: an effectful call
// downstream of directive-like untrusted content goes to a human, and the
// reason names where the content came from.
func TestThePromptInjectionGuardBlocksAnEffectfulCall(t *testing.T) {
	out := Evaluate(Input{
		Allow:           true,
		PromptInjection: PromptInjectionOn,
		DirectiveLike:   true,
		TaintSources:    []string{"https://example.test", "https://other.test"},
		EffectClass:     string(toolapi.EffectCompensable),
	})
	if !out.RequiresApproval {
		t.Fatalf("the call must need a human: %+v", out)
	}
	if out.Guard != GuardPromptInjection {
		t.Fatalf("guard = %q, want %q", out.Guard, GuardPromptInjection)
	}
	if !strings.Contains(out.Reason, "https://example.test") || !strings.Contains(out.Reason, "https://other.test") {
		t.Fatalf("the reason must name the sources: %q", out.Reason)
	}
	if out.Warn {
		t.Fatal("a blocked call is not a warning")
	}
}

// TestThePromptInjectionGuardWarnsInWarnMode: the default posture audits the
// call without interrupting it.
func TestThePromptInjectionGuardWarnsInWarnMode(t *testing.T) {
	out := Evaluate(Input{
		Allow:           true,
		PromptInjection: PromptInjectionWarn,
		DirectiveLike:   true,
		TaintSources:    []string{"https://example.test"},
		EffectClass:     string(toolapi.EffectCompensable),
	})
	if out.RequiresApproval || out.Guard != "" {
		t.Fatalf("warn mode must not stop the call: %+v", out)
	}
	if !out.Warn {
		t.Fatalf("warn mode must warn: %+v", out)
	}
}

// TestThePromptInjectionGuardWarnsForATrustedRun: a run the operator chose to
// trust is audited, not interrupted.
func TestThePromptInjectionGuardWarnsForATrustedRun(t *testing.T) {
	out := Evaluate(Input{
		Allow:               true,
		PromptInjection:     PromptInjectionOn,
		DirectiveLike:       true,
		TaintSources:        []string{"https://example.test"},
		EffectClass:         string(toolapi.EffectCompensable),
		TrustedObservations: true,
	})
	if out.RequiresApproval || out.Guard != "" {
		t.Fatalf("a trusted run must not be interrupted: %+v", out)
	}
	if !out.Warn {
		t.Fatalf("a trusted run must still be audited: %+v", out)
	}
}

// TestThePromptInjectionGuardIgnoresAReadOnlyCall: reading is not an effect.
func TestThePromptInjectionGuardIgnoresAReadOnlyCall(t *testing.T) {
	out := Evaluate(Input{
		Allow:           true,
		PromptInjection: PromptInjectionOn,
		DirectiveLike:   true,
		TaintSources:    []string{"https://example.test"},
		EffectClass:     string(toolapi.EffectReadOnly),
	})
	if out.RequiresApproval || out.Guard != "" || out.Warn {
		t.Fatalf("a read-only call is not downstream of anything: %+v", out)
	}
}

// TestThePromptInjectionGuardStaysSilentWhenOff: the operator turned it off.
func TestThePromptInjectionGuardStaysSilentWhenOff(t *testing.T) {
	out := Evaluate(Input{
		Allow:           true,
		PromptInjection: PromptInjectionOff,
		DirectiveLike:   true,
		TaintSources:    []string{"https://example.test"},
		EffectClass:     string(toolapi.EffectCompensable),
	})
	if out.RequiresApproval || out.Guard != "" || out.Warn {
		t.Fatalf("an unarmed guard must not fire: %+v", out)
	}
}

// TestTheGrantAnswersTheAskAxis: the session's blanket grant satisfies the
// Edict Ask axis.
func TestTheGrantAnswersTheAskAxis(t *testing.T) {
	out := Evaluate(Input{
		AskRequired: true,
		AskReason:   "capability needs approval",
		AutoApprove: true,
	})
	if !out.AutoApproved {
		t.Fatalf("the grant must answer the Ask axis: %+v", out)
	}
	if out.RequiresApproval {
		t.Fatalf("an auto-approved call does not need a human: %+v", out)
	}
}

// TestTheGrantDoesNotAnswerAGuard: a guard the operator armed deliberately
// asked a different question, and a blanket capability grant is not an answer
// to it.
func TestTheGrantDoesNotAnswerAGuard(t *testing.T) {
	out := Evaluate(Input{
		Allow:               true,
		EpistemicEscalation: true,
		Epistemic:           escalating(),
		AutoApprove:         true,
	})
	if out.AutoApproved {
		t.Fatalf("the grant must not answer a guard: %+v", out)
	}
	if !out.RequiresApproval || out.Guard != GuardEpistemic {
		t.Fatalf("the guard must still stand: %+v", out)
	}
}

// TestTheGrantAnswersTheAskAxisOfADeniedCall: the Ask axis is not a guard's
// business, so the grant answers it even when the call is not allowed yet.
func TestTheGrantAnswersTheAskAxisOfADeniedCall(t *testing.T) {
	out := Evaluate(Input{
		Allow:       false,
		AskRequired: true,
		AskReason:   "capability needs approval",
		AutoApprove: true,
	})
	if !out.AutoApproved {
		t.Fatalf("the grant must answer the Ask axis: %+v", out)
	}
}

// TestADeniedCallRaisesNoGuard: a guard only ever raises on a call the normal
// policy allowed.
func TestADeniedCallRaisesNoGuard(t *testing.T) {
	out := Evaluate(Input{
		Allow:                false,
		EpistemicEscalation:  true,
		Epistemic:            escalating(),
		IntentRegretGating:   true,
		RequiresConfirmation: true,
		PromptInjection:      PromptInjectionOn,
		DirectiveLike:        true,
		EffectClass:          string(toolapi.EffectCompensable),
	})
	if out.Guard != "" {
		t.Fatalf("a denied call is not a guard's business: %+v", out)
	}
}

// TestTheGrantDoesNotCreateAnApproval: a grant is an answer, not a question.
// With nothing to answer, it changes nothing.
func TestTheGrantDoesNotCreateAnApproval(t *testing.T) {
	out := Evaluate(Input{Allow: true, AutoApprove: true})
	if out.AutoApproved {
		t.Fatalf("nothing asked, so nothing was answered: %+v", out)
	}
	if out.RequiresApproval {
		t.Fatalf("the grant must not raise an approval: %+v", out)
	}
}

// TestTheAskAxisWithoutTheGrantNeedsAHuman: nothing satisfied it, so the
// operator is asked.
func TestTheAskAxisWithoutTheGrantNeedsAHuman(t *testing.T) {
	out := Evaluate(Input{
		AskRequired: true,
		AskReason:   "capability needs approval",
	})
	if !out.RequiresApproval {
		t.Fatalf("the call must need a human: %+v", out)
	}
	if out.Guard != "" {
		t.Fatalf("no guard raised: %+v", out)
	}
	if out.Reason != "capability needs approval" {
		t.Fatalf("reason = %q, want the Edict's reason", out.Reason)
	}
}

// TestTheModeParsing: the operator's string maps to a mode.
func TestTheModeParsing(t *testing.T) {
	for _, raw := range []string{"", "nonsense", "warn", "audit", "warning"} {
		if got := ParseMode(raw); got != PromptInjectionWarn {
			t.Fatalf("ParseMode(%q) = %v, want warn", raw, got)
		}
	}
	for _, raw := range []string{"on", "block", "prompt", "1", "true"} {
		if got := ParseMode(raw); got != PromptInjectionOn {
			t.Fatalf("ParseMode(%q) = %v, want on", raw, got)
		}
	}
	for _, raw := range []string{"off", "0", "false", "no"} {
		if got := ParseMode(raw); got != PromptInjectionOff {
			t.Fatalf("ParseMode(%q) = %v, want off", raw, got)
		}
	}
}

// TestTheAllowingDecisionDoesNotRaise: an advisory decision with the guard
// armed is still advisory when it does not escalate.
func TestTheAllowingDecisionDoesNotRaise(t *testing.T) {
	out := Evaluate(Input{
		Allow:               true,
		EpistemicEscalation: true,
		Epistemic:           allowing(),
	})
	if out.RequiresApproval || out.Guard != "" {
		t.Fatalf("an allowing decision must not raise: %+v", out)
	}
}
