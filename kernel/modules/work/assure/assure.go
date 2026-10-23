// SPDX-License-Identifier: MIT

// Package assure is the "do-it-for-sure" loop: run a task, verify it was
// actually accomplished, and retry with the verifier's gap fed back — up to a
// bounded number of attempts, stopping the moment a verdict reports the task
// complete (M651).
//
// This is the reliability primitive the owner asked for: "when I write something,
// definitely do it, and repeat as many times as needed until it's done." The loop
// itself is pure and side-effect-free — it takes a run closure and a verify
// closure — so it is trivially testable; the kernel supplies the real closures
// (a governed run + a provider-backed completion judge).
package assure

import (
	"context"
	"strings"

	"github.com/agezt/agezt/kernel/contract/assureapi"
)

// The rows and the verdict parser now live in the contract layer
// (kernel/contract/assureapi) — W3.31. The module keeps their names as
// ALIASES, so there is one row and one spelling of it: the loop here, the
// workboard's task row there, and the agent-facing tool on top.
//
// The LOOP stays here: it is the implementation (it runs a task and feeds the
// verifier's gap back), not a row.

// The rows.
type (
	// Verdict is the completion judgement for one attempt.
	Verdict = assureapi.Verdict
	// Attempt records one run+verify cycle.
	Attempt = assureapi.Attempt
	// Result is the outcome of an assured run.
	Result = assureapi.Result
)

// The attempt budget bounds.
const (
	DefaultMaxAttempts = assureapi.DefaultMaxAttempts
	MaxMaxAttempts     = assureapi.MaxMaxAttempts
)

// RunFn executes one attempt of the task and returns the answer. attempt is
// 1-based; task is the (possibly gap-augmented) instruction for this attempt.
type RunFn func(ctx context.Context, attempt int, task string) (string, error)

// VerifyFn judges whether answer fully accomplishes the ORIGINAL task.
type VerifyFn func(ctx context.Context, task, answer string) (Verdict, error)

// ParseVerdict extracts a Verdict from a model reply that SHOULD be strict JSON
// {"complete":bool,"gap":"..."} but in practice may arrive wrapped in prose or a
// ```json fence. It isolates the first balanced-looking {...} span and unmarshals
// it. The bool is false when no JSON object could be parsed — the caller decides
// how to treat an unparseable verdict (the kernel treats it as "not complete" so
// the bounded loop tries again rather than declaring a false success).
func ParseVerdict(reply string) (Verdict, bool) { return assureapi.ParseVerdict(reply) }

// clampAttempts normalizes the requested attempt budget into [1, MaxMaxAttempts],
// defaulting a non-positive request to DefaultMaxAttempts.
func clampAttempts(n int) int {
	if n < 1 {
		return DefaultMaxAttempts
	}
	if n > MaxMaxAttempts {
		return MaxMaxAttempts
	}
	return n
}

// Until runs the task, verifies completion, and retries with the verifier's gap
// fed back, up to maxAttempts — stopping as soon as a verdict reports complete.
// Verification is always against the ORIGINAL task, never the gap-augmented
// instruction, so "complete" means the real goal was met. A run error aborts the
// loop (it is typically persistent — a down provider — and retrying would only
// burn the budget); a verify error likewise returns what was produced so far.
func Until(ctx context.Context, task string, maxAttempts int, run RunFn, verify VerifyFn) (Result, error) {
	maxAttempts = clampAttempts(maxAttempts)
	var res Result
	cur := task
	for i := 1; i <= maxAttempts; i++ {
		ans, err := run(ctx, i, cur)
		if err != nil {
			return res, err
		}
		res.Answer = ans
		res.Attempts = i

		v, err := verify(ctx, task, ans)
		if err != nil {
			return res, err
		}
		res.History = append(res.History, Attempt{N: i, Answer: ans, Verdict: v})
		if v.Complete {
			res.Complete = true
			res.Gap = ""
			return res, nil
		}
		res.Gap = v.Gap
		cur = retryInstruction(task, v.Gap)
	}
	return res, nil
}

// retryInstruction augments the original task with the verifier's gap so the next
// attempt knows exactly what remained unfinished.
func retryInstruction(task, gap string) string {
	var b strings.Builder
	b.WriteString(task)
	b.WriteString("\n\nYour previous attempt did NOT fully complete this task.")
	if strings.TrimSpace(gap) != "" {
		b.WriteString(" What is still missing: ")
		b.WriteString(strings.TrimSpace(gap))
	}
	b.WriteString("\nFinish the task completely this time.")
	return b.String()
}

// ParseVerdict lives in the contract layer (kernel/contract/assureapi) — it
// parses into the row, so it travels with it. This file keeps the loop.
