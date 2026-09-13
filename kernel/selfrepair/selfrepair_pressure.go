// SPDX-License-Identifier: MIT

// Self-repair per-pressure-kind fingerprint + reason helpers (M846).
// One pair per pressure row type: Degraded + Retry + Routing +
// RoutingUnstable + RoutingForcedFailed + RoutingForcedExhausted.
// Extracted from selfrepair_fingerprints.go during the Day-202 god-file split.
// Public API unchanged.
package selfrepair

import (
	"fmt"
	"strings"

	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// A fingerprint identifies the REPAIR TARGET — this agent, this kind of
// problem — so the cooldown and the MaxAttempts cap can recognise the same
// incident recurring. It must therefore contain only STABLE identity.
//
// Until 2026-08-12 every one of these embedded the incident's live metrics
// (`failures=%d`, `count=%d`, a generation counter) and `LastReason`, the newest
// error string. All of those change on each recurrence, so the fingerprint was
// different every time: `prev.fingerprint == cand.Fingerprint` never matched and
// `previousAutoRepairAttempts` always returned 0. Both guards were inert — an
// agent could be auto-repaired without cooldown and without limit (SR-001). The
// `misconfigured` path built its fingerprint from stable issue strings and
// worked, which is the contrast that gave it away.
//
// The detail lives in the `reason` string, which is journaled alongside; it does
// not belong in the identity.
func autoRepairDegradedFingerprint(_ kernelruntime.DegradedAgent) string {
	return autoRepairFingerprint("degraded")
}

func autoRepairDegradedReason(row kernelruntime.DegradedAgent) string {
	base := fmt.Sprintf(
		"deterministic auto-repair: degraded by %d failed run(s) in the last %d judged run(s) (threshold %d)",
		row.Failures, row.Window, row.Threshold,
	)
	if reason := strings.TrimSpace(row.LastReason); reason != "" {
		base += ": " + autoRepairClip(reason, 220)
	}
	return base
}

func autoRepairRetryFingerprint(_ kernelruntime.RetryPressureAgent) string {
	return autoRepairFingerprint("retry_pressure")
}

func autoRepairRetryReason(row kernelruntime.RetryPressureAgent) string {
	base := fmt.Sprintf(
		"deterministic auto-repair: %d whole-run retry decision(s) in the last %ds (threshold %d)",
		row.Count, row.WindowSec, row.Threshold,
	)
	if row.NextAttempt > 0 && row.MaxAttempts > 0 {
		base += fmt.Sprintf("; latest retry planned attempt %d/%d", row.NextAttempt, row.MaxAttempts)
	}
	if reason := strings.TrimSpace(row.LastReason); reason != "" {
		base += ": " + autoRepairClip(reason, 220)
	}
	return base
}

// The routing kinds keep TaskType: it names WHICH routing table a repair would
// rewrite, so two task types are genuinely separate targets. The chain contents
// and the failing/next model are deliberately excluded — they change precisely
// BECAUSE repair rewrote them, which is the flapping the guard exists to damp.
func autoRepairRoutingFingerprint(row kernelruntime.RoutingPressureAgent) string {
	return autoRepairFingerprint("routing", row.TaskType)
}

func autoRepairRoutingReason(row kernelruntime.RoutingPressureAgent) string {
	base := fmt.Sprintf(
		"deterministic auto-repair: %d model-chain fallback hop(s) in the last %ds (threshold %d)",
		row.Count, row.WindowSec, row.Threshold,
	)
	if taskType := strings.TrimSpace(row.TaskType); taskType != "" {
		base += " for task type " + taskType
	}
	if row.LastFailedModel != "" || row.LastNextModel != "" {
		base += fmt.Sprintf(" — latest hop %s→%s", row.LastFailedModel, row.LastNextModel)
	}
	if reason := strings.TrimSpace(row.LastReason); reason != "" {
		base += ": " + autoRepairClip(reason, 220)
	}
	return base
}

func autoRepairRoutingUnstableFingerprint(row kernelruntime.RoutingUnstableAgent) string {
	return autoRepairFingerprint(
		"routing_unstable",
		row.TaskType,
	)
}

func autoRepairRoutingUnstableReason(row kernelruntime.RoutingUnstableAgent) string {
	base := fmt.Sprintf(
		"deterministic auto-repair: routing remained unstable after %d rollback event(s) in the last %ds",
		row.Count, row.WindowSec,
	)
	if taskType := strings.TrimSpace(row.TaskType); taskType != "" {
		base += " for task type " + taskType
	}
	if len(row.CurrentChain) > 0 {
		base += " — current chain " + strings.Join(row.CurrentChain, "->")
	}
	if len(row.PreviousChain) > 0 {
		base += " (previous stable " + strings.Join(row.PreviousChain, "->") + ")"
	}
	if reason := strings.TrimSpace(row.LastReason); reason != "" {
		base += ": " + autoRepairClip(reason, 220)
	}
	return base
}

func autoRepairRoutingForcedFailedFingerprint(row kernelruntime.RoutingForcedFailedAgent) string {
	return autoRepairFingerprint(
		"routing_forced_failed",
		row.TaskType,
	)
}

func autoRepairRoutingForcedFailedReason(row kernelruntime.RoutingForcedFailedAgent) string {
	base := fmt.Sprintf(
		"deterministic auto-repair: owner-forced chain stayed under routing pressure after probation with %d fallback hop(s) in the last %ds",
		row.Count, row.WindowSec,
	)
	if taskType := strings.TrimSpace(row.TaskType); taskType != "" {
		base += " for task type " + taskType
	}
	if len(row.ForcedChain) > 0 {
		base += " — forced chain " + strings.Join(row.ForcedChain, "->")
	}
	if reason := strings.TrimSpace(row.LastReason); reason != "" {
		base += ": " + autoRepairClip(reason, 220)
	}
	return base
}

func autoRepairRoutingForcedExhaustedFingerprint(row kernelruntime.RoutingForcedExhaustedAgent) string {
	return autoRepairFingerprint(
		"routing_forced_exhausted",
		row.TaskType,
	)
}

func autoRepairRoutingForcedExhaustedReason(row kernelruntime.RoutingForcedExhaustedAgent) string {
	gen := row.ForceGeneration
	if gen <= 0 {
		gen = 1
	}
	base := fmt.Sprintf(
		"deterministic auto-repair: owner-forced chain exhausted after generation %d with %d fallback hop(s) in the last %ds",
		gen, row.Count, row.WindowSec,
	)
	if taskType := strings.TrimSpace(row.TaskType); taskType != "" {
		base += " for task type " + taskType
	}
	if len(row.ForcedChain) > 0 {
		base += " — forced chain " + strings.Join(row.ForcedChain, "->")
	}
	if reason := strings.TrimSpace(row.LastReason); reason != "" {
		base += ": " + autoRepairClip(reason, 220)
	}
	return base
}
