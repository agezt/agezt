// SPDX-License-Identifier: MIT

package workboard

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCreatePersistsAndDedupesByIdempotencyKey(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	now := time.UnixMilli(1000)
	first, created, err := st.Create(CreateSpec{
		Title:          "Ship workboard",
		Priority:       5,
		Tenant:         "core",
		IdempotencyKey: "task-1",
		Tags:           []string{"phase4", "phase4", ""},
	}, now)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !created || first.ID == "" || first.Status != StatusTriage || len(first.Tags) != 1 {
		t.Fatalf("created task = %+v created=%v", first, created)
	}

	dup, created, err := st.Create(CreateSpec{Title: "Different title", Tenant: "core", IdempotencyKey: "task-1"}, now)
	if err != nil {
		t.Fatalf("Create duplicate: %v", err)
	}
	if created || dup.ID != first.ID || dup.Title != first.Title {
		t.Fatalf("duplicate = %+v created=%v, want existing %+v", dup, created, first)
	}

	reopened, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, found := reopened.Get(first.ID)
	if !found || got.Title != "Ship workboard" || got.Priority != 5 {
		t.Fatalf("persisted task found=%v task=%+v", found, got)
	}
}

func TestClaimHeartbeatBlockCompleteArchive(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := st.Create(CreateSpec{Title: "Investigate", Assignee: "worker"}, time.UnixMilli(1000))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := st.Claim(task.ID, "worker", "run-1", time.UnixMilli(2000))
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimed.Status != StatusRunning || claimed.Claim == nil || claimed.Claim.HeartbeatMS != 2000 || len(claimed.Attempts) != 1 {
		t.Fatalf("claimed = %+v", claimed)
	}
	if _, err := st.Claim(task.ID, "other", "run-2", time.UnixMilli(2100)); !errors.Is(err, ErrClaimConflict) {
		t.Fatalf("conflicting claim err=%v, want ErrClaimConflict", err)
	}

	beat, err := st.Heartbeat(task.ID, "worker", "run-1", time.UnixMilli(3000))
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if beat.Claim == nil || beat.Claim.HeartbeatMS != 3000 {
		t.Fatalf("heartbeat = %+v", beat)
	}

	blocked, err := st.Block(task.ID, "lead", "needs credentials", time.UnixMilli(4000))
	if err != nil {
		t.Fatalf("Block: %v", err)
	}
	if blocked.Status != StatusBlocked || blocked.BlockReason != "needs credentials" || len(blocked.Comments) != 1 {
		t.Fatalf("blocked = %+v", blocked)
	}

	unblocked, err := st.Unblock(task.ID, "lead", time.UnixMilli(5000))
	if err != nil {
		t.Fatalf("Unblock: %v", err)
	}
	if unblocked.Status != StatusReady || unblocked.BlockReason != "" {
		t.Fatalf("unblocked = %+v", unblocked)
	}

	done, err := st.Complete(task.ID, "worker", time.UnixMilli(6000))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if done.Status != StatusDone || done.CompletedMS != 6000 || done.Claim != nil || done.Attempts[0].Status != "done" {
		t.Fatalf("done = %+v", done)
	}

	archived, err := st.Archive(task.ID, "lead", time.UnixMilli(7000))
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if archived.Status != StatusArchived || archived.ArchivedMS != 7000 {
		t.Fatalf("archived = %+v", archived)
	}
	if got := st.List(Filter{}); len(got) != 0 {
		t.Fatalf("default list should hide archived, got %+v", got)
	}
	if got := st.List(Filter{IncludeArchived: true}); len(got) != 1 || got[0].ID != task.ID {
		t.Fatalf("archived list = %+v", got)
	}
}

func TestReviewClearsClaimAndFinishesAttempt(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := st.Create(CreateSpec{Title: "Dispatch me"}, time.UnixMilli(1000))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Claim(task.ID, "worker", "run-1", time.UnixMilli(2000)); err != nil {
		t.Fatal(err)
	}
	review, err := st.Review(task.ID, "worker", "agent returned answer", time.UnixMilli(3000))
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if review.Status != StatusReview || review.Claim != nil || len(review.Attempts) != 1 {
		t.Fatalf("review = %+v", review)
	}
	if review.Attempts[0].Status != "review" || review.Attempts[0].FinishedMS != 3000 || review.Attempts[0].Summary != "agent returned answer" {
		t.Fatalf("attempt = %+v", review.Attempts[0])
	}
	if len(review.Comments) != 1 || review.Comments[0].Body != "ready for review: agent returned answer" {
		t.Fatalf("comments = %+v", review.Comments)
	}
}

func TestFailAppliesRetryPolicyThenEscalates(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := st.Create(CreateSpec{
		Title:       "Retry me",
		Assignee:    "worker",
		RetryPolicy: &RetryPolicy{MaxAttempts: 2, EscalateTo: "lead"},
	}, time.UnixMilli(1000))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Claim(task.ID, "worker", "run-1", time.UnixMilli(2000)); err != nil {
		t.Fatal(err)
	}
	retry, decision, err := st.Fail(task.ID, "worker", "provider timeout", time.UnixMilli(3000))
	if err != nil {
		t.Fatalf("Fail first: %v", err)
	}
	if retry.Status != StatusReady || retry.Claim != nil || retry.Attempts[0].Status != "failed" {
		t.Fatalf("retry task = %+v", retry)
	}
	if !decision.Retry || decision.NextAttempt != 2 || decision.FailureCount != 1 || decision.Exhausted {
		t.Fatalf("first decision = %+v", decision)
	}
	if got := retry.Comments[len(retry.Comments)-1].Body; got != "retry scheduled: attempt 2/2 after failure: provider timeout" {
		t.Fatalf("retry comment = %q", got)
	}

	if _, err := st.Claim(task.ID, "worker", "run-2", time.UnixMilli(4000)); err != nil {
		t.Fatal(err)
	}
	blocked, decision, err := st.Fail(task.ID, "worker", "provider timeout again", time.UnixMilli(5000))
	if err != nil {
		t.Fatalf("Fail second: %v", err)
	}
	if blocked.Status != StatusBlocked || blocked.Claim != nil || !strings.Contains(blocked.BlockReason, "escalate to lead") {
		t.Fatalf("blocked task = %+v", blocked)
	}
	if decision.Retry || !decision.Exhausted || decision.FailureCount != 2 || decision.MaxAttempts != 2 || decision.EscalateTo != "lead" {
		t.Fatalf("second decision = %+v", decision)
	}
	if FailedAttemptCount(blocked) != 2 {
		t.Fatalf("failed attempts = %d", FailedAttemptCount(blocked))
	}
}

func TestDependenciesBlockUntilDoneAndRejectCycles(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main, _, err := st.Create(CreateSpec{Title: "Main"}, time.UnixMilli(1000))
	if err != nil {
		t.Fatal(err)
	}
	prereq, _, err := st.Create(CreateSpec{Title: "Prereq"}, time.UnixMilli(1100))
	if err != nil {
		t.Fatal(err)
	}
	withDep, err := st.AddDependency(main.ID, prereq.ID, time.UnixMilli(1200))
	if err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	if len(withDep.Dependencies) != 1 || withDep.Dependencies[0].ID != prereq.ID {
		t.Fatalf("dependencies = %+v", withDep.Dependencies)
	}
	blocked, err := st.BlockingDependencies(main.ID)
	if err != nil {
		t.Fatalf("BlockingDependencies: %v", err)
	}
	if len(blocked) != 1 || blocked[0].ID != prereq.ID || blocked[0].Status != StatusTriage {
		t.Fatalf("blocked = %+v", blocked)
	}
	if _, err := st.AddDependency(prereq.ID, main.ID, time.UnixMilli(1300)); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle err=%v", err)
	}
	if _, err := st.Complete(prereq.ID, "worker", time.UnixMilli(1400)); err != nil {
		t.Fatalf("Complete prereq: %v", err)
	}
	blocked, err = st.BlockingDependencies(main.ID)
	if err != nil {
		t.Fatalf("BlockingDependencies after complete: %v", err)
	}
	if len(blocked) != 0 {
		t.Fatalf("blocked after complete = %+v", blocked)
	}
}

func TestReclaimStaleClaim(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := st.Create(CreateSpec{Title: "Stale"}, time.UnixMilli(1000))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Claim(task.ID, "worker", "run-1", time.UnixMilli(2000)); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := st.ReclaimStale(task.ID, "lead", time.Second, time.UnixMilli(2500)); !errors.Is(err, ErrClaimFresh) {
		t.Fatalf("fresh reclaim err=%v, want ErrClaimFresh", err)
	}
	reclaimed, err := st.ReclaimStale(task.ID, "lead", time.Second, time.UnixMilli(3100))
	if err != nil {
		t.Fatalf("ReclaimStale: %v", err)
	}
	if reclaimed.Status != StatusReady || reclaimed.Claim != nil || reclaimed.Attempts[0].Status != "stale" {
		t.Fatalf("reclaimed = %+v", reclaimed)
	}
	if len(reclaimed.Comments) != 1 || reclaimed.Comments[0].Body != "reclaimed stale claim from worker" {
		t.Fatalf("comments = %+v", reclaimed.Comments)
	}
}

func TestSweepStaleClaims(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stale, _, err := st.Create(CreateSpec{Title: "Stale"}, time.UnixMilli(1000))
	if err != nil {
		t.Fatal(err)
	}
	fresh, _, err := st.Create(CreateSpec{Title: "Fresh"}, time.UnixMilli(1000))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Claim(stale.ID, "worker", "run-stale", time.UnixMilli(2000)); err != nil {
		t.Fatalf("Claim stale: %v", err)
	}
	if _, err := st.Claim(fresh.ID, "worker", "run-fresh", time.UnixMilli(2900)); err != nil {
		t.Fatalf("Claim fresh: %v", err)
	}
	reclaimed, err := st.SweepStaleClaims("sweeper", time.Second, 100, time.UnixMilli(3100))
	if err != nil {
		t.Fatalf("SweepStaleClaims: %v", err)
	}
	if len(reclaimed) != 1 || reclaimed[0].ID != stale.ID || reclaimed[0].Status != StatusReady || reclaimed[0].Claim != nil {
		t.Fatalf("reclaimed = %+v", reclaimed)
	}
	gotFresh, _ := st.Get(fresh.ID)
	if gotFresh.Status != StatusRunning || gotFresh.Claim == nil {
		t.Fatalf("fresh should still be claimed: %+v", gotFresh)
	}
}

func TestReclaimStaleClaimEscalatesWhenPolicyExhausted(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := st.Create(CreateSpec{
		Title:       "Stale policy",
		Assignee:    "worker",
		RetryPolicy: &RetryPolicy{MaxAttempts: 1, EscalateTo: "lead"},
	}, time.UnixMilli(1000))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Claim(task.ID, "worker", "run-stale", time.UnixMilli(2000)); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	reclaimed, err := st.ReclaimStale(task.ID, "sweeper", time.Second, time.UnixMilli(3100))
	if err != nil {
		t.Fatalf("ReclaimStale: %v", err)
	}
	if reclaimed.Status != StatusBlocked || !strings.Contains(reclaimed.BlockReason, "escalate to lead") {
		t.Fatalf("reclaimed = %+v", reclaimed)
	}
	if FailedAttemptCount(reclaimed) != 1 || reclaimed.Attempts[0].Status != "stale" {
		t.Fatalf("attempts = %+v", reclaimed.Attempts)
	}
}

func TestCommentAndLink(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := st.Create(CreateSpec{Title: "Wire UI"}, time.UnixMilli(1000))
	if err != nil {
		t.Fatal(err)
	}
	task, err = st.Comment(task.ID, "reviewer", "needs screenshots", time.UnixMilli(2000))
	if err != nil {
		t.Fatalf("Comment: %v", err)
	}
	task, err = st.Link(task.ID, "workflow", "deploy-flow", time.UnixMilli(3000))
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if len(task.Comments) != 1 || task.Comments[0].Body != "needs screenshots" {
		t.Fatalf("comments = %+v", task.Comments)
	}
	if len(task.Links) != 1 || task.Links[0].Type != "workflow" || task.Links[0].Target != "deploy-flow" {
		t.Fatalf("links = %+v", task.Links)
	}
}

func TestSeatCreateAndSet(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := st.Create(CreateSpec{Title: "build X", Seat: "builder"}, time.UnixMilli(1000))
	if err != nil {
		t.Fatal(err)
	}
	if task.Seat != "builder" {
		t.Fatalf("seat = %q, want builder", task.Seat)
	}
	changed, err := st.SetSeat(task.ID, "isolated", time.UnixMilli(2000))
	if err != nil {
		t.Fatalf("SetSeat: %v", err)
	}
	if changed.Seat != "isolated" {
		t.Fatalf("seat after set = %q, want isolated", changed.Seat)
	}
	cleared, err := st.SetSeat(task.ID, "", time.UnixMilli(3000))
	if err != nil {
		t.Fatalf("SetSeat clear: %v", err)
	}
	if cleared.Seat != "" {
		t.Fatalf("seat after clear = %q, want empty", cleared.Seat)
	}
}
