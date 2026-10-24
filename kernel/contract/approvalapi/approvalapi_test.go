// SPDX-License-Identifier: MIT

package approvalapi

import (
	"reflect"
	"testing"
)

// TestTheVerdictVocabularyIsClosed: the operator's answer is one of four
// values, and nothing else. A verdict outside the vocabulary would be a
// final outcome nobody can interpret — and the promotion flow switches on it.
func TestTheVerdictVocabularyIsClosed(t *testing.T) {
	for _, d := range []Decision{DecisionGrant, DecisionDeny, DecisionTimeout, DecisionCancel} {
		if !d.IsTerminal() {
			t.Errorf("%q must be a terminal outcome", d)
		}
		switch d {
		case "grant", "deny", "timeout", "cancel":
		default:
			t.Errorf("%q is not one of the four canonical verdicts", d)
		}
	}
	for _, d := range []Decision{"", "approved", "Grant"} {
		if d.IsTerminal() {
			t.Errorf("%q must not pass as a terminal outcome", d)
		}
	}
}

// TestTheVerdictIsAStringRow: the verdict travels as a plain string, so the
// wire that carries it does not change when the row moves.
func TestTheVerdictIsAStringRow(t *testing.T) {
	if reflect.TypeOf(Decision("")).Kind() != reflect.String {
		t.Fatalf("Decision must be a string row, got %v", reflect.TypeOf(Decision("")).Kind())
	}
	if reflect.TypeOf(Decision("")).Name() != "Decision" {
		t.Fatalf("the verdict row must be named Decision, got %v", reflect.TypeOf(Decision("")).Name())
	}
}
