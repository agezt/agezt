// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/tools/overseertool"
)

const (
	autoRepairPulseSubject          = "pulse.observer.system:reaper"
	autoRepairEventSubject          = "doctor.auto_repair"
	defaultAutoRepairCooldown       = 30 * time.Minute
	defaultRoutingRollbackProbation = 2 * time.Hour
	autoRepairReaperWindow          = 30 * 24 * time.Hour
)

type autoRepairSource interface {
	RepairAgent(ref, reason string) (overseertool.RepairResult, error)
}

type autoRepairRoutingRollbacker interface {
	RollbackRouting(ref, taskType string, targetChain []string, reason string) (overseertool.RepairResult, error)
}

type autoRepairRoutingChainApplier interface {
	ApplyRoutingChain(ref, taskType string, targetChain []string, reason string) (overseertool.RepairResult, error)
}

type autoRepairMailbox interface {
	HelpRequest(from, to, text string, nowMS int64) (board.Message, error)
	Get(id string) (board.Message, bool)
	Send(m board.Message, nowMS int64) (board.Message, error)
}

type autoRepairWakeResult struct {
	Target      string
	Correlation string
	Answer      string
	Resolution  *autoRepairResolution
	Skipped     string
	// Runbook is the woken target's autonomy contract (M-doctor-wake), attached to
	// escalation_woke / delegation_woke evidence so a doctor-triggered wake folds
	// into the woken agent's status like schedule/standing/mailbox/delegated wakes.
	Runbook map[string]any
}

type autoRepairResolution struct {
	Resolution     string   `json:"resolution"`
	Summary        string   `json:"summary"`
	DelegateTo     string   `json:"delegate_to"`
	TaskType       string   `json:"task_type"`
	TaskModelChain []string `json:"task_model_chain"`
}

type autoRepairResolutionOutcome struct {
	Phase                          string
	RoutingTaskType                string
	RoutingTaskModelChain          []string
	PreviousRoutingTaskModelChain  []string
	RoutingForceGeneration         int
	PreviousRoutingForceGeneration int
}

type autoRepairCoordinator struct {
	mu       sync.Mutex
	cooldown time.Duration
	now      func() time.Time
	inflight map[string]struct{}
	last     map[string]autoRepairStamp
}

type autoRepairStamp struct {
	fingerprint string
	at          time.Time
}

type autoRepairCandidate struct {
	Slug                     string
	Mode                     string
	Issues                   []string
	Fingerprint              string
	Reason                   string
	SelfRepairAttempt        int
	SelfRepairMaxAttempts    int
	SelfRepairExhausted      bool
	EscalateTo               string
	EscalateFrom             string
	RootAgent                string
	ChainDepth               int
	IncidentID               string
	RootChainID              string
	ParentHopID              string
	RoutingRollbackTaskType  string
	RoutingRollbackFromChain []string
	RoutingRollbackToChain   []string
}

func wireAutoRepair(ctx context.Context, k *kernelruntime.Kernel, baseDir string, mailbox autoRepairMailbox, postNotify func(board.Message, string)) string {
	if k == nil || k.Bus() == nil {
		return "disabled"
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv(brand.EnvPrefix+"AUTO_REPAIR")), "off") {
		return "disabled (AGEZT_AUTO_REPAIR=off)"
	}
	sub, err := k.Bus().Subscribe(autoRepairPulseSubject, 64)
	if err != nil {
		return fmt.Sprintf("disabled (subscribe failed: %v)", err)
	}
	coord := newAutoRepairCoordinator(autoRepairCooldown())
	src := overseertool.NewKernelSource(k, baseDir)
	go coord.run(ctx, sub, k, src, mailbox, postNotify)
	return fmt.Sprintf("armed (%s; cooldown %s)", autoRepairPulseSubject, coord.cooldown)
}

func newAutoRepairCoordinator(cooldown time.Duration) *autoRepairCoordinator {
	if cooldown <= 0 {
		cooldown = defaultAutoRepairCooldown
	}
	return &autoRepairCoordinator{
		cooldown: cooldown,
		now:      time.Now,
		inflight: map[string]struct{}{},
		last:     map[string]autoRepairStamp{},
	}
}

func autoRepairCooldown() time.Duration {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "AUTO_REPAIR_COOLDOWN"))
	if raw == "" {
		return defaultAutoRepairCooldown
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultAutoRepairCooldown
	}
	return d
}

func autoRepairRoutingRollbackProbation() time.Duration {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ROUTING_ROLLBACK_PROBATION"))
	if raw == "" {
		return defaultRoutingRollbackProbation
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultRoutingRollbackProbation
	}
	return d
}

func (c *autoRepairCoordinator) run(ctx context.Context, sub *bus.Subscription, k *kernelruntime.Kernel, src autoRepairSource, mailbox autoRepairMailbox, postNotify func(board.Message, string)) {
	defer sub.Cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.C:
			if !ok {
				return
			}
			if !autoRepairShouldHandle(ev) {
				continue
			}
			cut := c.now().Add(-autoRepairReaperWindow).UnixMilli()
			rep := k.ReaperScan(cut, cut)
			for _, cand := range c.claim(k, rep, k.Roster().List()) {
				publishAutoRepair(k.Bus(), "", map[string]any{
					"phase":                             autoRepairQueuedPhase(cand),
					"agent":                             cand.Slug,
					"mode":                              cand.Mode,
					"issues":                            cand.Issues,
					"reason":                            cand.Reason,
					"fingerprint":                       cand.Fingerprint,
					"self_repair_attempt":               cand.SelfRepairAttempt,
					"self_repair_max_attempts":          cand.SelfRepairMaxAttempts,
					"routing_task_type":                 cand.RoutingRollbackTaskType,
					"routing_task_model_chain":          cand.RoutingRollbackToChain,
					"previous_routing_task_model_chain": cand.RoutingRollbackFromChain,
					"incident_id":                       autoRepairIncidentIDValue(cand),
					"root_incident_id":                  autoRepairRootChainID(cand),
					"parent_incident_id":                strings.TrimSpace(cand.ParentHopID),
				})
				go c.dispatch(ctx, k, k.Bus(), src, mailbox, postNotify, cand)
			}
		}
	}
}

func autoRepairShouldHandle(ev *event.Event) bool {
	if ev == nil || ev.Subject != autoRepairPulseSubject || len(ev.Payload) == 0 {
		return false
	}
	var payload struct {
		Kind  string `json:"kind"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return false
	}
	return payload.Error == "" && payload.Kind == "reaper_candidates"
}

func (c *autoRepairCoordinator) claim(k *kernelruntime.Kernel, rep kernelruntime.ReaperReport, profiles []roster.Profile) []autoRepairCandidate {
	bySlug := make(map[string]roster.Profile, len(profiles))
	for _, p := range profiles {
		bySlug[p.Slug] = p
	}
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []autoRepairCandidate
	claimed := map[string]bool{}
	var claimOK bool
	for _, row := range rep.MisconfiguredAgents {
		p, ok := bySlug[row.Slug]
		if !ok || !autoRepairEligible(p) {
			continue
		}
		fp := autoRepairFingerprint(append([]string{"misconfigured"}, row.Issues...)...)
		incidentID := autoRepairIncidentID(p.Slug, fp, now)
		cand := autoRepairCandidate{
			Slug:         p.Slug,
			Mode:         "misconfigured",
			Issues:       append([]string(nil), row.Issues...),
			Fingerprint:  fp,
			Reason:       autoRepairReason(row.Issues),
			EscalateTo:   autoRepairEscalationTarget(p),
			EscalateFrom: autoRepairEscalationFrom(p),
			RootAgent:    p.Slug,
			ChainDepth:   0,
			IncidentID:   incidentID,
			RootChainID:  incidentID,
		}
		if cand, claimOK = c.claimOne(k, now, cand, p); !claimOK {
			continue
		}
		claimed[p.Slug] = true
		out = append(out, cand)
	}
	for _, row := range rep.RoutingUnstable {
		if claimed[row.Slug] {
			continue
		}
		p, ok := bySlug[row.Slug]
		if !ok || !autoRepairEligible(p) {
			continue
		}
		fp := autoRepairRoutingUnstableFingerprint(row)
		incidentID := autoRepairIncidentID(p.Slug, fp, now)
		cand := autoRepairCandidate{
			Slug:         p.Slug,
			Mode:         "routing_unstable",
			Fingerprint:  fp,
			Reason:       autoRepairRoutingUnstableReason(row),
			EscalateTo:   autoRepairEscalationTarget(p),
			EscalateFrom: autoRepairEscalationFrom(p),
			RootAgent:    p.Slug,
			ChainDepth:   0,
			IncidentID:   incidentID,
			RootChainID:  incidentID,
		}
		if cand, claimOK = c.claimOne(k, now, cand, p); !claimOK {
			continue
		}
		claimed[p.Slug] = true
		out = append(out, cand)
	}
	for _, row := range rep.RoutingForcedExhausted {
		if claimed[row.Slug] {
			continue
		}
		p, ok := bySlug[row.Slug]
		if !ok || !autoRepairEligible(p) {
			continue
		}
		fp := autoRepairRoutingForcedExhaustedFingerprint(row)
		incidentID := autoRepairIncidentID(p.Slug, fp, now)
		cand := autoRepairCandidate{
			Slug:                    p.Slug,
			Mode:                    "routing_forced_exhausted",
			Fingerprint:             fp,
			Reason:                  autoRepairRoutingForcedExhaustedReason(row),
			EscalateTo:              autoRepairEscalationTarget(p),
			EscalateFrom:            autoRepairEscalationFrom(p),
			RootAgent:               p.Slug,
			ChainDepth:              0,
			IncidentID:              incidentID,
			RootChainID:             incidentID,
			RoutingRollbackTaskType: row.TaskType,
			RoutingRollbackToChain:  append([]string(nil), row.ForcedChain...),
		}
		if cand, claimOK = c.claimOne(k, now, cand, p); !claimOK {
			continue
		}
		claimed[p.Slug] = true
		out = append(out, cand)
	}
	for _, row := range rep.RoutingForcedFailed {
		if claimed[row.Slug] {
			continue
		}
		p, ok := bySlug[row.Slug]
		if !ok || !autoRepairEligible(p) {
			continue
		}
		fp := autoRepairRoutingForcedFailedFingerprint(row)
		incidentID := autoRepairIncidentID(p.Slug, fp, now)
		cand := autoRepairCandidate{
			Slug:                    p.Slug,
			Mode:                    "routing_forced_failed",
			Fingerprint:             fp,
			Reason:                  autoRepairRoutingForcedFailedReason(row),
			EscalateTo:              autoRepairEscalationTarget(p),
			EscalateFrom:            autoRepairEscalationFrom(p),
			RootAgent:               p.Slug,
			ChainDepth:              0,
			IncidentID:              incidentID,
			RootChainID:             incidentID,
			RoutingRollbackTaskType: row.TaskType,
			RoutingRollbackToChain:  append([]string(nil), row.ForcedChain...),
		}
		if cand, claimOK = c.claimOne(k, now, cand, p); !claimOK {
			continue
		}
		claimed[p.Slug] = true
		out = append(out, cand)
	}
	for _, row := range rep.RoutingPressure {
		if claimed[row.Slug] {
			continue
		}
		p, ok := bySlug[row.Slug]
		if !ok || !autoRepairEligible(p) {
			continue
		}
		fp := autoRepairRoutingFingerprint(row)
		incidentID := autoRepairIncidentID(p.Slug, fp, now)
		cand := autoRepairCandidate{
			Slug:         p.Slug,
			Mode:         "routing",
			Fingerprint:  fp,
			Reason:       autoRepairRoutingReason(row),
			EscalateTo:   autoRepairEscalationTarget(p),
			EscalateFrom: autoRepairEscalationFrom(p),
			RootAgent:    p.Slug,
			ChainDepth:   0,
			IncidentID:   incidentID,
			RootChainID:  incidentID,
		}
		if plan := autoRepairRoutingRollbackPlan(k, p, row, now); plan != nil {
			cand.RoutingRollbackTaskType = plan.TaskType
			cand.RoutingRollbackFromChain = append([]string(nil), plan.FromChain...)
			cand.RoutingRollbackToChain = append([]string(nil), plan.ToChain...)
			cand.Reason = plan.Reason
		}
		if cand, claimOK = c.claimOne(k, now, cand, p); !claimOK {
			continue
		}
		claimed[p.Slug] = true
		out = append(out, cand)
	}
	for _, row := range rep.RetryPressure {
		if claimed[row.Slug] {
			continue // config/routing signals take priority when both exist
		}
		p, ok := bySlug[row.Slug]
		if !ok || !autoRepairEligible(p) {
			continue
		}
		fp := autoRepairRetryFingerprint(row)
		incidentID := autoRepairIncidentID(p.Slug, fp, now)
		cand := autoRepairCandidate{
			Slug:         p.Slug,
			Mode:         "retry_pressure",
			Fingerprint:  fp,
			Reason:       autoRepairRetryReason(row),
			EscalateTo:   autoRepairEscalationTarget(p),
			EscalateFrom: autoRepairEscalationFrom(p),
			RootAgent:    p.Slug,
			ChainDepth:   0,
			IncidentID:   incidentID,
			RootChainID:  incidentID,
		}
		if cand, claimOK = c.claimOne(k, now, cand, p); !claimOK {
			continue
		}
		claimed[p.Slug] = true
		out = append(out, cand)
	}
	for _, row := range rep.DegradedAgents {
		if claimed[row.Slug] {
			continue // config/routing/retry signals take priority when both exist
		}
		p, ok := bySlug[row.Slug]
		if !ok || !autoRepairEligible(p) {
			continue
		}
		fp := autoRepairDegradedFingerprint(row)
		incidentID := autoRepairIncidentID(p.Slug, fp, now)
		cand := autoRepairCandidate{
			Slug:         p.Slug,
			Mode:         "degraded",
			Fingerprint:  fp,
			Reason:       autoRepairDegradedReason(row),
			EscalateTo:   autoRepairEscalationTarget(p),
			EscalateFrom: autoRepairEscalationFrom(p),
			RootAgent:    p.Slug,
			ChainDepth:   0,
			IncidentID:   incidentID,
			RootChainID:  incidentID,
		}
		if cand, claimOK = c.claimOne(k, now, cand, p); !claimOK {
			continue
		}
		claimed[p.Slug] = true
		out = append(out, cand)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

func (c *autoRepairCoordinator) claimOne(k *kernelruntime.Kernel, now time.Time, cand autoRepairCandidate, p roster.Profile) (autoRepairCandidate, bool) {
	if _, busy := c.inflight[cand.Slug]; busy {
		return cand, false
	}
	if prev, ok := c.last[cand.Slug]; ok && prev.fingerprint == cand.Fingerprint && now.Sub(prev.at) < c.cooldown {
		return cand, false
	}
	max := autoRepairMaxAttempts(p)
	attempts := previousAutoRepairAttempts(k, cand.Slug, cand.Fingerprint)
	cand.SelfRepairMaxAttempts = max
	cand.SelfRepairAttempt = attempts + 1
	if max > 0 && attempts >= max {
		cand.SelfRepairAttempt = attempts
		cand.SelfRepairExhausted = true
		if cand.Reason != "" {
			cand.Reason += "; "
		}
		cand.Reason += fmt.Sprintf("self-repair attempts exhausted (%d/%d)", attempts, max)
	}
	c.inflight[cand.Slug] = struct{}{}
	c.last[cand.Slug] = autoRepairStamp{fingerprint: cand.Fingerprint, at: now}
	return cand, true
}

func autoRepairEligible(p roster.Profile) bool {
	if p.System || !p.Enabled || p.Retired || !p.AllowsDirectCall() {
		return false
	}
	return p.SelfRepairPolicy != nil && p.SelfRepairPolicy.Enabled
}

func autoRepairMaxAttempts(p roster.Profile) int {
	if p.SelfRepairPolicy == nil || p.SelfRepairPolicy.MaxAttempts <= 0 {
		return 0
	}
	return p.SelfRepairPolicy.MaxAttempts
}

func previousAutoRepairAttempts(k *kernelruntime.Kernel, slug, fingerprint string) int {
	if k == nil || k.Journal() == nil || strings.TrimSpace(slug) == "" || strings.TrimSpace(fingerprint) == "" {
		return 0
	}
	count := 0
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindInfo || e.Subject != autoRepairEventSubject {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		if strings.TrimSpace(autoRepairPayloadString(pl, "agent")) != slug || strings.TrimSpace(autoRepairPayloadString(pl, "fingerprint")) != fingerprint {
			return nil
		}
		switch strings.TrimSpace(autoRepairPayloadString(pl, "phase")) {
		case "queued", "routing_rollback_queued":
			count++
		}
		return nil
	})
	return count
}

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

func autoRepairDegradedFingerprint(row kernelruntime.DegradedAgent) string {
	return autoRepairFingerprint(
		"degraded",
		fmt.Sprintf("failures=%d", row.Failures),
		fmt.Sprintf("window=%d", row.Window),
		fmt.Sprintf("threshold=%d", row.Threshold),
		row.LastReason,
	)
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

func autoRepairRetryFingerprint(row kernelruntime.RetryPressureAgent) string {
	return autoRepairFingerprint(
		"retry_pressure",
		fmt.Sprintf("count=%d", row.Count),
		fmt.Sprintf("threshold=%d", row.Threshold),
		fmt.Sprintf("max_attempts=%d", row.MaxAttempts),
		row.LastReason,
	)
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

func autoRepairRoutingFingerprint(row kernelruntime.RoutingPressureAgent) string {
	return autoRepairFingerprint(
		"routing",
		fmt.Sprintf("count=%d", row.Count),
		fmt.Sprintf("threshold=%d", row.Threshold),
		row.TaskType,
		row.LastFailedModel,
		row.LastNextModel,
		row.LastReason,
	)
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
		fmt.Sprintf("count=%d", row.Count),
		fmt.Sprintf("threshold=%d", row.Threshold),
		row.TaskType,
		strings.Join(row.CurrentChain, "->"),
		strings.Join(row.PreviousChain, "->"),
		row.LastReason,
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
		fmt.Sprintf("count=%d", row.Count),
		fmt.Sprintf("threshold=%d", row.Threshold),
		row.TaskType,
		strings.Join(row.ForcedChain, "->"),
		row.LastReason,
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
		fmt.Sprintf("count=%d", row.Count),
		fmt.Sprintf("threshold=%d", row.Threshold),
		fmt.Sprintf("generation=%d", row.ForceGeneration),
		row.TaskType,
		strings.Join(row.ForcedChain, "->"),
		row.LastReason,
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

func autoRepairClip(s string, max int) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func autoRepairQueuedPhase(cand autoRepairCandidate) string {
	if cand.SelfRepairExhausted {
		return "attempts_exhausted"
	}
	if cand.Mode == "routing_forced_failed" {
		return "routing_forced_failed_detected"
	}
	if cand.Mode == "routing_forced_exhausted" {
		return "routing_force_exhausted_detected"
	}
	if cand.Mode == "routing_unstable" {
		return "routing_unstable_detected"
	}
	if cand.RoutingRollbackTaskType != "" {
		return "routing_rollback_queued"
	}
	return "queued"
}

func autoRepairCompletedPhase(cand autoRepairCandidate) string {
	if cand.RoutingRollbackTaskType != "" {
		return "routing_rollback_completed"
	}
	return "completed"
}

func autoRepairFailedPhase(cand autoRepairCandidate) string {
	if cand.RoutingRollbackTaskType != "" {
		return "routing_rollback_failed"
	}
	return "failed"
}

func firstNonEmptyStrings(primary, fallback []string) []string {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			return item
		}
	}
	return ""
}

func (c *autoRepairCoordinator) dispatch(ctx context.Context, k *kernelruntime.Kernel, b *bus.Bus, src autoRepairSource, mailbox autoRepairMailbox, postNotify func(board.Message, string), cand autoRepairCandidate) {
	defer c.release(cand.Slug)
	if cand.SelfRepairExhausted {
		err := fmt.Errorf("self-repair attempts exhausted (%d/%d)", cand.SelfRepairAttempt, cand.SelfRepairMaxAttempts)
		msg, mailboxErr := c.autoEscalate(mailbox, postNotify, cand, err)
		c.autoWakeManager(ctx, k, b, src, mailbox, postNotify, cand, msg, mailboxErr)
		return
	}
	if cand.Mode == "routing_unstable" || cand.Mode == "routing_forced_failed" || cand.Mode == "routing_forced_exhausted" {
		msg, mailboxErr := c.autoEscalate(mailbox, postNotify, cand, errors.New(cand.Reason))
		c.autoWakeManager(ctx, k, b, src, mailbox, postNotify, cand, msg, mailboxErr)
		return
	}
	var (
		res overseertool.RepairResult
		err error
	)
	if cand.RoutingRollbackTaskType != "" && len(cand.RoutingRollbackToChain) > 0 {
		rb, ok := src.(autoRepairRoutingRollbacker)
		if !ok {
			err = fmt.Errorf("routing rollback is not supported by the active repair source")
		} else {
			res, err = rb.RollbackRouting(cand.Slug, cand.RoutingRollbackTaskType, cand.RoutingRollbackToChain, cand.Reason)
		}
	} else {
		res, err = src.RepairAgent(cand.Slug, cand.Reason)
	}
	if err != nil {
		publishAutoRepair(b, "", map[string]any{
			"phase":                             autoRepairFailedPhase(cand),
			"agent":                             cand.Slug,
			"mode":                              cand.Mode,
			"issues":                            cand.Issues,
			"reason":                            cand.Reason,
			"fingerprint":                       cand.Fingerprint,
			"self_repair_attempt":               cand.SelfRepairAttempt,
			"self_repair_max_attempts":          cand.SelfRepairMaxAttempts,
			"error":                             err.Error(),
			"routing_task_type":                 firstNonEmpty(cand.RoutingRollbackTaskType, res.RoutingTaskType),
			"routing_task_model_chain":          firstNonEmptyStrings(cand.RoutingRollbackToChain, res.RoutingTaskModelChain),
			"previous_routing_task_model_chain": firstNonEmptyStrings(cand.RoutingRollbackFromChain, res.PreviousRoutingTaskModelChain),
			"incident_id":                       autoRepairIncidentIDValue(cand),
			"root_incident_id":                  autoRepairRootChainID(cand),
			"parent_incident_id":                strings.TrimSpace(cand.ParentHopID),
		})
		msg, mailboxErr := c.autoEscalate(mailbox, postNotify, cand, err)
		c.autoWakeManager(ctx, k, b, src, mailbox, postNotify, cand, msg, mailboxErr)
		return
	}
	publishAutoRepair(b, res.Correlation, map[string]any{
		"phase":                             autoRepairCompletedPhase(cand),
		"agent":                             cand.Slug,
		"mode":                              cand.Mode,
		"issues":                            cand.Issues,
		"reason":                            cand.Reason,
		"fingerprint":                       cand.Fingerprint,
		"self_repair_attempt":               cand.SelfRepairAttempt,
		"self_repair_max_attempts":          cand.SelfRepairMaxAttempts,
		"applied":                           res.Applied,
		"routing_task_type":                 res.RoutingTaskType,
		"routing_task_model_chain":          res.RoutingTaskModelChain,
		"previous_routing_task_model_chain": res.PreviousRoutingTaskModelChain,
		"answer":                            res.Answer,
		"incident_id":                       autoRepairIncidentIDValue(cand),
		"root_incident_id":                  autoRepairRootChainID(cand),
		"parent_incident_id":                strings.TrimSpace(cand.ParentHopID),
	})
}

func (c *autoRepairCoordinator) autoEscalate(mailbox autoRepairMailbox, postNotify func(board.Message, string), cand autoRepairCandidate, repairErr error) (*board.Message, error) {
	if mailbox == nil {
		return nil, nil
	}
	target := strings.TrimSpace(cand.EscalateTo)
	msg, err := mailbox.HelpRequest(cand.EscalateFrom, target, autoRepairEscalationText(cand, repairErr), c.now().UnixMilli())
	if err != nil {
		return nil, err
	}
	if postNotify != nil {
		postNotify(msg, "")
	}
	return &msg, nil
}

func (c *autoRepairCoordinator) autoWakeManager(ctx context.Context, k *kernelruntime.Kernel, b *bus.Bus, src autoRepairSource, mailbox autoRepairMailbox, postNotify func(board.Message, string), cand autoRepairCandidate, msg *board.Message, mailboxErr error) {
	res, err := autoRepairWakeAgent(ctx, k, cand, msg)
	if err != nil {
		publishAutoRepair(b, res.Correlation, map[string]any{
			"phase":                    "escalation_failed",
			"agent":                    cand.Slug,
			"mode":                     cand.Mode,
			"root_agent":               autoRepairRootAgent(cand),
			"chain_depth":              cand.ChainDepth,
			"incident_id":              autoRepairIncidentIDValue(cand),
			"root_incident_id":         autoRepairRootChainID(cand),
			"parent_incident_id":       strings.TrimSpace(cand.ParentHopID),
			"target_agent":             res.Target,
			"target_correlation":       res.Correlation,
			"fingerprint":              cand.Fingerprint,
			"self_repair_attempt":      cand.SelfRepairAttempt,
			"self_repair_max_attempts": cand.SelfRepairMaxAttempts,
			"reason":                   err.Error(),
			"mailbox_error":            autoRepairErrString(mailboxErr),
			"mailbox_message_id":       autoRepairMessageID(msg),
		})
		return
	}
	if res.Skipped != "" {
		publishAutoRepair(b, "", map[string]any{
			"phase":                    "escalation_skipped",
			"agent":                    cand.Slug,
			"mode":                     cand.Mode,
			"root_agent":               autoRepairRootAgent(cand),
			"chain_depth":              cand.ChainDepth,
			"incident_id":              autoRepairIncidentIDValue(cand),
			"root_incident_id":         autoRepairRootChainID(cand),
			"parent_incident_id":       strings.TrimSpace(cand.ParentHopID),
			"target_agent":             res.Target,
			"target_correlation":       res.Correlation,
			"fingerprint":              cand.Fingerprint,
			"self_repair_attempt":      cand.SelfRepairAttempt,
			"self_repair_max_attempts": cand.SelfRepairMaxAttempts,
			"reason":                   res.Skipped,
			"mailbox_error":            autoRepairErrString(mailboxErr),
			"mailbox_message_id":       autoRepairMessageID(msg),
		})
		return
	}
	if res.Target == "" {
		return
	}
	publishAutoRepair(b, res.Correlation, map[string]any{
		"phase":                    "escalation_woke",
		"agent":                    cand.Slug,
		"mode":                     cand.Mode,
		"root_agent":               autoRepairRootAgent(cand),
		"chain_depth":              cand.ChainDepth,
		"incident_id":              autoRepairIncidentIDValue(cand),
		"root_incident_id":         autoRepairRootChainID(cand),
		"parent_incident_id":       strings.TrimSpace(cand.ParentHopID),
		"target_agent":             res.Target,
		"target_correlation":       res.Correlation,
		"fingerprint":              cand.Fingerprint,
		"self_repair_attempt":      cand.SelfRepairAttempt,
		"self_repair_max_attempts": cand.SelfRepairMaxAttempts,
		"mailbox_error":            autoRepairErrString(mailboxErr),
		"mailbox_message_id":       autoRepairMessageID(msg),
		"wake_source":              "doctor",
		"autonomy_runbook":         res.Runbook,
	})
	if reply, err := c.autoReplyEscalation(mailbox, postNotify, res, msg); err == nil && reply != nil {
		payload := map[string]any{
			"phase":                    "escalation_answered",
			"agent":                    cand.Slug,
			"mode":                     cand.Mode,
			"root_agent":               autoRepairRootAgent(cand),
			"chain_depth":              cand.ChainDepth,
			"incident_id":              autoRepairIncidentIDValue(cand),
			"root_incident_id":         autoRepairRootChainID(cand),
			"parent_incident_id":       strings.TrimSpace(cand.ParentHopID),
			"target_agent":             res.Target,
			"target_correlation":       res.Correlation,
			"answer":                   autoRepairClip(res.Answer, 800),
			"fingerprint":              cand.Fingerprint,
			"self_repair_attempt":      cand.SelfRepairAttempt,
			"self_repair_max_attempts": cand.SelfRepairMaxAttempts,
			"mailbox_message_id":       autoRepairMessageID(msg),
			"reply_message_id":         strings.TrimSpace(reply.ID),
		}
		if res.Resolution != nil {
			payload["resolution"] = res.Resolution.Resolution
			payload["resolution_summary"] = res.Resolution.Summary
			if res.Resolution.DelegateTo != "" {
				payload["delegate_to"] = res.Resolution.DelegateTo
			}
			if res.Resolution.TaskType != "" {
				payload["routing_task_type"] = res.Resolution.TaskType
			}
			if len(res.Resolution.TaskModelChain) > 0 {
				payload["routing_task_model_chain"] = res.Resolution.TaskModelChain
			}
		}
		publishAutoRepair(b, res.Correlation, payload)
		outcome, err := c.applyAutoRepairResolution(ctx, k, src, mailbox, postNotify, cand, res)
		if err != nil {
			fail := map[string]any{
				"phase":                    "resolution_failed",
				"agent":                    cand.Slug,
				"mode":                     cand.Mode,
				"root_agent":               autoRepairRootAgent(cand),
				"chain_depth":              cand.ChainDepth,
				"incident_id":              autoRepairIncidentIDValue(cand),
				"root_incident_id":         autoRepairRootChainID(cand),
				"parent_incident_id":       strings.TrimSpace(cand.ParentHopID),
				"target_agent":             res.Target,
				"target_correlation":       res.Correlation,
				"fingerprint":              cand.Fingerprint,
				"self_repair_attempt":      cand.SelfRepairAttempt,
				"self_repair_max_attempts": cand.SelfRepairMaxAttempts,
				"reason":                   err.Error(),
				"mailbox_message_id":       autoRepairMessageID(msg),
			}
			if res.Resolution != nil {
				fail["resolution"] = res.Resolution.Resolution
				fail["resolution_summary"] = res.Resolution.Summary
				if res.Resolution.DelegateTo != "" {
					fail["delegate_to"] = res.Resolution.DelegateTo
				}
				if res.Resolution.TaskType != "" {
					fail["routing_task_type"] = res.Resolution.TaskType
				}
				if len(res.Resolution.TaskModelChain) > 0 {
					fail["routing_task_model_chain"] = res.Resolution.TaskModelChain
				}
			}
			publishAutoRepair(b, res.Correlation, fail)
		} else if outcome != nil {
			applied := map[string]any{
				"phase":                    firstNonEmpty(outcome.Phase, "resolution_applied"),
				"agent":                    cand.Slug,
				"mode":                     cand.Mode,
				"root_agent":               autoRepairRootAgent(cand),
				"chain_depth":              cand.ChainDepth,
				"incident_id":              autoRepairIncidentIDValue(cand),
				"root_incident_id":         autoRepairRootChainID(cand),
				"parent_incident_id":       strings.TrimSpace(cand.ParentHopID),
				"target_agent":             res.Target,
				"target_correlation":       res.Correlation,
				"fingerprint":              cand.Fingerprint,
				"self_repair_attempt":      cand.SelfRepairAttempt,
				"self_repair_max_attempts": cand.SelfRepairMaxAttempts,
				"resolution":               res.Resolution.Resolution,
				"resolution_summary":       res.Resolution.Summary,
				"mailbox_message_id":       autoRepairMessageID(msg),
			}
			if res.Resolution.DelegateTo != "" {
				applied["delegate_to"] = res.Resolution.DelegateTo
			}
			if outcome.RoutingTaskType != "" {
				applied["routing_task_type"] = outcome.RoutingTaskType
			}
			if len(outcome.RoutingTaskModelChain) > 0 {
				applied["routing_task_model_chain"] = outcome.RoutingTaskModelChain
			}
			if len(outcome.PreviousRoutingTaskModelChain) > 0 {
				applied["previous_routing_task_model_chain"] = outcome.PreviousRoutingTaskModelChain
			}
			if outcome.RoutingForceGeneration > 0 {
				applied["routing_force_generation"] = outcome.RoutingForceGeneration
			}
			if outcome.PreviousRoutingForceGeneration > 0 {
				applied["previous_routing_force_generation"] = outcome.PreviousRoutingForceGeneration
			}
			publishAutoRepair(b, res.Correlation, applied)
		}
	}
}

func autoRepairWakeAgent(ctx context.Context, k *kernelruntime.Kernel, cand autoRepairCandidate, msg *board.Message) (autoRepairWakeResult, error) {
	if k == nil {
		return autoRepairWakeResult{}, fmt.Errorf("auto-repair wake requires kernel")
	}
	target := strings.TrimSpace(cand.EscalateTo)
	if target == "" {
		return autoRepairWakeResult{}, nil
	}
	p, ok := k.Roster().Get(target)
	if !ok {
		return autoRepairWakeResult{Target: target, Skipped: "unknown target agent " + target}, nil
	}
	if reason := autoRepairWakeSkipReason(p); reason != "" {
		return autoRepairWakeResult{Target: p.Slug, Skipped: reason}, nil
	}
	corr := k.NewCorrelation()
	rctx := kernelruntime.WithAgentProfile(ctx, p)
	if p.MaxCostMc > 0 {
		rctx = kernelruntime.WithMaxCost(rctx, p.MaxCostMc)
	}
	intent := autoRepairWakeIntent(cand, msg)
	var (
		err    error
		answer string
	)
	if p.RetryPolicy != nil && p.RetryPolicy.MaxAttempts > 1 {
		answer, err = k.RunWithRetry(rctx, corr, intent, *p.RetryPolicy)
	} else {
		answer, err = k.RunWith(rctx, corr, intent)
	}
	return autoRepairWakeResult{
		Target:      p.Slug,
		Correlation: corr,
		Answer:      answer,
		Resolution:  parseAutoRepairResolution(answer),
		Runbook:     roster.AutonomyRunbook(p),
	}, err
}

func autoRepairWakeSkipReason(p roster.Profile) string {
	if !p.Enabled {
		return "target agent " + p.Slug + " is paused"
	}
	if p.Retired {
		return "target agent " + p.Slug + " is retired"
	}
	if !p.AllowsDirectCall() {
		return "target agent " + p.Slug + " is a managed sub-agent"
	}
	return ""
}

func autoRepairWakeIntent(cand autoRepairCandidate, msg *board.Message) string {
	var b strings.Builder
	b.WriteString("Escalation wake-up.\n")
	b.WriteString("A doctor/self-repair attempt failed for agent ")
	b.WriteString(cand.Slug)
	b.WriteString(".\n")
	if mode := strings.TrimSpace(cand.Mode); mode != "" {
		b.WriteString("Failure mode: ")
		b.WriteString(mode)
		b.WriteString(".\n")
	}
	if reason := strings.TrimSpace(cand.Reason); reason != "" {
		b.WriteString("Reason: ")
		b.WriteString(reason)
		b.WriteString("\n")
	}
	if fp := strings.TrimSpace(cand.Fingerprint); fp != "" {
		b.WriteString("Fingerprint: ")
		b.WriteString(autoRepairClip(fp, 320))
		b.WriteString("\n")
	}
	if msg != nil {
		b.WriteString("Mailbox help request")
		if msg.ID != "" {
			b.WriteString(" id=")
			b.WriteString(msg.ID)
		}
		b.WriteString(" was posted")
		if to := strings.TrimSpace(msg.To); to != "" {
			b.WriteString(" to ")
			b.WriteString(to)
		}
		b.WriteString(".\n")
	}
	if root := autoRepairRootAgent(cand); root != "" {
		b.WriteString("Escalation root agent: ")
		b.WriteString(root)
		b.WriteString(".\n")
	}
	if incidentID := autoRepairIncidentIDValue(cand); incidentID != "" {
		b.WriteString("Incident id: ")
		b.WriteString(incidentID)
		b.WriteString(".\n")
	}
	if cand.ChainDepth > 0 {
		b.WriteString("Escalation chain depth: ")
		b.WriteString(fmt.Sprintf("%d", cand.ChainDepth))
		b.WriteString(".\n")
	}
	switch strings.TrimSpace(cand.Mode) {
	case "routing_forced_exhausted":
		b.WriteString("Routing state: an owner-forced chain has already been retried across multiple forced generations and still remains under fallback pressure.\n")
		if taskType := strings.TrimSpace(cand.RoutingRollbackTaskType); taskType != "" {
			b.WriteString("Forced task type: ")
			b.WriteString(taskType)
			b.WriteString(".\n")
		}
		if len(cand.RoutingRollbackToChain) > 0 {
			b.WriteString("Forced chain: ")
			b.WriteString(strings.Join(cand.RoutingRollbackToChain, " → "))
			b.WriteString(".\n")
		}
		b.WriteString("Treat this as an exhausted routing policy. Prefer retire, pause, or delegate unless you have a concrete new force_chain backed by stronger evidence.\n")
	case "routing_forced_failed":
		b.WriteString("Routing state: an owner-forced chain already served its probation window and the same task routing is still under fallback pressure.\n")
		if taskType := strings.TrimSpace(cand.RoutingRollbackTaskType); taskType != "" {
			b.WriteString("Forced task type: ")
			b.WriteString(taskType)
			b.WriteString(".\n")
		}
		if len(cand.RoutingRollbackToChain) > 0 {
			b.WriteString("Forced chain: ")
			b.WriteString(strings.Join(cand.RoutingRollbackToChain, " → "))
			b.WriteString(".\n")
		}
		b.WriteString("Choose deliberately between pause, retire, delegate, or a new force_chain decision. Do not answer with handled unless you actually stabilized ownership and routing policy.\n")
	case "routing_unstable":
		b.WriteString("Routing state: a previous routing rewrite rolled back and the task chain still destabilized again.\n")
		b.WriteString("Treat this as an ownership decision, not a routine retry. Prefer pause, retire, delegate, or force_chain when you have a concrete stable route.\n")
	}
	b.WriteString("Take ownership. Inspect the agent's health, mailbox, logs, and repair state; then recover it, pause/retire it, delegate follow-up, or force a stable routing chain when the task routing is clearly wrong.\n")
	b.WriteString("End your final answer with EXACTLY ONE fenced json block so the escalation can be classified deterministically:\n")
	b.WriteString("```json\n")
	b.WriteString("{\"resolution\":\"handled|paused|retired|delegated|blocked|force_chain\",\"summary\":\"short operator-facing closure note\",\"delegate_to\":\"optional target agent slug when delegated\",\"task_type\":\"required when resolution=force_chain\",\"task_model_chain\":[\"required\",\"when\",\"resolution=force_chain\"]}\n")
	b.WriteString("```\n")
	b.WriteString("Use only one resolution value. Omit delegate_to unless resolution is delegated. Use task_type and task_model_chain only when resolution is force_chain.")
	return b.String()
}

func autoRepairMessageID(msg *board.Message) string {
	if msg == nil {
		return ""
	}
	return strings.TrimSpace(msg.ID)
}

func autoRepairErrString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func plStringMap(pl map[string]any, key string) string {
	if pl == nil {
		return ""
	}
	s, _ := pl[key].(string)
	return strings.TrimSpace(s)
}

func plStringsMap(pl map[string]any, key string) []string {
	raw, ok := pl[key].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func plIntAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func (c *autoRepairCoordinator) autoReplyEscalation(mailbox autoRepairMailbox, postNotify func(board.Message, string), wake autoRepairWakeResult, msg *board.Message) (*board.Message, error) {
	if mailbox == nil || msg == nil || strings.TrimSpace(msg.ID) == "" || strings.TrimSpace(wake.Target) == "" {
		return nil, nil
	}
	orig, ok := mailbox.Get(msg.ID)
	if !ok {
		return nil, nil
	}
	reply, err := mailbox.Send(board.Message{
		Topic:   orig.Topic,
		From:    wake.Target,
		To:      orig.From,
		ReplyTo: orig.ID,
		Text:    autoRepairWakeReplyText(wake),
	}, c.now().UnixMilli())
	if err != nil {
		return nil, err
	}
	if postNotify != nil {
		postNotify(reply, wake.Correlation)
	}
	return &reply, nil
}

func autoRepairWakeReplyText(wake autoRepairWakeResult) string {
	if wake.Resolution != nil {
		var parts []string
		parts = append(parts, "Resolution: "+wake.Resolution.Resolution+".")
		if wake.Resolution.Summary != "" {
			parts = append(parts, wake.Resolution.Summary)
		}
		if wake.Resolution.DelegateTo != "" {
			parts = append(parts, "Delegated to "+wake.Resolution.DelegateTo+".")
		}
		return autoRepairClip(strings.Join(parts, " "), 800)
	}
	answer := strings.TrimSpace(wake.Answer)
	if answer == "" {
		return "Escalation received and handled."
	}
	return autoRepairClip(answer, 800)
}

func parseAutoRepairResolution(finalText string) *autoRepairResolution {
	if strings.TrimSpace(finalText) == "" {
		return nil
	}
	var candidates []string
	for _, block := range strings.Split(finalText, "```") {
		b := strings.TrimSpace(block)
		if b == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(b), "json") {
			b = strings.TrimSpace(b[4:])
		}
		if strings.HasPrefix(b, "{") && strings.Contains(b, "}") {
			candidates = append(candidates, b)
		}
	}
	if len(candidates) == 0 {
		last := strings.LastIndex(finalText, "{")
		end := strings.LastIndex(finalText, "}")
		if last >= 0 && end > last {
			candidates = append(candidates, finalText[last:end+1])
		}
	}
	for i := len(candidates) - 1; i >= 0; i-- {
		var out autoRepairResolution
		if err := json.Unmarshal([]byte(strings.TrimSpace(candidates[i])), &out); err != nil {
			continue
		}
		cleanAutoRepairResolution(&out)
		if out.Resolution != "" {
			return &out
		}
	}
	return nil
}

func sanitizeTaskModelChain(models []string) []string {
	out := make([]string, 0, len(models))
	for _, model := range models {
		if model = strings.TrimSpace(model); model != "" {
			out = append(out, model)
		}
	}
	return out
}

func cleanAutoRepairResolution(out *autoRepairResolution) {
	if out == nil {
		return
	}
	out.Resolution = strings.TrimSpace(strings.ToLower(out.Resolution))
	switch out.Resolution {
	case "handled", "paused", "retired", "delegated", "blocked", "force_chain":
	default:
		out.Resolution = ""
	}
	out.Summary = strings.TrimSpace(strings.Join(strings.Fields(out.Summary), " "))
	out.DelegateTo = strings.TrimSpace(out.DelegateTo)
	out.TaskType = strings.TrimSpace(out.TaskType)
	out.TaskModelChain = sanitizeTaskModelChain(out.TaskModelChain)
	if out.Resolution != "delegated" {
		out.DelegateTo = ""
	}
	if out.Resolution != "force_chain" {
		out.TaskType = ""
		out.TaskModelChain = nil
	}
	if out.Resolution == "force_chain" && (out.TaskType == "" || len(out.TaskModelChain) == 0) {
		out.Resolution = ""
		out.TaskType = ""
		out.TaskModelChain = nil
	}
}

func (c *autoRepairCoordinator) applyAutoRepairResolution(ctx context.Context, k *kernelruntime.Kernel, src autoRepairSource, mailbox autoRepairMailbox, postNotify func(board.Message, string), cand autoRepairCandidate, wake autoRepairWakeResult) (*autoRepairResolutionOutcome, error) {
	if wake.Resolution == nil || wake.Resolution.Resolution == "" {
		return nil, nil
	}
	if err := validateAutoRepairResolution(cand, wake); err != nil {
		return nil, err
	}
	switch wake.Resolution.Resolution {
	case "handled", "blocked":
		return nil, nil
	case "paused":
		if k == nil {
			return nil, fmt.Errorf("paused resolution requires kernel access")
		}
		_, err := k.SetProfileEnabled(cand.Slug, false)
		if err != nil {
			return nil, err
		}
		return &autoRepairResolutionOutcome{Phase: "resolution_applied"}, nil
	case "retired":
		if k == nil {
			return nil, fmt.Errorf("retired resolution requires kernel access")
		}
		reason := strings.TrimSpace(wake.Resolution.Summary)
		if reason == "" {
			reason = "retired by escalation resolution"
		}
		_, err := k.SetProfileRetired(cand.Slug, true, reason)
		if err != nil {
			return nil, err
		}
		return &autoRepairResolutionOutcome{Phase: "resolution_applied"}, nil
	case "force_chain":
		return c.applyForcedRoutingResolution(k, src, cand, wake)
	case "delegated":
		if k == nil {
			return nil, fmt.Errorf("delegated resolution requires kernel access")
		}
		return nil, c.applyDelegatedResolution(ctx, k, k.Bus(), mailbox, postNotify, cand, wake)
	default:
		return nil, nil
	}
}

func (c *autoRepairCoordinator) applyForcedRoutingResolution(k *kernelruntime.Kernel, src autoRepairSource, cand autoRepairCandidate, wake autoRepairWakeResult) (*autoRepairResolutionOutcome, error) {
	if wake.Resolution == nil {
		return nil, nil
	}
	if src == nil {
		return nil, fmt.Errorf("force_chain resolution requires an active repair source")
	}
	applier, ok := src.(autoRepairRoutingChainApplier)
	if !ok {
		return nil, fmt.Errorf("force_chain resolution is not supported by the active repair source")
	}
	taskType := strings.TrimSpace(wake.Resolution.TaskType)
	if taskType == "" || len(wake.Resolution.TaskModelChain) == 0 {
		return nil, fmt.Errorf("force_chain resolution requires task_type and task_model_chain")
	}
	if cand.Mode == "routing_forced_exhausted" &&
		taskType == strings.TrimSpace(cand.RoutingRollbackTaskType) &&
		equalStringSlices(wake.Resolution.TaskModelChain, cand.RoutingRollbackToChain) {
		return nil, fmt.Errorf("force_chain resolution must choose a new chain for exhausted routing policy")
	}
	prevGeneration := autoRepairLatestForceGeneration(k, cand.Slug, taskType)
	nextGeneration := prevGeneration + 1
	if nextGeneration <= 0 {
		nextGeneration = 1
	}
	reason := strings.TrimSpace(wake.Resolution.Summary)
	if reason == "" {
		reason = "forced by escalation resolution"
	}
	res, err := applier.ApplyRoutingChain(cand.Slug, taskType, wake.Resolution.TaskModelChain, reason)
	if err != nil {
		return nil, err
	}
	return &autoRepairResolutionOutcome{
		Phase:                          "resolution_applied",
		RoutingTaskType:                firstNonEmpty(res.RoutingTaskType, taskType),
		RoutingTaskModelChain:          firstNonEmptyStrings(res.RoutingTaskModelChain, wake.Resolution.TaskModelChain),
		PreviousRoutingTaskModelChain:  res.PreviousRoutingTaskModelChain,
		RoutingForceGeneration:         nextGeneration,
		PreviousRoutingForceGeneration: prevGeneration,
	}, nil
}

func validateAutoRepairResolution(cand autoRepairCandidate, wake autoRepairWakeResult) error {
	if wake.Resolution == nil {
		return nil
	}
	if strings.TrimSpace(cand.Mode) != "routing_forced_exhausted" {
		return nil
	}
	switch strings.TrimSpace(wake.Resolution.Resolution) {
	case "paused", "retired", "delegated", "force_chain":
		return nil
	case "handled", "blocked":
		return fmt.Errorf("%s resolution is not allowed for exhausted routing policy", strings.TrimSpace(wake.Resolution.Resolution))
	default:
		return nil
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}

func (c *autoRepairCoordinator) applyDelegatedResolution(ctx context.Context, k *kernelruntime.Kernel, b *bus.Bus, mailbox autoRepairMailbox, postNotify func(board.Message, string), cand autoRepairCandidate, wake autoRepairWakeResult) error {
	target := strings.TrimSpace(wake.Resolution.DelegateTo)
	if target == "" {
		return fmt.Errorf("delegated resolution is missing delegate_to")
	}
	if target == cand.Slug {
		return fmt.Errorf("delegated resolution points back to the broken agent %s", cand.Slug)
	}
	if strings.TrimSpace(wake.Target) != "" && target == strings.TrimSpace(wake.Target) {
		return fmt.Errorf("delegated resolution points back to the current owner %s", wake.Target)
	}
	if mailbox == nil {
		return fmt.Errorf("delegated resolution requires mailbox access")
	}
	msg, err := mailbox.HelpRequest(strings.TrimSpace(wake.Target), target, autoRepairDelegationText(cand, wake), c.now().UnixMilli())
	if err != nil {
		return err
	}
	if postNotify != nil {
		postNotify(msg, wake.Correlation)
	}
	childIncidentID := autoRepairChildIncidentID(cand, target, c.now())
	publishAutoRepair(b, wake.Correlation, map[string]any{
		"phase":              "delegation_queued",
		"agent":              cand.Slug,
		"mode":               cand.Mode,
		"root_agent":         autoRepairRootAgent(cand),
		"chain_depth":        cand.ChainDepth + 1,
		"incident_id":        childIncidentID,
		"root_incident_id":   autoRepairRootChainID(cand),
		"parent_incident_id": autoRepairIncidentIDValue(cand),
		"target_agent":       target,
		"delegated_by":       strings.TrimSpace(wake.Target),
		"target_correlation": wake.Correlation,
		"fingerprint":        cand.Fingerprint,
		"mailbox_message_id": strings.TrimSpace(msg.ID),
		"resolution":         wake.Resolution.Resolution,
		"resolution_summary": wake.Resolution.Summary,
		"delegate_to":        target,
	})
	res, wakeErr := autoRepairWakeAgent(ctx, k, autoRepairCandidate{
		Slug:        cand.Slug,
		Mode:        cand.Mode,
		Reason:      autoRepairDelegationReason(cand, wake),
		Fingerprint: cand.Fingerprint,
		EscalateTo:  target,
		RootAgent:   autoRepairRootAgent(cand),
		ChainDepth:  cand.ChainDepth + 1,
		IncidentID:  childIncidentID,
		RootChainID: autoRepairRootChainID(cand),
		ParentHopID: autoRepairIncidentIDValue(cand),
	}, &msg)
	if wakeErr != nil {
		publishAutoRepair(b, res.Correlation, map[string]any{
			"phase":              "delegation_failed",
			"agent":              cand.Slug,
			"mode":               cand.Mode,
			"root_agent":         autoRepairRootAgent(cand),
			"chain_depth":        cand.ChainDepth + 1,
			"incident_id":        childIncidentID,
			"root_incident_id":   autoRepairRootChainID(cand),
			"parent_incident_id": autoRepairIncidentIDValue(cand),
			"target_agent":       target,
			"delegated_by":       strings.TrimSpace(wake.Target),
			"target_correlation": res.Correlation,
			"fingerprint":        cand.Fingerprint,
			"reason":             wakeErr.Error(),
			"mailbox_message_id": strings.TrimSpace(msg.ID),
			"resolution":         wake.Resolution.Resolution,
			"resolution_summary": wake.Resolution.Summary,
			"delegate_to":        target,
		})
		return wakeErr
	}
	if res.Skipped != "" {
		publishAutoRepair(b, "", map[string]any{
			"phase":              "delegation_failed",
			"agent":              cand.Slug,
			"mode":               cand.Mode,
			"root_agent":         autoRepairRootAgent(cand),
			"chain_depth":        cand.ChainDepth + 1,
			"incident_id":        childIncidentID,
			"root_incident_id":   autoRepairRootChainID(cand),
			"parent_incident_id": autoRepairIncidentIDValue(cand),
			"target_agent":       target,
			"delegated_by":       strings.TrimSpace(wake.Target),
			"target_correlation": res.Correlation,
			"fingerprint":        cand.Fingerprint,
			"reason":             res.Skipped,
			"mailbox_message_id": strings.TrimSpace(msg.ID),
			"resolution":         wake.Resolution.Resolution,
			"resolution_summary": wake.Resolution.Summary,
			"delegate_to":        target,
		})
		return nil
	}
	publishAutoRepair(b, res.Correlation, map[string]any{
		"phase":              "delegation_woke",
		"agent":              cand.Slug,
		"mode":               cand.Mode,
		"root_agent":         autoRepairRootAgent(cand),
		"chain_depth":        cand.ChainDepth + 1,
		"incident_id":        childIncidentID,
		"root_incident_id":   autoRepairRootChainID(cand),
		"parent_incident_id": autoRepairIncidentIDValue(cand),
		"target_agent":       target,
		"delegated_by":       strings.TrimSpace(wake.Target),
		"target_correlation": res.Correlation,
		"fingerprint":        cand.Fingerprint,
		"mailbox_message_id": strings.TrimSpace(msg.ID),
		"resolution":         wake.Resolution.Resolution,
		"resolution_summary": wake.Resolution.Summary,
		"delegate_to":        target,
		"wake_source":        "doctor",
		"autonomy_runbook":   res.Runbook,
	})
	return nil
}

func autoRepairDelegationText(cand autoRepairCandidate, wake autoRepairWakeResult) string {
	var parts []string
	parts = append(parts, "Escalated responsibility for agent "+cand.Slug+".")
	switch cand.Mode {
	case "degraded":
		parts = append(parts, "This is a degraded-doctor recovery follow-up.")
	case "routing":
		parts = append(parts, "This is a routing-repair follow-up.")
	case "routing_forced_exhausted":
		parts = append(parts, "This is a forced-chain-exhausted follow-up after multiple owner-forced generations still failed.")
		if taskType := strings.TrimSpace(cand.RoutingRollbackTaskType); taskType != "" {
			parts = append(parts, "Forced task type: "+taskType+".")
		}
		if len(cand.RoutingRollbackToChain) > 0 {
			parts = append(parts, "Forced chain: "+strings.Join(cand.RoutingRollbackToChain, " → ")+".")
		}
	case "routing_unstable":
		parts = append(parts, "This is an unstable-routing follow-up after rollback pressure returned.")
	case "routing_forced_failed":
		parts = append(parts, "This is a forced-chain-failed follow-up after owner probation expired.")
		if taskType := strings.TrimSpace(cand.RoutingRollbackTaskType); taskType != "" {
			parts = append(parts, "Forced task type: "+taskType+".")
		}
		if len(cand.RoutingRollbackToChain) > 0 {
			parts = append(parts, "Forced chain: "+strings.Join(cand.RoutingRollbackToChain, " → ")+".")
		}
	default:
		parts = append(parts, "This is a config-repair follow-up.")
	}
	if reason := autoRepairClip(cand.Reason, 220); reason != "" {
		parts = append(parts, "Original reason: "+reason)
	}
	if wake.Resolution != nil && wake.Resolution.Summary != "" {
		parts = append(parts, "Owner note: "+autoRepairClip(wake.Resolution.Summary, 220))
	}
	return strings.Join(parts, " ")
}

func autoRepairDelegationReason(cand autoRepairCandidate, wake autoRepairWakeResult) string {
	base := "delegated escalation follow-up for agent " + cand.Slug
	if wake.Resolution != nil && wake.Resolution.Summary != "" {
		return base + ": " + autoRepairClip(wake.Resolution.Summary, 220)
	}
	if cand.Reason != "" {
		return base + ": " + autoRepairClip(cand.Reason, 220)
	}
	return base
}

func autoRepairRootAgent(cand autoRepairCandidate) string {
	root := strings.TrimSpace(cand.RootAgent)
	if root != "" {
		return root
	}
	return strings.TrimSpace(cand.Slug)
}

func autoRepairIncidentID(root, fingerprint string, now time.Time) string {
	root = autoRepairIDPart(root)
	if root == "" {
		root = "agent"
	}
	fp := autoRepairIDPart(fingerprint)
	if fp == "" {
		fp = "repair"
	}
	return fmt.Sprintf("%s-%d-%s", root, now.UnixMilli(), fp)
}

func autoRepairChildIncidentID(cand autoRepairCandidate, target string, now time.Time) string {
	fp := strings.TrimSpace(cand.Fingerprint)
	if target = strings.TrimSpace(target); target != "" {
		fp += " " + target
	}
	return autoRepairIncidentID(autoRepairRootAgent(cand), fp, now)
}

func autoRepairIDPart(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-', r == '_':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= 32 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

func autoRepairIncidentIDValue(cand autoRepairCandidate) string {
	return strings.TrimSpace(cand.IncidentID)
}

func autoRepairRootChainID(cand autoRepairCandidate) string {
	if root := strings.TrimSpace(cand.RootChainID); root != "" {
		return root
	}
	return autoRepairIncidentIDValue(cand)
}

func autoRepairEscalationText(cand autoRepairCandidate, repairErr error) string {
	target := strings.TrimSpace(cand.Slug)
	mode := strings.TrimSpace(cand.Mode)
	text := "Doctor self-repair failed"
	if mode == "degraded" {
		text = "Doctor recovery failed"
	}
	if target != "" {
		text += " for agent " + target
	}
	if reason := autoRepairClip(cand.Reason, 220); reason != "" {
		text += ". Reason: " + reason
	}
	if repairErr != nil {
		text += ". Error: " + autoRepairClip(repairErr.Error(), 220)
	}
	if cand.Fingerprint != "" {
		text += ". Fingerprint: " + autoRepairClip(cand.Fingerprint, 160)
	}
	return text
}

func (c *autoRepairCoordinator) release(slug string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.inflight, slug)
}

func publishAutoRepair(b *bus.Bus, corr string, payload map[string]any) {
	if b == nil {
		return
	}
	_, _ = b.Publish(event.Spec{
		Subject:       autoRepairEventSubject,
		Kind:          event.KindInfo,
		Actor:         "kernel",
		CorrelationID: corr,
		Payload:       payload,
	})
}
