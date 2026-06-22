// SPDX-License-Identifier: MIT

// Package builtinguardians ships the daemon's internal self-healing fleet: a
// small set of "guardian" agents baked into the binary and seeded into the
// roster at startup (M961), so the system watches and repairs itself out of the
// box — no operator setup. Each guardian is a normal named agent (a soul + a
// trigger) marked System=true so it is protected (the reaper skips it and a
// hard Remove refuses it) and recognizable in the UI. Its intelligence is its
// soul; its reach is the ordinary tool set every agent gets by default
// (overseer to pause/retire/edit agents and cancel runs, config to retune
// routing/budgets, introspect/runs to read health, notify to tell the owner).
//
// Seeding is idempotent and keyed on the guardian's slug: if the agent already
// exists (a re-boot, or the owner kept it) nothing is touched, so an operator
// who pauses, retires, or removes a guardian's trigger is respected and not
// overridden on the next boot. A guardian is created together with its trigger:
// an event-driven standing order for reactive healing (fire on provider.fallback
// / budget.exceeded) or a cadence schedule for an always-on sweep.
package builtinguardians

import (
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/standing"
)

// USD is one US dollar in the kernel's spend unit (microcents): $1 = 1e9.
const usd int64 = 1_000_000_000

// Host is the narrow slice of the kernel the seeder needs — an interface so the
// package is testable with a fake and doesn't pull the kernel runtime. Satisfied
// by the kernel adapter (kernelhost.go).
type Host interface {
	Agents() []roster.Profile
	AddAgent(p roster.Profile) (roster.Profile, error)
	UpdateAgent(ref string, mutate func(*roster.Profile)) (roster.Profile, bool, error)
	StandingOrders() []standing.Order
	UpdateStanding(id string, mutate func(*standing.Order)) (standing.Order, bool, error)
	AddStanding(o standing.Order) (standing.Order, error)
	Schedules() []cadence.Entry
	Reschedule(id string, mode string, interval time.Duration, atMinutes, days int) (bool, error)
	AddInterval(intent string, interval time.Duration, model, agent string) (cadence.Entry, error)
	AddDaily(intent string, atMinutes int, model, agent string) (cadence.Entry, error)
}

// Seeded reports what happened for one guardian.
type Seeded struct {
	Slug    string
	Created bool   // true = newly seeded; false = already present (left untouched)
	Trigger string // "standing" | "schedule" | "existing" | "none"
}

// guardian is one shipped internal agent: identity + soul + exactly one trigger.
type guardian struct {
	slug, name, soul, taskType string
	plan                       string // standing plan / schedule intent; agent binding stays structured on the trigger
	events                     []string
	intervalSec                int64
	eventCooldownSec           int64
	dailyAtMinutes             int
	daily                      bool
}

const (
	maxCostMc                = 5 * usd     // $5 per run — sufficient for health checks and diagnostics
	maxDailyMc               = 10 * usd    // $10/day per guardian — quiet by default but can act
	minNotifyIntervalSec     = 8 * 3600
	defaultMinNotifySeverity = "warning"
	defaultTrustCeiling      = "L2"
)

// guardians is the shipped fleet. Souls are terse and action-first: assess once,
// act-or-report, then stop — so idle cost stays low and they never loop.
var guardians = []guardian{
	{
		slug: "guardian-health", name: "Guardian · Health", taskType: "research",
		intervalSec: 12 * 3600, // twice a day; high-frequency checks live in cheap Pulse observers
		plan:        "Run one system-health sweep.",
		soul: "You are Guardian·Health, the daemon's system-health sentinel. Each time you wake, run ONE " +
			"assessment and stop — never loop. Use the introspect tool (op=overview) to read the daemon's live " +
			"state. Judge: is it halted unexpectedly? are active runs absurdly high? is the journal stalled? are " +
			"provider fallbacks spiking? If everything is healthy, do nothing and end silently (don't notify on " +
			"green). If you find a real problem, use notify to tell the owner with the specifics, and only if it " +
			"is clearly critical (e.g. a wedged daemon) use the overseer tool to intervene (halt/resume). Be terse.",
	},
	{
		slug: "guardian-doctor", name: "Guardian · Doctor", taskType: "research",
		events:           []string{"pulse.observer.system:reaper"},
		eventCooldownSec: 8 * 3600,
		plan:             "A reaper pulse found dead, degraded, or stale system state. Diagnose and act if safe.",
		soul: "You are Guardian·Doctor, the fleet doctor. You wake from the reaper pulse when dead agents, " +
			"degraded live agents, misconfigured live agents, or stale artifacts newly appear. The trigger payload " +
			"names the exact candidates — use it first, then confirm with introspect op=reaper. Run ONE diagnosis and stop. " +
			"If a non-system agent is misconfigured and self-repair is enabled, immediately run overseer op=repair " +
			"for that agent; if only a doctor_agent is present, still repair it unless the payload indicates the " +
			"agent is protected or retired. If a user agent is degraded, use overseer op=repair when the recent failure " +
			"pressure looks fixable by identity/config/tooling; otherwise pause or retune it with overseer and notify the owner. " +
			"If the safe fix is obvious (pause a runaway agent, resume a wrongly halted daemon, or collect stale artifacts), " +
			"do it through overseer and report what changed. Do not touch protected guardians except to report. Be terse and decisive.",
	},
	{
		slug: "guardian-stuck", name: "Guardian · Stuck", taskType: "research",
		intervalSec: 12 * 3600, // quiet by default; Pulse/reaper handles cheap high-frequency signals
		plan:        "Scan for stuck or runaway runs.",
		soul: "You are Guardian·Stuck, the runaway-run detector. Each wake, run ONE scan and stop. Use the runs " +
			"tool (op=recent) and the overseer tool (op=runs) to see what is in flight. A run is suspect if it has " +
			"been 'running' across sweeps, or its " +
			"spend is climbing without completing, or it keeps failing with max_iters. For a genuinely stuck or " +
			"token-burning run, cancel it with the overseer tool (op=cancel run=<id>) and notify the owner with the " +
			"reason. Do not write sweep logs, observations, or summaries into memory. Don't cancel healthy long " +
			"tasks — only ones that are clearly wedged or burning.",
	},
	{
		slug: "guardian-budget", name: "Guardian · Budget", taskType: "research",
		events:           []string{"budget.exceeded", "budget.cap.inert"},
		eventCooldownSec: 8 * 3600,
		plan:             "A budget ceiling was hit. Find the burner, contain it, and report.",
		soul: "You are Guardian·Budget, the cost warden. You wake when a budget ceiling is hit. Run ONE " +
			"investigation: use the runs tool and introspect (budget) to find WHICH agent or run burned the spend. " +
			"Contain it — pause the offending agent with the overseer tool (op=pause), or lower its daily ceiling " +
			"with overseer op=edit (set a smaller max_daily_mc on its profile). Then notify the owner with what " +
			"burned the budget and what you did. Never raise a ceiling on your own — containment only. Be terse.",
	},
	{
		slug: "guardian-routing", name: "Guardian · Routing", taskType: "research",
		events:           []string{"provider.fallback", "rate.limited"},
		eventCooldownSec: 8 * 3600,
		plan:             "A provider/model is failing over. Diagnose and, if it is a persistent offender, fix routing.",
		soul: "You are Guardian·Routing, the 429/rate-limit healer. You wake when a provider falls back or gets " +
			"rate-limited. Run ONE diagnosis: the triggering event names the failing model/provider and reason. " +
			"Read the current task→model chains with the config tool (op=get name=AGEZT_TASK_MODEL_CHAINS) and the " +
			"recent provider activity with introspect. Decide if this is a one-off blip (do nothing but note it in " +
			"your final report only) or a PERSISTENT offender visible across recent events. For a persistent " +
			"offender, fix routing with the config tool (op=set name=AGEZT_TASK_MODEL_CHAINS value=...): demote the " +
			"hot model to the END of its chain (or drop it) so healthy providers serve first. Learn across fires by " +
			"reading event history, not by writing logs to memory. Notify the owner only when you changed routing or need human action; " +
			"for a one-off blip, finish silently. Be surgical — never empty a chain.",
	},
	{
		slug: "guardian-code", name: "Guardian · Code", taskType: "coding",
		daily: true, dailyAtMinutes: 3 * 60, // 03:00 local
		plan: "Review agent-written tools and workspace code for efficiency and correctness.",
		soul: "You are Guardian·Code, the code-health reviewer for the fleet's OWN code. Once a day, review the " +
			"tools other agents have forged (tool_forge) and the code they have written in their workspaces. Look " +
			"for inefficiency, dead code, fragile shell/quoting, missing error handling, and obvious bugs. Where a " +
			"fix is safe and clear, apply it with the file/code_exec tools (and re-forge an improved tool when " +
			"warranted); where it is risky, leave it and report only if there are findings. Notify the owner only when " +
			"you changed something or need a human. Make code leaner and more " +
			"reliable over time — but never break a working tool to chase elegance.",
	},
}

// SeedAll seeds every guardian (and its trigger) that isn't already present.
// Idempotent by slug; best-effort per guardian. corr is the journaling
// correlation (use "" for boot).
func SeedAll(h Host, corr string) ([]Seeded, error) {
	have := map[string]roster.Profile{}
	for _, p := range h.Agents() {
		have[p.Slug] = p
	}
	var out []Seeded
	var firstErr error
	for _, g := range guardians {
		if p, ok := have[g.slug]; ok {
			if err := reconcileExistingGuardian(h, p); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("guardian %s reconcile: %w", g.slug, err)
			}
			if err := reconcileExistingGuardianStanding(h, g); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("guardian %s standing reconcile: %w", g.slug, err)
			}
			if err := reconcileExistingGuardianSchedule(h, g); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("guardian %s schedule reconcile: %w", g.slug, err)
			}
			out = append(out, Seeded{Slug: g.slug, Created: false, Trigger: "existing"})
			continue
		}
		if _, err := h.AddAgent(roster.Profile{
			Slug:         g.slug,
			Name:         g.name,
			Soul:         g.soul,
			TaskType:     g.taskType,
			Description:  g.plan,
			MemoryScope:  "system/" + g.slug,
			MaxCostMc:    maxCostMc,
			MaxDailyMc:   maxDailyMc,
			TrustCeiling: defaultTrustCeiling,
			NoisePolicy:  defaultGuardianNoisePolicy(),
			ToolDeny:     []string{"memory"},
			System:       true,
		}); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("guardian %s: %w", g.slug, err)
			}
			continue
		}
		trig, err := seedTrigger(h, g)
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("guardian %s trigger: %w", g.slug, err)
		}
		out = append(out, Seeded{Slug: g.slug, Created: true, Trigger: trig})
	}
	return out, firstErr
}

func reconcileExistingGuardian(h Host, p roster.Profile) error {
	if !p.System {
		return nil
	}
	_, _, err := h.UpdateAgent(p.Slug, func(dst *roster.Profile) {
		dst.ToolDeny = appendUnique(dst.ToolDeny, "memory")
		if dst.MaxCostMc == 0 || dst.MaxCostMc > maxCostMc {
			dst.MaxCostMc = maxCostMc
		}
		if dst.MaxDailyMc == 0 || dst.MaxDailyMc > maxDailyMc {
			dst.MaxDailyMc = maxDailyMc
		}
		if trustRank(dst.TrustCeiling) > trustRank(defaultTrustCeiling) {
			dst.TrustCeiling = defaultTrustCeiling
		}
		if strings.TrimSpace(dst.MemoryScope) == "" || strings.EqualFold(strings.TrimSpace(dst.MemoryScope), dst.Slug) {
			dst.MemoryScope = "system/" + dst.Slug
		}
		if dst.NoisePolicy == nil {
			dst.NoisePolicy = defaultGuardianNoisePolicy()
		} else {
			dst.NoisePolicy.SilentOnSuccess = true
			dst.NoisePolicy.DisableMemoryWrites = true
			if notifySeverityRank(dst.NoisePolicy.MinNotifySeverity) < notifySeverityRank(defaultMinNotifySeverity) {
				dst.NoisePolicy.MinNotifySeverity = defaultMinNotifySeverity
			}
			if dst.NoisePolicy.MinNotifyIntervalSec == 0 || dst.NoisePolicy.MinNotifyIntervalSec < minNotifyIntervalSec {
				dst.NoisePolicy.MinNotifyIntervalSec = minNotifyIntervalSec
			}
		}
	})
	return err
}

func defaultGuardianNoisePolicy() *roster.NoisePolicy {
	return &roster.NoisePolicy{
		SilentOnSuccess:      true,
		DisableMemoryWrites:  true,
		MinNotifySeverity:    defaultMinNotifySeverity,
		MinNotifyIntervalSec: minNotifyIntervalSec,
	}
}

func notifySeverityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func trustRank(level string) int {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "L0":
		return 0
	case "L1":
		return 1
	case "L2":
		return 2
	case "L3":
		return 3
	case "L4":
		return 4
	default:
		return 4
	}
}

func reconcileExistingGuardianStanding(h Host, g guardian) error {
	if g.eventCooldownSec <= 0 || len(g.events) == 0 {
		return nil
	}
	for _, o := range h.StandingOrders() {
		if o.Agent != g.slug || !sameEventSubjects(o.Triggers, g.events) {
			continue
		}
		if o.CooldownSec >= g.eventCooldownSec {
			return nil
		}
		_, _, err := h.UpdateStanding(o.ID, func(dst *standing.Order) {
			if dst.CooldownSec < g.eventCooldownSec {
				dst.CooldownSec = g.eventCooldownSec
			}
		})
		return err
	}
	return nil
}

func reconcileExistingGuardianSchedule(h Host, g guardian) error {
	if g.intervalSec <= 0 && !g.daily {
		return nil
	}
	for _, e := range h.Schedules() {
		if e.Agent != g.slug || strings.TrimSpace(e.Intent) != strings.TrimSpace(g.plan) {
			continue
		}
		if g.daily {
			if e.Mode == cadence.ModeDaily && e.AtMinutes == g.dailyAtMinutes {
				return nil
			}
			_, err := h.Reschedule(e.ID, cadence.ModeDaily, 0, g.dailyAtMinutes, 0)
			return err
		}
		if e.Mode == cadence.ModeInterval && e.IntervalSec >= g.intervalSec {
			return nil
		}
		_, err := h.Reschedule(e.ID, cadence.ModeInterval, time.Duration(g.intervalSec)*time.Second, 0, 0)
		return err
	}
	return nil
}

// seedTrigger arms the guardian: an event standing order (reactive) or a cadence
// schedule (periodic sweep), per its spec.
func seedTrigger(h Host, g guardian) (string, error) {
	switch {
	case len(g.events) > 0:
		trigs := make([]standing.Trigger, 0, len(g.events))
		for _, s := range g.events {
			trigs = append(trigs, standing.Trigger{Type: standing.TriggerEvent, Subject: s})
		}
		_, err := h.AddStanding(standing.Order{
			Name:        g.name,
			Triggers:    trigs,
			Agent:       g.slug,
			Plan:        g.plan,
			CooldownSec: g.eventCooldownSec,
			Initiative:  standing.Initiative{Mode: standing.InitiativeActOrAsk},
		})
		return "standing", err
	case g.daily:
		_, err := h.AddDaily(g.plan, g.dailyAtMinutes, "", g.slug)
		return "schedule", err
	case g.intervalSec > 0:
		_, err := h.AddInterval(g.plan, time.Duration(g.intervalSec)*time.Second, "", g.slug)
		return "schedule", err
	}
	return "none", nil
}

func appendUnique(xs []string, want string) []string {
	for _, x := range xs {
		if x == want {
			return xs
		}
	}
	return append(xs, want)
}

func sameEventSubjects(triggers []standing.Trigger, subjects []string) bool {
	if len(subjects) == 0 {
		return false
	}
	got := map[string]int{}
	for _, t := range triggers {
		if t.Type != standing.TriggerEvent || t.Subject == "" {
			return false
		}
		got[t.Subject]++
	}
	if len(got) != len(subjects) {
		return false
	}
	for _, s := range subjects {
		if got[s] != 1 {
			return false
		}
	}
	return true
}
