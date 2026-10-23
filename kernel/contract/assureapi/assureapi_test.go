// SPDX-License-Identifier: MIT

package assureapi

import (
	"encoding/json"
	"testing"
)

// TestTheVerdictWireShape: the verdict crosses the wire — it is the model's
// answer, parsed into the row and then carried by the proof. "complete" is
// always present; the gap is dropped when there is none.
func TestTheVerdictWireShape(t *testing.T) {
	raw, err := json.Marshal(Verdict{Complete: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("the verdict wire has %d keys, want 1 (complete): %v", len(got), got)
	}
	if _, ok := got["complete"]; !ok {
		t.Errorf("the verdict wire must always carry \"complete\", got %v", got)
	}

	raw, err = json.Marshal(Verdict{Complete: false, Gap: "the tests are missing"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got = nil
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := got["gap"]; !ok {
		t.Errorf("the verdict wire must carry the gap when there is one, got %v", got)
	}
}

// TestTheResultWireShape: the outcome of an assured run — the answer and the
// completion flag are always present, the history and the gap are dropped
// when there is none.
func TestTheResultWireShape(t *testing.T) {
	raw, err := json.Marshal(Result{Answer: "done", Complete: true, Attempts: 1})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"answer", "complete", "attempts"} {
		if _, ok := got[k]; !ok {
			t.Errorf("the result wire must always carry %q, got %v", k, got)
		}
	}
	for _, k := range []string{"gap", "history"} {
		if _, ok := got[k]; ok {
			t.Errorf("the result wire must drop %q when it is empty, got %v", k, got)
		}
	}
}

// TestTheAttemptBudgetIsTheContract: the budget bounds are the contract's —
// the loop clamps to them, so a change has to be deliberate.
func TestTheAttemptBudgetIsTheContract(t *testing.T) {
	if DefaultMaxAttempts != 3 {
		t.Errorf("the default attempt budget is %d, want 3", DefaultMaxAttempts)
	}
	if MaxMaxAttempts != 10 {
		t.Errorf("the attempt ceiling is %d, want 10", MaxMaxAttempts)
	}
	if DefaultMaxAttempts > MaxMaxAttempts {
		t.Errorf("the default budget %d exceeds the ceiling %d", DefaultMaxAttempts, MaxMaxAttempts)
	}
}

// TestParseVerdictReadsTheStrictJSON: the model is asked for strict JSON, and
// the parser reads it.
func TestParseVerdictReadsTheStrictJSON(t *testing.T) {
	v, ok := ParseVerdict(`{"complete":true}`)
	if !ok {
		t.Fatal("a strict verdict must parse")
	}
	if !v.Complete {
		t.Error("the verdict must be complete")
	}
	if v.Gap != "" {
		t.Errorf("the gap must be empty, got %q", v.Gap)
	}

	v, ok = ParseVerdict(`{"complete":false,"gap":"the tests are missing"}`)
	if !ok {
		t.Fatal("a strict verdict must parse")
	}
	if v.Complete {
		t.Error("the verdict must not be complete")
	}
	if v.Gap != "the tests are missing" {
		t.Errorf("the gap is %q", v.Gap)
	}
}

// TestParseVerdictReadsTheFencedReply: in practice the reply arrives wrapped
// in a ```json fence — the parser has to see through it.
func TestParseVerdictReadsTheFencedReply(t *testing.T) {
	v, ok := ParseVerdict("```json\n{\"complete\":true,\"gap\":\"\"}\n```")
	if !ok {
		t.Fatal("a fenced verdict must parse")
	}
	if !v.Complete {
		t.Error("the verdict must be complete")
	}
}

// TestParseVerdictReadsTheProseWrappedReply: the reply may arrive wrapped in
// prose — the parser isolates the JSON object.
func TestParseVerdictReadsTheProseWrappedReply(t *testing.T) {
	v, ok := ParseVerdict(`Here is my answer: {"complete":true} — hope that helps.`)
	if !ok {
		t.Fatal("a prose-wrapped verdict must parse")
	}
	if !v.Complete {
		t.Error("the verdict must be complete")
	}
}

// TestParseVerdictRefusesTheUnreadable: an unreadable reply is NOT a success.
// The caller treats an unparseable verdict as "not complete", so the parser
// must refuse rather than answer a zero verdict.
func TestParseVerdictRefusesTheUnreadable(t *testing.T) {
	for _, in := range []string{"", "   ", "not json at all", "{", `{"complete":"yes"}`} {
		v, ok := ParseVerdict(in)
		if ok {
			t.Errorf("ParseVerdict(%q) must refuse, got %+v", in, v)
		}
		if v.Complete {
			t.Errorf("ParseVerdict(%q) must not report completion", in)
		}
	}
}

// TestParseVerdictReadsTheEmptyObject: an empty JSON object IS an object, so
// it parses — into a verdict that does not claim completion. The rule is
// "no JSON object could be parsed", not "the object had to be complete".
func TestParseVerdictReadsTheEmptyObject(t *testing.T) {
	v, ok := ParseVerdict("{}")
	if !ok {
		t.Fatal("an empty JSON object must parse")
	}
	if v.Complete {
		t.Error("an empty verdict must not claim completion")
	}
}
