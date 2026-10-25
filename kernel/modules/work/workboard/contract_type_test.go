// SPDX-License-Identifier: MIT

package workboard

import (
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
)

// TestTheRowsAreTheContractRows: every row the module keeps is the
// CONTRACT'S row, not a copy of it. A copy would drift — the module would
// then keep a shape the contract no longer describes, and the agent-facing
// tool (which speaks the contract) would no longer fit it.
func TestTheRowsAreTheContractRows(t *testing.T) {
	for _, c := range []struct {
		name     string
		module   any
		contract any
	}{
		{"Status", Status(""), workboardapi.Status("")},
		{"Task", Task{}, workboardapi.Task{}},
		{"Claim", Claim{}, workboardapi.Claim{}},
		{"Dependency", Dependency{}, workboardapi.Dependency{}},
		{"DependencyState", DependencyState{}, workboardapi.DependencyState{}},
		{"Attempt", Attempt{}, workboardapi.Attempt{}},
		{"Comment", Comment{}, workboardapi.Comment{}},
		{"Link", Link{}, workboardapi.Link{}},
		{"RetryPolicy", RetryPolicy{}, workboardapi.RetryPolicy{}},
		{"RetryDecision", RetryDecision{}, workboardapi.RetryDecision{}},
		{"CreateSpec", CreateSpec{}, workboardapi.CreateSpec{}},
		{"Filter", Filter{}, workboardapi.Filter{}},
	} {
		if reflect.TypeOf(c.module) != reflect.TypeOf(c.contract) {
			t.Errorf("the %s row drifted from the contract: %v vs %v",
				c.name, reflect.TypeOf(c.module), reflect.TypeOf(c.contract))
		}
	}
}

// TestTheLifecycleStatesAreTheContracts: the states are WIRE — they are
// stored in files and printed by the CLI, so the module and the contract must
// name the same eight values.
func TestTheLifecycleStatesAreTheContracts(t *testing.T) {
	for _, c := range []struct {
		name     string
		module   workboardapi.Status
		contract workboardapi.Status
	}{
		{"StatusTriage", StatusTriage, workboardapi.StatusTriage},
		{"StatusTodo", StatusTodo, workboardapi.StatusTodo},
		{"StatusReady", StatusReady, workboardapi.StatusReady},
		{"StatusRunning", StatusRunning, workboardapi.StatusRunning},
		{"StatusBlocked", StatusBlocked, workboardapi.StatusBlocked},
		{"StatusReview", StatusReview, workboardapi.StatusReview},
		{"StatusDone", StatusDone, workboardapi.StatusDone},
		{"StatusArchived", StatusArchived, workboardapi.StatusArchived},
	} {
		if c.module != c.contract {
			t.Errorf("the %s state is %q, want %q", c.name, c.module, c.contract)
		}
	}
}

// TestTheInvalidStatusErrorIsTheContracts: the refusal the module answers
// with is the contract's — the tool maps it to one answer.
func TestTheInvalidStatusErrorIsTheContracts(t *testing.T) {
	if !errors.Is(ErrInvalidStatus, workboardapi.ErrInvalidStatus) {
		t.Errorf("the module's invalid-status error is not the contract's: %v vs %v",
			ErrInvalidStatus, workboardapi.ErrInvalidStatus)
	}
	if ErrInvalidStatus.Error() != workboardapi.ErrInvalidStatus.Error() {
		t.Errorf("the message differs: %q vs %q",
			ErrInvalidStatus.Error(), workboardapi.ErrInvalidStatus.Error())
	}
}

// TestTheParseStatusDelegatesToTheContract: the module's rule is a
// delegation, so the two must agree on EVERY verdict — the accepted status,
// the normalization and the refusal.
func TestTheParseStatusDelegatesToTheContract(t *testing.T) {
	for _, in := range []string{
		"todo", "  TODO  ", "Ready", "archived", "triage", "running", "blocked", "review", "done",
		"", "   ", "bogus", "donee", "to do",
	} {
		gotStatus, gotErr := ParseStatus(in)
		wantStatus, wantErr := workboardapi.ParseStatus(in)
		if gotStatus != wantStatus {
			t.Errorf("ParseStatus(%q) = %q, want %q", in, gotStatus, wantStatus)
		}
		switch {
		case gotErr == nil && wantErr == nil:
		case gotErr == nil || wantErr == nil:
			t.Errorf("ParseStatus(%q): error %v, want %v", in, gotErr, wantErr)
		default:
			// The two refusals are separate wrappings of the same sentinel,
			// so they are compared by what they say and by WHICH error they
			// carry — not by identity.
			if gotErr.Error() != wantErr.Error() {
				t.Errorf("ParseStatus(%q) = %v, want %v", in, gotErr, wantErr)
			}
			if errors.Is(gotErr, workboardapi.ErrInvalidStatus) != errors.Is(wantErr, workboardapi.ErrInvalidStatus) {
				t.Errorf("ParseStatus(%q) carries %v, want %v", in, gotErr, wantErr)
			}
		}
	}
}

// TestTheFailedAttemptCountDelegatesToTheContract: the module's retry rule
// is a delegation, so the two must agree on EVERY task — the count is what
// the retry decision is made on.
func TestTheFailedAttemptCountDelegatesToTheContract(t *testing.T) {
	for _, task := range []Task{
		{},
		{Attempts: []Attempt{{ID: "a1", Status: "failed"}}},
		{Attempts: []Attempt{
			{ID: "a1", Status: "failed"},
			{ID: "a2", Status: "done"},
			{ID: "a3", Status: "stale"},
			{ID: "a4", Status: "FAILED"},
		}},
		{Attempts: []Attempt{{ID: "a1", Status: "running"}}},
	} {
		got := FailedAttemptCount(task)
		want := workboardapi.FailedAttemptCount(task)
		if got != want {
			t.Errorf("FailedAttemptCount(%+v) = %d, want %d", task.Attempts, got, want)
		}
	}
}

// TestTheStoreIsTheContractsPort: the store answers the contract's port, so
// the agent-facing tool can be handed the tasks without ever naming this
// module.
func TestTheStoreIsTheContractsPort(t *testing.T) {
	var _ workboardapi.Store = (*Store)(nil)
}

// TestTheStoreIsTheContractsHostPort: the store also answers the contract's
// HOST port (W3.47) — the twenty verbs the host drives the board with. A port
// change is a compile error here, in the module that implements it.
func TestTheStoreIsTheContractsHostPort(t *testing.T) {
	var _ workboardapi.HostStore = (*Store)(nil)
}

// TestTheNotFoundErrorIsTheContracts: the "no such task" answer is the
// CONTRACT's — the host is the one who reports it, and the host may not name a
// module's error.
func TestTheNotFoundErrorIsTheContracts(t *testing.T) {
	if !errors.Is(ErrNotFound, workboardapi.ErrNotFound) {
		t.Errorf("the module's not-found error is not the contract's: %v vs %v",
			ErrNotFound, workboardapi.ErrNotFound)
	}
	if ErrNotFound.Error() != workboardapi.ErrNotFound.Error() {
		t.Errorf("the message differs: %q vs %q",
			ErrNotFound.Error(), workboardapi.ErrNotFound.Error())
	}
}

// TestTheRetryDecisionDelegatesToTheContract: the retry rule is a delegation,
// so the two must agree on EVERY task and reason — the decision is what
// decides whether a failed task retries, escalates or blocks.
func TestTheRetryDecisionDelegatesToTheContract(t *testing.T) {
	for _, task := range []Task{
		{},
		{RetryPolicy: &RetryPolicy{MaxAttempts: 3}},
		{RetryPolicy: &RetryPolicy{MaxAttempts: 3}, Attempts: []Attempt{{Status: "failed"}}},
		{RetryPolicy: &RetryPolicy{MaxAttempts: 3}, Attempts: []Attempt{{Status: "failed"}, {Status: "failed"}, {Status: "failed"}}},
		{RetryPolicy: &RetryPolicy{EscalateTo: "lead"}, Attempts: []Attempt{{Status: "failed"}}},
	} {
		for _, reason := range []string{"", "boom", "  stale heartbeat  "} {
			got := RetryDecisionFor(task, reason)
			want := workboardapi.RetryDecisionFor(task, reason)
			// The decision carries a policy POINTER, so the two are compared by
			// what they hold, not by identity.
			if !reflect.DeepEqual(got, want) {
				t.Errorf("RetryDecisionFor(%+v, %q) = %+v, want %+v", task.Attempts, reason, got, want)
			}
		}
	}
}
