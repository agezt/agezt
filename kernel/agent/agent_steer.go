// SPDX-License-Identifier: MIT

// Agent steering: Steerer interface + Directive + DefaultContextProtectLast + DefaultContextProtectFirst + ContextCharsPerToken + DefaultCompressFraction.
// Code extracted from agent.go during the Day-78 god-file split. Public API unchanged.
package agent

import (
	"context"
)


// guidance, pause, single-step, resume — without cancelling it. The kernel
// supplies the live implementation (kernel/runtime); tests can supply a fake.
type Steerer interface {
	// Wait blocks while the run is paused and returns when it is resumed,
	// single-stepped, or ctx is done. It returns ctx.Err() if the context ends
	// while waiting (so a paused run still honours halt/cancel/timeout), and nil
	// otherwise. When the run is not paused it returns immediately.
	Wait(ctx context.Context) error
	// Drain returns and clears any directives the operator has injected since the
	// last call, in submission order. The loop appends each as a user turn before
	// the next model call. Returns nil when none are pending.
	Drain() []Directive
}

// Directive is one operator injection the loop folds into the run at a safe
// boundary (M962). Note distinguishes a soft "by the way" — read it, but finish
// the current step and stay on task — from a forceful steer that re-prioritises.
type Directive struct {
	Text string
	Note bool
}

// DefaultContextProtectLast is how many trailing messages context compaction
// never touches, so the model always keeps its most recent exchange whole.
const DefaultContextProtectLast = 4

// DefaultContextProtectFirst is how many leading messages context compaction
// never touches by default. 0 means protect-first is opt-in: with it unset,
// compaction elides strictly oldest-first and only the tail is shielded.
const DefaultContextProtectFirst = 0

// ContextCharsPerToken is the rough chars-per-token ratio used to translate a
// model's token context window (catalog Limit.Context) into the char-denominated
// budget the loop measures. ~4 is the common English approximation; the budget is
// a soft cap, not an exact token count, so an approximation is fine.
const ContextCharsPerToken = 4

// DefaultCompressFraction is the fraction of the model's context window at which
// auto-budgeting starts compacting (SPEC-16 §3 compress_at_fraction). Half the
// window leaves ample room for the model's own output + a safety margin.
const DefaultCompressFraction = 0.5

