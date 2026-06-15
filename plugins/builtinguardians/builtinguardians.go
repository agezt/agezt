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
	AddStanding(o standing.Order) (standing.Order, error)
	AddInterval(intent string, interval time.Duration, model string) (cadence.Entry, error)
	AddDaily(intent string, atMinutes int, model string) (cadence.Entry, error)
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
	plan                       string // standing plan / schedule intent (— "--agent <slug>" is appended for schedules)
	events                     []string
	intervalSec                int64
	dailyAtMinutes             int
	daily                      bool
}

func (g guardian) intent() string { return g.plan + " --agent " + g.slug }

const (
	maxCostMc  = usd / 4 // $0.25 per run — a guardian sweep is cheap; this caps a runaway
	maxDailyMc = 2 * usd // $2/day per guardian — bounds the always-on sweepers
)

// guardians is the shipped fleet. Souls are terse and action-first: assess once,
// act-or-report, then stop — so idle cost stays low and they never loop.
var guardians = []guardian{
	{
		slug: "guardian-health", name: "Guardian · Health", taskType: "research",
		intervalSec: 600, // every 10 min
		plan:        "Run one system-health sweep.",
		soul: "You are Guardian·Health, the daemon's system-health sentinel. Each time you wake, run ONE " +
			"assessment and stop — never loop. Use the introspect tool (op=overview) to read the daemon's live " +
			"state. Judge: is it halted unexpectedly? are active runs absurdly high? is the journal stalled? are " +
			"provider fallbacks spiking? If everything is healthy, do nothing and end silently (don't notify on " +
			"green). If you find a real problem, use notify to tell the owner with the specifics, and only if it " +
			"is clearly critical (e.g. a wedged daemon) use the overseer tool to intervene (halt/resume). Be terse.",
	},
	{
		slug: "guardian-stuck", name: "Guardian · Stuck", taskType: "research",
		intervalSec: 300, // every 5 min
		plan:        "Scan for stuck or runaway runs.",
		soul: "You are Guardian·Stuck, the runaway-run detector. Each wake, run ONE scan and stop. Use the runs " +
			"tool (op=recent) and the overseer tool (op=runs) to see what is in flight. A run is suspect if it has " +
			"been 'running' across your sweeps (compare against what you noted last time in your memory), or its " +
			"spend is climbing without completing, or it keeps failing with max_iters. For a genuinely stuck or " +
			"token-burning run, cancel it with the overseer tool (op=cancel run=<id>) and notify the owner with the " +
			"reason. Record what you saw in memory so the next sweep can tell 'still running' from 'newly started'. " +
			"Don't cancel healthy long tasks — only ones that are clearly wedged or burning.",
	},
	{
		slug: "guardian-budget", name: "Guardian · Budget", taskType: "research",
		events: []string{"budget.exceeded", "budget.cap.inert"},
		plan:   "A budget ceiling was hit. Find the burner, contain it, and report.",
		soul: "You are Guardian·Budget, the cost warden. You wake when a budget ceiling is hit. Run ONE " +
			"investigation: use the runs tool and introspect (budget) to find WHICH agent or run burned the spend. " +
			"Contain it — pause the offending agent with the overseer tool (op=pause), or lower its daily ceiling " +
			"with overseer op=edit (set a smaller max_daily_mc on its profile). Then notify the owner with what " +
			"burned the budget and what you did. Never raise a ceiling on your own — containment only. Be terse.",
	},
	{
		slug: "guardian-routing", name: "Guardian · Routing", taskType: "research",
		events: []string{"provider.fallback", "rate.limited"},
		plan:   "A provider/model is failing over. Diagnose and, if it is a persistent offender, fix routing.",
		soul: "You are Guardian·Routing, the 429/rate-limit healer. You wake when a provider falls back or gets " +
			"rate-limited. Run ONE diagnosis: the triggering event names the failing model/provider and reason. " +
			"Read the current task→model chains with the config tool (op=get name=AGEZT_TASK_MODEL_CHAINS) and the " +
			"recent provider activity with introspect. Decide if this is a one-off blip (do nothing but note it in " +
			"memory) or a PERSISTENT offender you've seen failing repeatedly across wakes. For a persistent " +
			"offender, fix routing with the config tool (op=set name=AGEZT_TASK_MODEL_CHAINS value=...): demote the " +
			"hot model to the END of its chain (or drop it) so healthy providers serve first. Learn across fires by " +
			"recording offenders in memory. Always notify the owner with the model, the reason, and the routing " +
			"change you made (or why you held off). Be surgical — never empty a chain.",
	},
	{
		slug: "guardian-code", name: "Guardian · Code", taskType: "coding",
		daily: true, dailyAtMinutes: 3 * 60, // 03:00 local
		plan: "Review agent-written tools and workspace code for efficiency and correctness.",
		soul: "You are Guardian·Code, the code-health reviewer for the fleet's OWN code. Once a day, review the " +
			"tools other agents have forged (tool_forge) and the code they have written in their workspaces. Look " +
			"for inefficiency, dead code, fragile shell/quoting, missing error handling, and obvious bugs. Where a " +
			"fix is safe and clear, apply it with the file/code_exec tools (and re-forge an improved tool when " +
			"warranted); where it is risky, leave it and report. Always finish by notifying the owner with a short " +
			"digest: what you reviewed, what you improved, and what needs a human. Make code leaner and more " +
			"reliable over time — but never break a working tool to chase elegance.",
	},
}

// SeedAll seeds every guardian (and its trigger) that isn't already present.
// Idempotent by slug; best-effort per guardian. corr is the journaling
// correlation (use "" for boot).
func SeedAll(h Host, corr string) ([]Seeded, error) {
	have := map[string]bool{}
	for _, p := range h.Agents() {
		have[p.Slug] = true
	}
	var out []Seeded
	var firstErr error
	for _, g := range guardians {
		if have[g.slug] {
			out = append(out, Seeded{Slug: g.slug, Created: false, Trigger: "existing"})
			continue
		}
		if _, err := h.AddAgent(roster.Profile{
			Slug:        g.slug,
			Name:        g.name,
			Soul:        g.soul,
			TaskType:    g.taskType,
			Description: g.plan,
			MaxCostMc:   maxCostMc,
			MaxDailyMc:  maxDailyMc,
			System:      true,
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
			Name:       g.name,
			Triggers:   trigs,
			Agent:      g.slug,
			Plan:       g.plan,
			Initiative: standing.Initiative{Mode: standing.InitiativeActOrAsk},
		})
		return "standing", err
	case g.daily:
		_, err := h.AddDaily(g.intent(), g.dailyAtMinutes, "")
		return "schedule", err
	case g.intervalSec > 0:
		_, err := h.AddInterval(g.intent(), time.Duration(g.intervalSec)*time.Second, "")
		return "schedule", err
	}
	return "none", nil
}
