// SPDX-License-Identifier: MIT

package proofapi

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/contract/assureapi"
)

// TestTheProofWireShape: the proof is **persisted** with the task it belongs
// to, so its JSON keys are a contract with every stored proof. The
// always-present keys and the dropped-when-empty ones are pinned.
func TestTheProofWireShape(t *testing.T) {
	raw, err := json.Marshal(Proof{Verdict: assureapi.Verdict{Complete: true}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"verdict", "evidence", "proved_ms"} {
		if _, ok := got[k]; !ok {
			t.Errorf("the proof wire must always carry %q, got %v", k, got)
		}
	}
	for _, k := range []string{"criteria", "attempts", "judge"} {
		if _, ok := got[k]; ok {
			t.Errorf("the proof wire must drop %q when it is empty, got %v", k, got)
		}
	}
}

// TestTheCriterionWireShape: the criterion is one acceptance condition —
// its text and whether it was met are always on the wire, the judge's note is
// not.
func TestTheCriterionWireShape(t *testing.T) {
	raw, err := json.Marshal(Criterion{Text: "tests pass"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"text", "met"} {
		if _, ok := got[k]; !ok {
			t.Errorf("the criterion wire must always carry %q, got %v", k, got)
		}
	}
	if _, ok := got["note"]; ok {
		t.Errorf("the criterion wire must drop the note when it is empty, got %v", got)
	}

	raw, err = json.Marshal(Criterion{Text: "tests pass", Met: true, Note: "the CI run is green"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got = nil
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := got["note"]; !ok {
		t.Errorf("the criterion wire must carry the note when there is one, got %v", got)
	}
}

// TestTheEvidenceWireShape: the evidence points at what the proof rests on —
// every part of it is optional, so an empty evidence leaves no key behind.
func TestTheEvidenceWireShape(t *testing.T) {
	raw, err := json.Marshal(Evidence{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("an empty evidence must leave no key on the wire, got %v", got)
	}

	raw, err = json.Marshal(Evidence{Corr: "corr-1", Artifacts: []string{"a.txt"}, JournalFrom: 1, JournalTo: 9})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got = nil
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"corr", "artifacts", "journal_from", "journal_to"} {
		if _, ok := got[k]; !ok {
			t.Errorf("the evidence wire must carry %q when it is set, got %v", k, got)
		}
	}
}
