// SPDX-License-Identifier: MIT

package selfrepair

// Self-repair fingerprint + reason + routing-rollback helpers (M846):
// the pure-logic helpers used by both the coordinator (claim/handleTick/
// dispatch) and the apply pipeline (autoEscalate/autoWake). Carved out of
// selfrepair.go during the Day 25 god file split #5 so the main file can
// focus on the coordinator state machine.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

func autoRepairPayloadString(pl map[string]any, key string) string {
	if pl == nil {
		return ""
	}
	v, _ := pl[key].(string)
	return strings.TrimSpace(v)
}

func autoRepairEscalationTarget(p roster.Profile) string {
	for _, ref := range []string{strings.TrimSpace(p.ParentAgent), strings.TrimSpace(p.OwnerAgent)} {
		if ref != "" && !strings.EqualFold(ref, p.Slug) {
			return ref
		}
	}
	return ""
}

func autoRepairEscalationFrom(p roster.Profile) string {
	if p.HealthPolicy != nil {
		if doc := strings.TrimSpace(p.HealthPolicy.DoctorAgent); doc != "" {
			return doc
		}
	}
	return "system:doctor"
}

func autoRepairFingerprint(parts ...string) string {
	cp := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			cp = append(cp, part)
		}
	}
	if len(cp) == 0 {
		return ""
	}
	sort.Strings(cp)
	return strings.Join(cp, "\n")
}

func autoRepairReason(issues []string) string {
	base := "deterministic auto-repair: invalid runtime override(s)"
	if len(issues) == 0 {
		return base
	}
	text := strings.Join(issues, "; ")
	if len(text) > 700 {
		text = text[:700] + "..."
	}
	return base + ": " + text
}

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

type autoRepairRoutingRollback struct {
	TaskType  string
	FromChain []string
	ToChain   []string
	Reason    string
}

func autoRepairRoutingRollbackPlan(k *kernelruntime.Kernel, p roster.Profile, row kernelruntime.RoutingPressureAgent, now time.Time) *autoRepairRoutingRollback {
	if k == nil {
		return nil
	}
	taskType := strings.TrimSpace(row.TaskType)
	if taskType == "" {
		taskType = strings.TrimSpace(p.TaskType)
	}
	if taskType == "" {
		return nil
	}
	probation := autoRepairRoutingRollbackProbation()
	if probation <= 0 {
		return nil
	}
	latest := autoRepairLatestRoutingRewrite(k, p.Slug, taskType, now.Add(-probation).UnixMilli())
	if latest == nil || len(latest.PreviousChain) == 0 || len(latest.NewChain) == 0 {
		return nil
	}
	currentChain := autoRepairCurrentTaskModelChain(k, taskType)
	if !autoRepairSameChain(currentChain, latest.NewChain) {
		return nil
	}
	if autoRepairSameChain(latest.PreviousChain, latest.NewChain) {
		return nil
	}
	return &autoRepairRoutingRollback{
		TaskType:  taskType,
		FromChain: append([]string(nil), latest.NewChain...),
		ToChain:   append([]string(nil), latest.PreviousChain...),
		Reason:    autoRepairRoutingRollbackReason(row, latest.PreviousChain),
	}
}

type autoRepairRoutingRewrite struct {
	TSMS          int64
	TaskType      string
	NewChain      []string
	PreviousChain []string
}

func autoRepairLatestForceGeneration(k *kernelruntime.Kernel, slug, taskType string) int {
	if k == nil {
		return 0
	}
	latestTS := int64(0)
	latestGen := 0
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindInfo || (e.Subject != "doctor.auto_repair" && e.Subject != "agent.resolve") {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		phase := plStringMap(pl, "phase")
		if plStringMap(pl, "agent") != slug || (phase != "resolution_applied" && phase != "completed") || plStringMap(pl, "resolution") != "force_chain" || plStringMap(pl, "routing_task_type") != taskType {
			return nil
		}
		gen := plIntAny(pl["routing_force_generation"])
		if gen <= 0 {
			gen = 1
		}
		if e.TSUnixMS >= latestTS {
			latestTS = e.TSUnixMS
			latestGen = gen
		}
		return nil
	})
	return latestGen
}

func autoRepairLatestRoutingRewrite(k *kernelruntime.Kernel, slug, taskType string, cutoffMS int64) *autoRepairRoutingRewrite {
	var latest *autoRepairRoutingRewrite
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.TSUnixMS < cutoffMS {
			return nil
		}
		if e.Subject != "doctor.auto_repair" && e.Subject != "agent.repair" {
			return nil
		}
		if e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil || plStringMap(pl, "agent") != slug {
			return nil
		}
		if plStringMap(pl, "phase") != "completed" || plStringMap(pl, "routing_task_type") != taskType {
			return nil
		}
		newChain := plStringsMap(pl, "routing_task_model_chain")
		prevChain := plStringsMap(pl, "previous_routing_task_model_chain")
		if len(newChain) == 0 || len(prevChain) == 0 {
			return nil
		}
		if latest == nil || e.TSUnixMS >= latest.TSMS {
			latest = &autoRepairRoutingRewrite{
				TSMS:          e.TSUnixMS,
				TaskType:      taskType,
				NewChain:      newChain,
				PreviousChain: prevChain,
			}
		}
		return nil
	})
	return latest
}

func autoRepairCurrentTaskModelChain(k *kernelruntime.Kernel, taskType string) []string {
	taskType = strings.TrimSpace(taskType)
	if k == nil || taskType == "" {
		return nil
	}
	type taskModelChainsSource interface {
		TaskModelChainsView() map[string][]string
	}
	gov, ok := k.Provider().(taskModelChainsSource)
	if !ok {
		return nil
	}
	src := gov.TaskModelChainsView()[taskType]
	if len(src) == 0 {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

func autoRepairSameChain(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(strings.TrimSpace(a[i]), strings.TrimSpace(b[i])) {
			return false
		}
	}
	return len(a) > 0
}

func autoRepairRoutingRollbackReason(row kernelruntime.RoutingPressureAgent, chain []string) string {
	base := autoRepairRoutingReason(row)
	if len(chain) == 0 {
		return base
	}
	return base + " — recurrence after a recent routing rewrite; rolling back to " + strings.Join(chain, " -> ")
}

