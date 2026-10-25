// SPDX-License-Identifier: MIT

package approvalapi

import (
	"reflect"
	"testing"
)

// TestTheSubmitRowCarriesWhatTheOperatorMustSee: the submit row is what the
// operator answers — what is wanted, why, and what they need to see to decide.
//
// The row lives in the contract layer because the workflow engine's port
// carries it (W3.48): the engine is a MODULE, and a module's port is the
// contract's.
//
// The fields are pinned because a dropped field is a dropped question: without
// the effect class the operator cannot weigh the request, and without the
// correlation the journal could not walk the chain from the task to the
// verdict.
func TestTheSubmitRowCarriesWhatTheOperatorMustSee(t *testing.T) {
	typ := reflect.TypeOf(SubmitSpec{})
	want := []string{
		"Capability",
		"ToolName",
		"Input",
		"Reason",
		"Actor",
		"CorrelationID",
		"AutoRec",
		"ValuePreview",
		"EffectClass",
		"PredictedEffects",
		"AffectedResources",
		"RollbackNotes",
		"Confidence",
		"CanonicalIntent",
		"HarmfulInterpretation",
		"AmbiguityScore",
		"RegretAxes",
		"ConfirmationPrompt",
	}
	if typ.NumField() != len(want) {
		t.Fatalf("the submit row carries %d fields, want %d", typ.NumField(), len(want))
	}
	for i, name := range want {
		if got := typ.Field(i).Name; got != name {
			t.Errorf("submit field %d = %q, want %q", i, got, name)
		}
	}
}

// TestTheOutcomeRowCarriesTheVerdictAndWhy: the outcome is the registry's
// answer — the verdict, why it came out that way, and who resolved it.
//
// "who resolved it" matters: an operator's answer and a timeout are both
// terminal verdicts, and the journal must be able to tell them apart.
func TestTheOutcomeRowCarriesTheVerdictAndWhy(t *testing.T) {
	typ := reflect.TypeOf(Outcome{})
	want := []string{"Decision", "Reason", "ResolvedBy"}
	if typ.NumField() != len(want) {
		t.Fatalf("the outcome row carries %d fields, want %d", typ.NumField(), len(want))
	}
	for i, name := range want {
		if got := typ.Field(i).Name; got != name {
			t.Errorf("outcome field %d = %q, want %q", i, got, name)
		}
	}
	if got := reflect.TypeOf(Outcome{}).Field(0).Type.String(); got != "approvalapi.Decision" {
		t.Fatalf("the outcome carries %q as its verdict, want the contract's Decision", got)
	}
}

// TestTheSubmitRowIsTheRegistrysAnswerShape: the row is plain data — no
// methods, no behaviour — so the module that fills it and the platform that
// reads it cannot drift apart in what they carry.
func TestTheSubmitRowIsTheRegistrysAnswerShape(t *testing.T) {
	if typ := reflect.TypeOf(SubmitSpec{}); typ.NumMethod() != 0 {
		t.Fatalf("the submit row must carry no methods, got %d", typ.NumMethod())
	}
	if typ := reflect.TypeOf(Outcome{}); typ.NumMethod() != 0 {
		t.Fatalf("the outcome row must carry no methods, got %d", typ.NumMethod())
	}
}
