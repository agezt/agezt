// SPDX-License-Identifier: MIT

package assure

import (
	"context"
	"errors"
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

func TestParseVerdict(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantOK    bool
		wantDone  bool
		wantGap   string
	}{
		{"strict", `{"complete":true}`, true, true, ""},
		{"with gap", `{"complete":false,"gap":"no tests"}`, true, false, "no tests"},
		{"fenced", "```json\n{\"complete\": true, \"gap\": \"\"}\n```", true, true, ""},
		{"prose wrapped", `Sure! Here is my verdict: {"complete": false, "gap": "x"} hope that helps`, true, false, "x"},
		{"garbage", `I think it is done.`, false, false, ""},
		{"empty", ``, false, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, ok := ParseVerdict(c.in)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if ok && (v.Complete != c.wantDone || v.Gap != c.wantGap) {
				t.Errorf("verdict = %+v, want complete=%v gap=%q", v, c.wantDone, c.wantGap)
			}
		})
	}
}
