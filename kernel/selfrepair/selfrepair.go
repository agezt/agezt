// SPDX-License-Identifier: MIT

// Package selfrepair wires the deterministic doctor/auto-repair coordinator
// (Phase 2.6 extraction from cmd/agezt): it subscribes to the reaper pulse
// observer, claims broken/degraded/routing-unstable agents, drives the
// overseertool repair source, and escalates through the mailbox + wake chain
// when a repair fails. The daemon arms it once at boot via WireAutoRepair.
//
// Import posture: selfrepair imports kernel/runtime and (like
// kernel/controlplane) the overseertool plugin as its repair source; nothing in
// kernel/runtime may ever import selfrepair.
package selfrepair

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
	"github.com/agezt/agezt/internal/strutil"
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

// Mailbox is the message-board surface auto-repair escalations post through.
// *board.Store satisfies it; a nil-tolerant caller may pass nil to disable
// mailbox escalation.
type Mailbox interface {
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

// WireAutoRepair subscribes the auto-repair coordinator to the reaper pulse
// subject and launches it on ctx. It returns the boot-banner status string
// ("armed (…)" or a "disabled (…)" reason) exactly as the daemon prints it.
func WireAutoRepair(ctx context.Context, k *kernelruntime.Kernel, baseDir string, mailbox Mailbox, postNotify func(board.Message, string)) string {
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

func (c *autoRepairCoordinator) run(ctx context.Context, sub *bus.Subscription, k *kernelruntime.Kernel, src autoRepairSource, mailbox Mailbox, postNotify func(board.Message, string)) {
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
			c.handleTick(ctx, k, src, mailbox, postNotify)
		}
	}
}

// handleTick scans the reaper window and dispatches one repair per claimed
// candidate. It is a separate frame purely so the panic firewall (WF-001) is
// scoped to ONE tick: run() is launched with a bare `go` from WireAutoRepair, so
// a panic in ReaperScan or claim used to take the daemon down with it. Recovering
// per tick rather than around the whole loop also keeps auto-repair ARMED after a
// bad event — disarming the fleet's healer on one malformed pulse would be a
// silent, permanent degradation.
func (c *autoRepairCoordinator) handleTick(ctx context.Context, k *kernelruntime.Kernel, src autoRepairSource, mailbox Mailbox, postNotify func(board.Message, string)) {
	// claim() marks candidates in-flight and dispatch() is what releases them, so
	// a panic between the two would leak the claim and wedge every future repair
	// of that agent. Track the hand-off point and release whatever never made it.
	var (
		pending  []autoRepairCandidate
		launched int
	)
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		for _, cand := range pending[launched:] {
			c.release(cand.Slug)
		}
		fmt.Fprintf(os.Stderr, "auto-repair tick panicked: %v\n", r)
		if k == nil || k.Bus() == nil {
			return
		}
		_, _ = k.Bus().Publish(event.Spec{
			Subject: autoRepairPulseSubject,
			Kind:    event.KindSelfRepairPanic,
			Actor:   "selfrepair",
			Payload: map[string]any{
				"phase":     "tick",
				"panic":     fmt.Sprintf("%v", r),
				"abandoned": len(pending) - launched,
			},
		})
	}()

	cut := c.now().Add(-autoRepairReaperWindow).UnixMilli()
	rep := k.ReaperScan(cut, cut)
	pending = c.claim(k, rep, k.Roster().List())
	for _, cand := range pending {
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
		launched++
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

func (c *autoRepairCoordinator) dispatch(ctx context.Context, k *kernelruntime.Kernel, b *bus.Bus, src autoRepairSource, mailbox Mailbox, postNotify func(board.Message, string), cand autoRepairCandidate) {
	// Panic firewall (WF-001). dispatch is launched with a bare `go` from the
	// coordinator loop, so nothing above can recover it — and it drives a full
	// governed run: provider calls, tools, plugin subprocesses, MCP servers, then
	// a mailbox post over the network. Any of those can panic, and a repair
	// attempt taking the daemon down is the worst possible failure mode for the
	// component whose entire job is keeping the fleet healthy.
	//
	// Registered BEFORE the release defer so it runs last, after the slug lock is
	// returned: a contained panic must not also leak the in-flight claim and
	// wedge every future repair of this agent.
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		fmt.Fprintf(os.Stderr, "auto-repair for %q panicked: %v\n", cand.Slug, r)
		if b != nil {
			_, _ = b.Publish(event.Spec{
				Subject: autoRepairPulseSubject,
				Kind:    event.KindSelfRepairPanic,
				Actor:   "selfrepair",
				Payload: map[string]any{
					"agent":       cand.Slug,
					"mode":        cand.Mode,
					"incident_id": autoRepairIncidentIDValue(cand),
					"panic":       fmt.Sprintf("%v", r),
				},
			})
		}
	}()
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
			"routing_task_type":                 strutil.FirstNonEmpty(cand.RoutingRollbackTaskType, res.RoutingTaskType),
			"routing_task_model_chain":          strutil.FirstNonEmptySlice(cand.RoutingRollbackToChain, res.RoutingTaskModelChain),
			"previous_routing_task_model_chain": strutil.FirstNonEmptySlice(cand.RoutingRollbackFromChain, res.PreviousRoutingTaskModelChain),
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

func (c *autoRepairCoordinator) autoEscalate(mailbox Mailbox, postNotify func(board.Message, string), cand autoRepairCandidate, repairErr error) (*board.Message, error) {
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
