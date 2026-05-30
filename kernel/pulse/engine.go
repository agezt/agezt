// SPDX-License-Identifier: MIT

package pulse

import (
	"context"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/ulid"
	"github.com/agezt/agezt/kernel/warden"
)

// Default tuning.
const (
	defaultCadence     = 60 * time.Second
	defaultNoveltyTTL  = 30 * time.Minute
	defaultDigestEvery = 30 // flush the digest every N beats
)

// Config constructs an Engine. Only Bus is strictly required; everything else
// has a safe default (no observers = a heartbeat that does nothing).
type Config struct {
	Bus         *bus.Bus
	State       *state.FileStore // for novelty cache + observer state; optional
	Warden      warden.Engine    // for the probe observer; optional
	Provider    agent.Provider   // for the optional LLM salience refine
	Model       string
	Observers   []Observer
	Dial        Dial
	Cadence     time.Duration
	QuietHours  QuietHours
	UseLLM      bool
	NoveltyTTL  time.Duration
	Sink        BriefSink
	DigestEvery int
	Now         func() time.Time
}

// Engine is the Pulse resident: a heartbeat that fans out to observers, scores
// the deltas, decides inform-or-ask, and briefs the user — every stage
// journaled (SPEC-03 §1).
type Engine struct {
	bus       *bus.Bus
	sal       *Salience
	observers []Observer
	dial      Dial
	cadence   time.Duration
	quiet     QuietHours
	sink      BriefSink
	now       func() time.Time

	digestEvery int

	mu         sync.Mutex
	paused     bool
	ticks      int64
	lastTickMS int64
	started    time.Time
	digest     []Brief
}

// New builds an Engine, filling defaults.
func New(cfg Config) *Engine {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	cadence := cfg.Cadence
	if cadence <= 0 {
		cadence = defaultCadence
	}
	ttl := cfg.NoveltyTTL
	if ttl <= 0 {
		ttl = defaultNoveltyTTL
	}
	digestEvery := cfg.DigestEvery
	if digestEvery <= 0 {
		digestEvery = defaultDigestEvery
	}
	dial := ParseDial(string(cfg.Dial))
	var sink BriefSink = cfg.Sink
	if sink == nil {
		sink = LogSink{} // nil writer → no-op delivery (still journaled)
	}
	return &Engine{
		bus:         cfg.Bus,
		observers:   cfg.Observers,
		dial:        dial,
		cadence:     cadence,
		quiet:       cfg.QuietHours,
		sink:        sink,
		now:         now,
		digestEvery: digestEvery,
		started:     now(),
		sal: &Salience{
			state:      cfg.State,
			provider:   cfg.Provider,
			model:      cfg.Model,
			dial:       dial,
			useLLM:     cfg.UseLLM,
			noveltyTTL: ttl,
			now:        now,
		},
	}
}

// Start runs the heartbeat until ctx is cancelled (which is how `agt halt`,
// SIGTERM, and `agt shutdown` stop Pulse along with everything else). Returns
// immediately; the loop runs in a goroutine.
func (e *Engine) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(e.cadence)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if e.IsPaused() {
					continue
				}
				e.tickOnce(ctx)
			}
		}
	}()
}

// tickOnce executes a single heartbeat: publish the tick, poll observers, and
// run each delta through salience → initiative → briefing. Exposed for
// deterministic tests (drive beats without a real ticker).
func (e *Engine) tickOnce(ctx context.Context) {
	e.mu.Lock()
	e.ticks++
	n := e.ticks
	e.lastTickMS = e.now().UnixMilli()
	e.mu.Unlock()

	tickEv, _ := e.publish(event.KindPulseTick, "pulse.tick", "pulse-"+ulid.New(), "", map[string]any{
		"beat":      n,
		"observers": len(e.observers),
	})
	tickID := ""
	if tickEv != nil {
		tickID = tickEv.ID
	}

	for _, obs := range e.observers {
		deltas, err := obs.Poll(ctx)
		if err != nil {
			e.publish(event.KindObserverDelta, "pulse.observer."+obs.Name(), "pulse-"+ulid.New(), tickID, map[string]any{
				"observer": obs.Name(),
				"error":    err.Error(),
			})
			continue
		}
		for _, d := range deltas {
			e.process(ctx, d, tickID)
		}
	}

	if n%int64(e.digestEvery) == 0 {
		e.flushDigest()
	}
}

// process runs one delta through the remaining three stages.
func (e *Engine) process(ctx context.Context, d Delta, tickID string) {
	corr := "pulse-" + ulid.New()

	e.publish(event.KindObserverDelta, "pulse.observer."+d.Source, corr, tickID, map[string]any{
		"source":  d.Source,
		"kind":    d.Kind,
		"summary": d.Summary,
		"before":  d.Before,
		"after":   d.After,
		"hints":   d.Hints,
	})

	sc := e.sal.Score(ctx, d)
	e.publish(event.KindSalienceScored, "pulse.salience", corr, tickID, map[string]any{
		"source":      d.Source,
		"score":       sc.Value,
		"reason":      sc.Reason,
		"disposition": string(sc.Disposition),
	})
	if sc.Disposition == DispDrop {
		return
	}

	delivery := Route(e.dial, sc.Disposition, e.quiet.Active(e.now()))
	if delivery == DeliverDrop {
		return
	}

	// Initiative v1: inform-or-ask only. `act` is downgraded to `ask` (no
	// autonomous fixing yet — SPEC-03 §9.4); everything else is `inform`.
	branch := "inform"
	if sc.Disposition == DispAct {
		branch = "ask"
	}
	e.publish(event.KindInitiativeTaken, "pulse.initiative", corr, tickID, map[string]any{
		"source": d.Source,
		"branch": branch,
		"reason": sc.Reason,
	})

	// Mark the issue surfaced so an identical repeat within the TTL is
	// suppressed by novelty.
	e.sal.MarkSeen(d.IssueKey())

	switch delivery {
	case DeliverNow:
		b := composeBrief(d, sc, corr)
		_ = e.sink.Deliver(b)
		e.publish(event.KindBriefingSent, "pulse.briefing", corr, tickID, briefPayload(b))
	case DeliverDigest:
		e.mu.Lock()
		e.digest = append(e.digest, composeBrief(d, sc, corr))
		e.mu.Unlock()
	}
}

// flushDigest coalesces accumulated digest items into one brief and delivers
// it (SPEC-03 §6.2). No-op when empty.
func (e *Engine) flushDigest() {
	e.mu.Lock()
	items := e.digest
	e.digest = nil
	e.mu.Unlock()
	if len(items) == 0 {
		return
	}
	b := composeDigest(items)
	corr := "pulse-" + ulid.New()
	b.CorrelationID = corr
	_ = e.sink.Deliver(b)
	e.publish(event.KindBriefingSent, "pulse.briefing", corr, "", briefPayload(b))
}

// publish is the journaling helper: durable-before-publish through the bus,
// carrying correlation (per-delta chain) and causation (the originating tick).
func (e *Engine) publish(kind event.Kind, subject, corr, causation string, payload any) (*event.Event, error) {
	if e.bus == nil {
		return nil, nil
	}
	return e.bus.Publish(event.Spec{
		Subject:       subject,
		Kind:          kind,
		Actor:         "pulse",
		CorrelationID: corr,
		CausationID:   causation,
		Payload:       payload,
	})
}

func briefPayload(b Brief) map[string]any {
	return map[string]any{
		"title":       b.Title,
		"body":        b.Body,
		"disposition": string(b.Disposition),
		"issue_key":   b.IssueKey,
		"items":       b.Items,
	}
}

// --- control surface (used by `agt pulse status|pause|resume`) ------------

// Status is the snapshot returned to `agt pulse status`.
type Status struct {
	Running       bool     `json:"running"`
	Paused        bool     `json:"paused"`
	Beats         int64    `json:"beats"`
	Observers     []string `json:"observers"`
	Dial          string   `json:"dial"`
	CadenceMS     int64    `json:"cadence_ms"`
	LastTickMS    int64    `json:"last_tick_ms"`
	DigestPending int      `json:"digest_pending"`
}

// Status returns a snapshot of the engine for operators.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	names := make([]string, 0, len(e.observers))
	for _, o := range e.observers {
		names = append(names, o.Name())
	}
	return Status{
		Running:       !e.paused,
		Paused:        e.paused,
		Beats:         e.ticks,
		Observers:     names,
		Dial:          string(e.dial),
		CadenceMS:     e.cadence.Milliseconds(),
		LastTickMS:    e.lastTickMS,
		DigestPending: len(e.digest),
	}
}

// StatusMap is the control-plane-facing snapshot: the same data as Status()
// but as a map[string]any so the control plane can return it directly without
// importing this package (it depends on the interface, the daemon injects the
// engine).
func (e *Engine) StatusMap() map[string]any {
	s := e.Status()
	return map[string]any{
		"running":        s.Running,
		"paused":         s.Paused,
		"beats":          s.Beats,
		"observers":      s.Observers,
		"dial":           s.Dial,
		"cadence_ms":     s.CadenceMS,
		"last_tick_ms":   s.LastTickMS,
		"digest_pending": s.DigestPending,
	}
}

// IsPaused reports whether beats are currently suppressed.
func (e *Engine) IsPaused() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.paused
}

// Pause suppresses new beats (in-flight processing finishes). Journaled.
func (e *Engine) Pause() {
	e.mu.Lock()
	if e.paused {
		e.mu.Unlock()
		return
	}
	e.paused = true
	e.mu.Unlock()
	e.publish(event.KindPulsePaused, "pulse.control", "", "", map[string]any{"paused": true})
}

// Resume re-enables beats. Journaled.
func (e *Engine) Resume() {
	e.mu.Lock()
	if !e.paused {
		e.mu.Unlock()
		return
	}
	e.paused = false
	e.mu.Unlock()
	e.publish(event.KindPulseResumed, "pulse.control", "", "", map[string]any{"paused": false})
}
