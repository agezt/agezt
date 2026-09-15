// SPDX-License-Identifier: MIT
package selfrepair

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

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
