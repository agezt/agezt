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
