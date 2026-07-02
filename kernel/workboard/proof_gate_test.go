// SPDX-License-Identifier: MIT

package workboard

import (
	"errors"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/assure"
	"github.com/agezt/agezt/kernel/proof"
)

// A task with no acceptance criteria is ungated: Complete works as before.
func TestCompleteUngatedWithoutCriteria(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := st.Create(CreateSpec{Title: "ship it"}, time.UnixMilli(1000))
	if err != nil {
		t.Fatal(err)
	}
	done, err := st.Complete(task.ID, "worker", time.UnixMilli(2000))
	if err != nil {
		t.Fatalf("Complete ungated: %v", err)
	}
	if done.Status != StatusDone {
		t.Fatalf("status = %s, want done", done.Status)
	}
}

// A criteria-bearing task cannot Complete until a satisfying proof is recorded.
func TestCompleteGatedByCriteria(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := st.Create(CreateSpec{
		Title:              "ship X",
		AcceptanceCriteria: []string{"tests pass", "doc updated"},
	}, time.UnixMilli(1000))
	if err != nil {
		t.Fatal(err)
	}
	if len(task.Criteria) != 2 || task.Criteria[0].Text != "tests pass" || task.Criteria[0].Met {
		t.Fatalf("criteria not seeded unmet: %+v", task.Criteria)
	}
	if _, err := st.Complete(task.ID, "worker", time.UnixMilli(2000)); !errors.Is(err, ErrUnproven) {
		t.Fatalf("Complete without proof err=%v, want ErrUnproven", err)
	}
}

// An unsatisfied proof parks the task in review with the gap visible; a later
// satisfying proof lets it reach done, and only then does Complete succeed.
func TestProveGateReviewThenDone(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := st.Create(CreateSpec{
		Title:              "ship X",
		AcceptanceCriteria: []string{"tests pass", "doc updated"},
	}, time.UnixMilli(1000))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Claim(task.ID, "worker", "run-1", time.UnixMilli(1100)); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	// First proof: verifier not convinced, one criterion unmet → review.
	unmet := proof.Proof{
		Verdict:  assure.Verdict{Complete: false, Gap: "doc still stale"},
		Criteria: []proof.Criterion{{Text: "tests pass", Met: true}, {Text: "doc updated", Met: false}},
	}
	rev, err := st.Prove(task.ID, "judge", unmet, time.UnixMilli(2000))
	if err != nil {
		t.Fatalf("Prove(unmet): %v", err)
	}
	if rev.Status != StatusReview {
		t.Fatalf("status = %s, want review", rev.Status)
	}
	if rev.Proof == nil || rev.Proof.Satisfied() {
		t.Fatalf("proof should be attached and unsatisfied: %+v", rev.Proof)
	}
	if rev.Claim != nil {
		t.Fatalf("claim should be cleared after prove")
	}
	// Gate still holds while unproven.
	if _, err := st.Complete(task.ID, "worker", time.UnixMilli(2100)); !errors.Is(err, ErrUnproven) {
		t.Fatalf("Complete while unproven err=%v, want ErrUnproven", err)
	}

	// Second proof: complete + all met → done directly via Prove.
	met := proof.Proof{
		Verdict:  assure.Verdict{Complete: true},
		Criteria: []proof.Criterion{{Text: "tests pass", Met: true}, {Text: "doc updated", Met: true}},
		Evidence: proof.Evidence{Corr: "run-1", Artifacts: []string{"art-1"}, JournalFrom: 10, JournalTo: 42},
		Judge:    "verify",
	}
	done, err := st.Prove(task.ID, "judge", met, time.UnixMilli(3000))
	if err != nil {
		t.Fatalf("Prove(met): %v", err)
	}
	if done.Status != StatusDone || done.CompletedMS != 3000 {
		t.Fatalf("done = %+v", done)
	}
	if done.Proof == nil || !done.Proof.Satisfied() || done.Proof.Evidence.JournalTo != 42 {
		t.Fatalf("satisfying proof not persisted: %+v", done.Proof)
	}
	// Every declared criterion should now read met.
	for _, c := range done.Criteria {
		if !c.Met {
			t.Fatalf("criterion %q left unmet after satisfying proof", c.Text)
		}
	}
}

// Re-proving an already-done task with a now-failing verdict sends it back to
// review and must NOT leave a stale completion timestamp behind.
func TestReproveClearsCompletedMS(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := st.Create(CreateSpec{Title: "ship X", AcceptanceCriteria: []string{"tests pass"}}, time.UnixMilli(1000))
	if err != nil {
		t.Fatal(err)
	}
	// First proof satisfies → done with a completion time.
	ok := proof.Proof{Verdict: assure.Verdict{Complete: true}, Criteria: []proof.Criterion{{Text: "tests pass", Met: true}}}
	done, err := st.Prove(task.ID, "judge", ok, time.UnixMilli(2000))
	if err != nil || done.Status != StatusDone || done.CompletedMS != 2000 {
		t.Fatalf("first prove: status=%s completed=%d err=%v", done.Status, done.CompletedMS, err)
	}
	// Re-prove, now failing → review, and CompletedMS must be cleared.
	bad := proof.Proof{Verdict: assure.Verdict{Complete: false, Gap: "regressed"}, Criteria: []proof.Criterion{{Text: "tests pass", Met: false}}}
	rev, err := st.Prove(task.ID, "judge", bad, time.UnixMilli(3000))
	if err != nil {
		t.Fatalf("re-prove: %v", err)
	}
	if rev.Status != StatusReview {
		t.Fatalf("status = %s, want review", rev.Status)
	}
	if rev.CompletedMS != 0 {
		t.Fatalf("CompletedMS = %d, want 0 (stale completion not cleared)", rev.CompletedMS)
	}
}

// Prove reconciles judged outcomes onto declared criteria by text, ignoring
// order and stray judged entries, and defaults unaddressed criteria to unmet.
func TestProveReconcilesCriteriaByText(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := st.Create(CreateSpec{
		Title:              "ship X",
		AcceptanceCriteria: []string{"tests pass", "doc updated"},
	}, time.UnixMilli(1000))
	if err != nil {
		t.Fatal(err)
	}
	// Judged list is out of order, only addresses one criterion, and carries a
	// stray entry that must be ignored.
	p := proof.Proof{
		Verdict: assure.Verdict{Complete: true},
		Criteria: []proof.Criterion{
			{Text: "unrelated", Met: true},
			{Text: "tests pass", Met: true, Note: "green"},
		},
	}
	got, err := st.Prove(task.ID, "judge", p, time.UnixMilli(2000))
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if len(got.Criteria) != 2 {
		t.Fatalf("criteria len = %d, want 2", len(got.Criteria))
	}
	if got.Criteria[0].Text != "tests pass" || !got.Criteria[0].Met || got.Criteria[0].Note != "green" {
		t.Fatalf("criterion 0 = %+v", got.Criteria[0])
	}
	if got.Criteria[1].Text != "doc updated" || got.Criteria[1].Met {
		t.Fatalf("unaddressed criterion should be unmet: %+v", got.Criteria[1])
	}
	// Complete verdict but one unmet criterion ⇒ not satisfied ⇒ review.
	if got.Status != StatusReview {
		t.Fatalf("status = %s, want review (one criterion unmet)", got.Status)
	}
}
