// SPDX-License-Identifier: MIT

// Package reflect implements "Reflection v1" (SPEC-05 §6): the meta-cognition
// loop. Periodically (or on demand via `agt reflect run`) the system reviews
// its OWN behaviour from the journal and recalibrates. It sits a level above
// Forge — Forge improves *skills*, Reflection improves *judgment* (salience,
// initiative, world-model weights).
//
// Reflection holds no durable state of its own: a Report is derived from the
// journal (the same read-only fold `agt runs list`/`skill history` use) and
// then journaled as a reflection.completed event, so `agt reflect show` just
// reads the newest one back. The single auto-applied adjustment is world-model
// decay (SPEC-05 §6.3) — within safe bounds, owned data, never touches
// autonomy. Everything that WOULD touch judgment/autonomy is surfaced as an
// advisory Proposal only (§6.4): the system may propose lowering its own
// autonomy but never raises it, and never silently changes it.
package reflect

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/worldmodel"
)

// Config tunes a reflection pass.
type Config struct {
	// Decay parameters for the world-model weight adjustment.
	Decay worldmodel.DecayOptions
	// BriefVolume: at or above this many briefs in the window, propose
	// reviewing the Pulse dial. Default 8.
	BriefVolume int
	// DenyExcess: when approvals denied exceed granted by at least this many,
	// propose lowering autonomy. Default 3.
	DenyExcess int
}

const (
	defaultBriefVolume = 8
	defaultDenyExcess  = 3
)

// Observations are the counts folded from the journal for one pass.
type Observations struct {
	WindowEvents     int `json:"window_events"`
	TasksStarted     int `json:"tasks_started"`
	TasksCompleted   int `json:"tasks_completed"`
	TasksFailed      int `json:"tasks_failed"`
	BriefsSent       int `json:"briefs_sent"`
	SkillsActivated  int `json:"skills_activated"`
	ApprovalsGranted int `json:"approvals_granted"`
	ApprovalsDenied  int `json:"approvals_denied"`
	EntitiesTotal    int `json:"entities_total"`
}

// Proposal is an advisory recalibration the system suggests but does NOT apply
// (anything affecting judgment/autonomy, §6.4).
type Proposal struct {
	Area        string `json:"area"`        // "pulse", "autonomy", "tasks", ...
	Observation string `json:"observation"` // what was seen
	Suggestion  string `json:"suggestion"`  // what to consider (never auto-done)
}

// Report is the output of one reflection pass — journaled, then read back by
// `agt reflect show`.
type Report struct {
	GeneratedMS     int64        `json:"generated_ms"`
	Observations    Observations `json:"observations"`
	EntitiesDecayed int          `json:"entities_decayed"`
	Proposals       []Proposal   `json:"proposals"`
}

// Engine runs reflection passes. It reads the journal, mutates the world graph
// for decay, and publishes the report. Mirrors how Pulse is a thin resident
// over the bus + state — here it's the journal + world.
type Engine struct {
	journal *journal.Journal
	world   *worldmodel.Graph
	bus     *bus.Bus
	cfg     Config
	now     func() time.Time
}

// New builds an Engine. journal and world are required; bus may be nil in
// tests that don't assert on the published report.
func New(j *journal.Journal, w *worldmodel.Graph, b *bus.Bus, cfg Config) *Engine {
	return &Engine{journal: j, world: w, bus: b, cfg: cfg, now: time.Now}
}

// Reflect runs one pass: fold the journal into observations, apply world-model
// decay (the one safe auto-adjustment), derive advisory proposals, and journal
// the report under corr. ctx is accepted for forward-compatibility (an
// LLM-assisted narrative is deferred); v1 is deterministic and offline.
func (e *Engine) Reflect(ctx context.Context, corr string) (Report, error) {
	_ = ctx
	obs := e.observe()

	decayed, err := e.world.Decay(corr, e.cfg.Decay)
	if err != nil {
		return Report{}, err
	}

	rep := Report{
		GeneratedMS:     e.now().UnixMilli(),
		Observations:    obs,
		EntitiesDecayed: decayed,
		Proposals:       e.proposals(obs),
	}
	e.publish(corr, rep)
	return rep, nil
}

// observe folds the whole journal into the per-pass counts. It is a pure read
// (the same range-and-count the control-plane folds use). EntitiesTotal comes
// from the live world graph (active count).
func (e *Engine) observe() Observations {
	var o Observations
	_ = e.journal.Range(func(ev *event.Event) error {
		o.WindowEvents++
		switch ev.Kind {
		case event.KindTaskReceived:
			o.TasksStarted++
		case event.KindTaskCompleted:
			o.TasksCompleted++
		case event.KindBriefingSent:
			o.BriefsSent++
		case event.KindSkillActivated:
			o.SkillsActivated++
		case event.KindApprovalGranted:
			o.ApprovalsGranted++
		case event.KindApprovalDenied:
			o.ApprovalsDenied++
		}
		return nil
	})
	if o.TasksStarted > o.TasksCompleted {
		o.TasksFailed = o.TasksStarted - o.TasksCompleted
	}
	o.EntitiesTotal = e.world.Count()
	return o
}

// proposals derives advisory recalibrations from the counts via pure rules —
// no LLM, fully deterministic and testable. Each rule is a judgment/autonomy
// signal the SPEC says must be PROPOSED, never silently applied (§6.4).
func (e *Engine) proposals(o Observations) []Proposal {
	briefVol := e.cfg.BriefVolume
	if briefVol <= 0 {
		briefVol = defaultBriefVolume
	}
	denyExcess := e.cfg.DenyExcess
	if denyExcess <= 0 {
		denyExcess = defaultDenyExcess
	}

	var ps []Proposal
	if o.BriefsSent >= briefVol {
		ps = append(ps, Proposal{
			Area:        "pulse",
			Observation: strconv.Itoa(o.BriefsSent) + " briefs delivered this window",
			Suggestion:  "review the Pulse dial — a quieter setting may reduce noise",
		})
	}
	if o.ApprovalsDenied-o.ApprovalsGranted >= denyExcess {
		ps = append(ps, Proposal{
			Area:        "autonomy",
			Observation: strconv.Itoa(o.ApprovalsDenied) + " approvals denied vs " + strconv.Itoa(o.ApprovalsGranted) + " granted",
			Suggestion:  "consider lowering autonomy on the frequently-denied tools (reflection never raises it)",
		})
	}
	if o.TasksFailed > 0 && o.TasksFailed*2 >= o.TasksStarted && o.TasksStarted > 0 {
		ps = append(ps, Proposal{
			Area:        "tasks",
			Observation: strconv.Itoa(o.TasksFailed) + " of " + strconv.Itoa(o.TasksStarted) + " runs did not complete",
			Suggestion:  "investigate failing/abandoned runs (budget, loops, or tool errors)",
		})
	}
	return ps
}

// Latest reads the newest reflection.completed report back from the journal,
// or ok=false if none has been written. The same "range and keep the last"
// the status/history reads use.
func (e *Engine) Latest() (Report, bool) {
	var (
		latest Report
		found  bool
	)
	_ = e.journal.Range(func(ev *event.Event) error {
		if ev.Kind != event.KindReflectionCompleted {
			return nil
		}
		var r Report
		if json.Unmarshal(ev.Payload, &r) == nil {
			latest = r
			found = true
		}
		return nil
	})
	return latest, found
}

func (e *Engine) publish(corr string, rep Report) {
	if e.bus == nil {
		return
	}
	_, _ = e.bus.Publish(event.Spec{
		Subject:       "reflection.completed",
		Kind:          event.KindReflectionCompleted,
		Actor:         "reflect",
		CorrelationID: corr,
		Payload:       rep,
	})
}
