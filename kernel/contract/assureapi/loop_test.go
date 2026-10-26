// SPDX-License-Identifier: MIT

package assureapi

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

// completeAfter returns a VerifyFn that reports the task incomplete for the first
// (n-1) calls and complete on the nth — modelling a task that takes n tries.
func completeAfter(n int) VerifyFn {
	calls := 0
	return func(_ context.Context, _, _ string) (Verdict, error) {
		calls++
		if calls >= n {
			return Verdict{Complete: true}, nil
		}
		return Verdict{Complete: false, Gap: "still missing step"}, nil
	}
}

func TestUntil_CompletesFirstTry(t *testing.T) {
	runs := 0
	res, err := Until(context.Background(), "do it", 3,
		func(_ context.Context, _ int, _ string) (string, error) { runs++; return "done", nil },
		completeAfter(1))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !res.Complete || res.Attempts != 1 || runs != 1 {
		t.Fatalf("want complete in 1 attempt, got %+v (runs=%d)", res, runs)
	}
}

func TestUntil_RetriesThenCompletes(t *testing.T) {
	var lastTask string
	res, err := Until(context.Background(), "ship the feature", 5,
		func(_ context.Context, attempt int, task string) (string, error) {
			lastTask = task
			return "attempt-output", nil
		},
		completeAfter(3))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !res.Complete || res.Attempts != 3 {
		t.Fatalf("want complete on 3rd attempt, got %+v", res)
	}
	if len(res.History) != 3 {
		t.Errorf("history should record all 3 attempts, got %d", len(res.History))
	}
	// The retry instruction must carry the original task AND the gap feedback.
	if !strings.Contains(lastTask, "ship the feature") || !strings.Contains(lastTask, "still missing step") {
		t.Errorf("retry instruction missing task or gap: %q", lastTask)
	}
}

func TestUntil_ExhaustsWithoutCompleting(t *testing.T) {
	res, err := Until(context.Background(), "impossible", 2,
		func(_ context.Context, _ int, _ string) (string, error) { return "partial", nil },
		func(_ context.Context, _, _ string) (Verdict, error) {
			return Verdict{Complete: false, Gap: "everything"}, nil
		})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Complete {
		t.Fatal("should not be complete")
	}
	if res.Attempts != 2 {
		t.Errorf("attempts = %d, want 2 (the budget)", res.Attempts)
	}
	if res.Gap != "everything" {
		t.Errorf("final gap = %q, want the last unmet gap", res.Gap)
	}
	if res.Answer != "partial" {
		t.Errorf("answer = %q, want the last attempt's output", res.Answer)
	}
}

func TestUntil_RunErrorAborts(t *testing.T) {
	boom := errors.New("provider down")
	_, err := Until(context.Background(), "x", 3,
		func(_ context.Context, _ int, _ string) (string, error) { return "", boom },
		completeAfter(1))
	if !errors.Is(err, boom) {
		t.Fatalf("run error should abort the loop, got %v", err)
	}
}

func TestUntil_VerifyErrorReturnsProgress(t *testing.T) {
	boom := errors.New("judge unavailable")
	res, err := Until(context.Background(), "x", 3,
		func(_ context.Context, _ int, _ string) (string, error) { return "ans", nil },
		func(_ context.Context, _, _ string) (Verdict, error) { return Verdict{}, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("verify error should surface, got %v", err)
	}
	if res.Answer != "ans" {
		t.Errorf("should still return the produced answer, got %q", res.Answer)
	}
}

func TestUntil_ClampsAttempts(t *testing.T) {
	// 0 → DefaultMaxAttempts; a huge value → MaxMaxAttempts.
	for _, tc := range []struct{ in, want int }{{0, DefaultMaxAttempts}, {-5, DefaultMaxAttempts}, {999, MaxMaxAttempts}} {
		runs := 0
		Until(context.Background(), "x", tc.in,
			func(_ context.Context, _ int, _ string) (string, error) { runs++; return "p", nil },
			func(_ context.Context, _, _ string) (Verdict, error) { return Verdict{Complete: false, Gap: "g"}, nil })
		if runs != tc.want {
			t.Errorf("maxAttempts %d → ran %d times, want %d", tc.in, runs, tc.want)
		}
	}
}

func TestVerifyAgainstOriginalTask(t *testing.T) {
	// The verifier must always receive the ORIGINAL task, not the gap-augmented one.
	var seen []string
	Until(context.Background(), "ORIGINAL", 3,
		func(_ context.Context, _ int, _ string) (string, error) { return "a", nil },
		func(_ context.Context, task, _ string) (Verdict, error) {
			seen = append(seen, task)
			return Verdict{Complete: false, Gap: "more"}, nil
		})
	for i, task := range seen {
		if task != "ORIGINAL" {
			t.Errorf("verify call %d saw task %q, want ORIGINAL every time", i, task)
		}
	}
}

// TestTheAttemptNumberIsOneBased: the run closure is told which attempt it
// is, and the count starts at ONE — a loop that numbers its attempts from
// zero would report attempt 0 to the agent and to the journal.
func TestTheAttemptNumberIsOneBased(t *testing.T) {
	var seen []int
	Until(context.Background(), "x", 3,
		func(_ context.Context, attempt int, _ string) (string, error) {
			seen = append(seen, attempt)
			return "a", nil
		},
		func(_ context.Context, _, _ string) (Verdict, error) { return Verdict{Complete: false, Gap: "g"}, nil })
	if len(seen) != 3 {
		t.Fatalf("the run closure was called %d times, want 3", len(seen))
	}
	for i, got := range seen {
		if want := i + 1; got != want {
			t.Errorf("attempt %d was numbered %d, want %d", i, got, want)
		}
	}
}

// TestTheHistoryCarriesEachAttempt: the trail is the evidence of what the
// loop tried — the answer and the verdict of every attempt, in order.
func TestTheHistoryCarriesEachAttempt(t *testing.T) {
	res, err := Until(context.Background(), "x", 3,
		func(_ context.Context, attempt int, _ string) (string, error) {
			return "answer-" + strconv.Itoa(attempt), nil
		},
		func(_ context.Context, _, _ string) (Verdict, error) { return Verdict{Complete: false, Gap: "g"}, nil })
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(res.History) != 3 {
		t.Fatalf("history has %d attempts, want 3", len(res.History))
	}
	for i, a := range res.History {
		if want := i + 1; a.N != want {
			t.Errorf("history[%d].N = %d, want %d", i, a.N, want)
		}
		if a.Answer == "" {
			t.Errorf("history[%d] carries no answer", i)
		}
		if a.Verdict.Complete {
			t.Errorf("history[%d] claims completion, want incomplete", i)
		}
	}
}
