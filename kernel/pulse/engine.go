// SPDX-License-Identifier: MIT

package pulse

import (
	"context"
	"fmt"
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
	Relevance   Relevance // world-model relevance signal; optional
	Observers   []Observer
	Dial        Dial
	Initiative  InitiativeLevel // autonomy level (off|ask|act); default act
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
	bus        *bus.Bus
	sal        *Salience
	observers  []Observer
	dial       Dial
	initiative InitiativeLevel
	cadence    time.Duration
	quiet      QuietHours
	sink       BriefSink
	now        func() time.Time

	digestEvery int

	// beat carries on-demand "think now" requests (M756) into the Start loop, so a
	// manual beat runs on the same goroutine as the cadence ticks — never racing one.
	// Buffered (1) so a request is accepted without blocking; extra requests while one
	// is pending coalesce.
	beat chan struct{}

	// retune signals the Start loop to reset its ticker after a live cadence change
	// (M757). It's just a wakeup; the new interval is read from e.cadence under mu, so
	// the latest value always wins even if several changes coalesce.
	retune chan struct{}

	mu         sync.Mutex
	paused     bool
	ticks      int64
	lastTickMS int64
	started    time.Time
	digest     []Brief

	// removable tracks which observers were added at runtime (M767/M768) and so may be
	// removed again (M769). Keyed by the observer instance — all three observer types are
	// pointers, hence comparable — so a runtime "system:disk" watch can be removed without
	// touching a startup disk observer that happens to share the same Name().
	removable map[Observer]bool

	// asks holds the actionable observations raised under initiative=ask that are
	// awaiting an operator verdict (M1001). Keyed by issue_key so a repeated signal
	// updates in place rather than stacking. The operator approves one (re-emitted as
	// pulse.initiative.act, taking the normal act path) or rejects it from the Jarvis
	// presence pillar. Bounded by trimming oldest past maxPendingAsks.
	asks map[string]*pendingAsk
}

// maxPendingAsks bounds the pending-ask queue so a chatty observer can't grow it
// without limit; the oldest are trimmed first.
const maxPendingAsks = 50

// pendingAsk is one actionable observation waiting on the operator (M1001). It keeps
// the original act-event payload so an approval can re-emit it verbatim onto the act
// subject the responder already binds to.
type pendingAsk struct {
	IssueKey string         `json:"issue_key"`
	Source   string         `json:"source"`
	Kind     string         `json:"kind"`
	Summary  string         `json:"summary"`
	Reason   string         `json:"reason"`
	Score    float64        `json:"score"`
	TS       int64          `json:"ts_unix_ms"`
	payload  map[string]any // the act payload, re-emitted verbatim on approval
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
		initiative:  ParseInitiative(string(cfg.Initiative)),
		cadence:     cadence,
		quiet:       cfg.QuietHours,
		sink:        sink,
		now:         now,
		digestEvery: digestEvery,
		beat:        make(chan struct{}, 1),
		retune:      make(chan struct{}, 1),
		removable:   map[Observer]bool{},
		asks:        map[string]*pendingAsk{},
		started:     now(),
		sal: &Salience{
			state:      cfg.State,
			provider:   cfg.Provider,
			model:      cfg.Model,
			relevance:  cfg.Relevance,
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
				// When ctx is cancelled AND a tick is ready, select picks a case at
				// random — so a fast ticker could keep firing beats after cancel
				// until the random draw lands on ctx.Done(). Re-check here so a
				// cancelled engine stops promptly (at most one in-flight tick) rather
				// than racing the scheduler.
				if ctx.Err() != nil {
					return
				}
				if e.IsPaused() {
					continue
				}
				e.tickOnce(ctx)
			case <-e.beat:
				// On-demand beat (M756): an explicit operator "think now". Runs on this
				// same goroutine, so it never races a scheduled tick. Fires even when
				// paused — the operator asked for one beat, distinct from resuming the
				// cadence — but still stops if the daemon is shutting down.
				if ctx.Err() != nil {
					return
				}
				e.tickOnce(ctx)
			case <-e.retune:
				// Live cadence change (M757): reset the ticker to the new interval. The
				// value is read from e.cadence under mu, so the latest SetCadence wins.
				e.mu.Lock()
				c := e.cadence
				e.mu.Unlock()
				ticker.Reset(c)
			}
		}
	}()
}

// Beat requests a single on-demand heartbeat (M756) — the operator's "think now".
// It's non-blocking: the request is handed to the Start loop, which runs the beat on
// its own goroutine (serialized with cadence ticks, so no race). The resulting
// observations/initiatives surface asynchronously, exactly like a scheduled tick.
// A no-op if a manual beat is already pending (coalesced) or if Start never ran
// (the control plane reports Pulse as disabled in that case).
func (e *Engine) Beat() {
	select {
	case e.beat <- struct{}{}:
	default: // one already pending — coalesce
	}
}

// Cadence bounds for live retuning (M757): fast enough to be responsive, slow
// enough not to hammer providers; never zero (which would busy-spin the ticker).
const (
	minCadence = 5 * time.Second
	maxCadence = 24 * time.Hour
)

// SetCadence changes the heartbeat interval live (M757), clamped to [5s, 24h], and
// returns the applied value. It takes effect on the next beat — the Start loop resets
// its ticker. Runtime-only: like pause state, it resets to the configured default
// (AGEZT_PULSE_CADENCE) when the daemon restarts.
func (e *Engine) SetCadence(d time.Duration) time.Duration {
	if d < minCadence {
		d = minCadence
	}
	if d > maxCadence {
		d = maxCadence
	}
	e.mu.Lock()
	e.cadence = d
	e.mu.Unlock()
	select {
	case e.retune <- struct{}{}:
	default: // a retune is already pending; it'll read the latest e.cadence
	}
	return d
}

// SetDial changes the proactivity dial live (M758): quiet (only alerts reach you),
// balanced (notify and up), or chatty (digests too). An unknown value normalizes to
// balanced (ParseDial). Takes effect on the next delta; returns the applied dial.
// Runtime-only — resets to the configured default on restart.
func (e *Engine) SetDial(s string) string {
	nd := ParseDial(s)
	e.mu.Lock()
	e.dial = nd
	e.sal.dial = nd
	e.mu.Unlock()
	return string(nd)
}

// SetInitiative changes the autonomy level live (M999): off (inform only), ask
// (emit a pulse.initiative.ask event for approval), or act (emit pulse.initiative.act
// so a bound standing order fires). An unknown value normalizes to act
// (ParseInitiative). Takes effect on the next delta; returns the applied level.
// Runtime-only — resets to the configured default (AGEZT_PULSE_INITIATIVE) on restart.
func (e *Engine) SetInitiative(s string) string {
	ni := ParseInitiative(s)
	e.mu.Lock()
	e.initiative = ni
	e.mu.Unlock()
	return string(ni)
}

// SetQuietHours changes the quiet window live (M770): during it, only alert/act briefs
// break through (lower-priority observations are held), regardless of the dial. spec is
// the "START-END" 24h form ParseQuietHours accepts (e.g. "22-7"); an empty or invalid
// spec disables quiet hours. Takes effect on the next delta (process reads e.quiet under
// the lock); returns the applied canonical spec ("" when disabled). Runtime-only — resets
// to the configured default on restart unless persisted.
func (e *Engine) SetQuietHours(spec string) string {
	q := ParseQuietHours(spec)
	e.mu.Lock()
	e.quiet = q
	e.mu.Unlock()
	return q.Spec()
}

// queueAsk records (or refreshes) a pending ask, trimming the oldest if the queue
// is full so a chatty observer can't grow it without bound (M1001).
func (e *Engine) queueAsk(a *pendingAsk) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.asks[a.IssueKey] = a
	for len(e.asks) > maxPendingAsks {
		var oldestKey string
		var oldestTS int64 = 1<<63 - 1
		for k, v := range e.asks {
			if v.TS < oldestTS {
				oldestTS, oldestKey = v.TS, k
			}
		}
		delete(e.asks, oldestKey)
	}
}

// PendingAsks returns the queued asks awaiting an operator verdict (M1001), newest
// first, as plain maps the control plane can return without importing this package.
func (e *Engine) PendingAsks() []map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]map[string]any, 0, len(e.asks))
	for _, a := range e.asks {
		out = append(out, map[string]any{
			"issue_key":  a.IssueKey,
			"source":     a.Source,
			"kind":       a.Kind,
			"summary":    a.Summary,
			"reason":     a.Reason,
			"score":      a.Score,
			"ts_unix_ms": a.TS,
		})
	}
	// Newest first; a stable order keeps the UI from reshuffling on each poll.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j]["ts_unix_ms"].(int64) > out[i]["ts_unix_ms"].(int64) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// ResolveAsk settles a pending ask (M1001). approve=true re-emits the original signal
// onto pulse.initiative.act — the exact path act-mode takes, so the responder (when
// enabled) fires a governed run; approve=false just drops it. Returns whether the key
// was found and, on approval, whether the act event was emitted.
func (e *Engine) ResolveAsk(issueKey string, approve bool) (found, acted bool) {
	e.mu.Lock()
	a := e.asks[issueKey]
	if a != nil {
		delete(e.asks, issueKey)
	}
	e.mu.Unlock()
	if a == nil {
		return false, false
	}
	if approve {
		// Re-emit verbatim onto the act subject the responder binds to. The engine still
		// takes no action itself — it only promotes ask→act; the governed standing runner
		// does the rest (and does nothing if the operator left the responder disabled).
		e.publish(event.KindInitiativeAct, "pulse.initiative.act", "pulse-"+ulid.New(), "", a.payload)
		return true, true
	}
	return true, false
}

// tickOnce executes a single heartbeat: publish the tick, poll observers, and
// run each delta through salience → initiative → briefing. Exposed for
// deterministic tests (drive beats without a real ticker).
func (e *Engine) tickOnce(ctx context.Context) {
	e.mu.Lock()
	e.ticks++
	n := e.ticks
	e.lastTickMS = e.now().UnixMilli()
	// Snapshot the observers under the lock — AddObserver (M767) can append from the
	// control-plane goroutine while this beat iterates them.
	obs := make([]Observer, len(e.observers))
	copy(obs, e.observers)
	e.mu.Unlock()

	tickEv, _ := e.publish(event.KindPulseTick, "pulse.tick", "pulse-"+ulid.New(), "", map[string]any{
		"beat":      n,
		"observers": len(obs),
	})
	tickID := ""
	if tickEv != nil {
		tickID = tickEv.ID
	}

	for _, o := range obs {
		e.safePoll(ctx, o, tickID)
	}

	if n%int64(e.digestEvery) == 0 {
		e.safeFlushDigest()
	}
}

// AddObserver registers a new observer at runtime (M767) — e.g. an operator adding a
// disk watch from the console. Appended under the lock; the next beat picks it up
// (tickOnce snapshots the slice under the same lock). Returns the observer's name.
func (e *Engine) AddObserver(o Observer) string {
	e.mu.Lock()
	e.observers = append(e.observers, o)
	e.removable[o] = true
	e.mu.Unlock()
	return o.Name()
}

// RemoveObserver removes runtime-added observers whose Name() matches (M769) — the
// inverse of AddObserver, so a console-added disk or command watch can be stopped
// without restarting the daemon. Only observers registered via AddObserver are
// removable; startup observers (self:health and any AGEZT_PULSE_* probes) are never
// removed, even on a name collision. Returns how many were dropped (0 if none matched
// or the name belongs only to a permanent observer). Takes effect on the next beat
// (tickOnce snapshots the slice under this same lock).
func (e *Engine) RemoveObserver(name string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	kept := e.observers[:0:0]
	removed := 0
	for _, o := range e.observers {
		if o.Name() == name && e.removable[o] {
			delete(e.removable, o)
			removed++
			continue
		}
		kept = append(kept, o)
	}
	e.observers = kept
	return removed
}

// safePoll polls one observer and runs its deltas through the pipeline, recovering
// from any panic so a buggy observer, a panicking provider in the salience refine, or
// a panicking briefing sink can never crash the whole daemon (M423). The pulse loop
// runs on a single resident goroutine with no recovering frame, so without this an
// observer/provider/sink panic terminates the process — every channel, the control
// plane, and all in-flight runs with it. Mirrors kernel/standing's safeFire and
// kernel/cadence's fireOne. The panic is journaled so it stays diagnosable.
func (e *Engine) safePoll(ctx context.Context, obs Observer, tickID string) {
	defer func() {
		if r := recover(); r != nil {
			e.publish(event.KindObserverDelta, "pulse.observer."+obs.Name(), "pulse-"+ulid.New(), tickID, map[string]any{
				"observer": obs.Name(),
				"error":    fmt.Sprintf("panic (contained): %v", r),
			})
		}
	}()
	deltas, err := obs.Poll(ctx)
	if err != nil {
		e.publish(event.KindObserverDelta, "pulse.observer."+obs.Name(), "pulse-"+ulid.New(), tickID, map[string]any{
			"observer": obs.Name(),
			"error":    err.Error(),
		})
		return
	}
	for _, d := range deltas {
		e.process(ctx, d, tickID)
	}
}

// FlushDigest delivers any accumulated digest items immediately (M761) instead of
// waiting for the periodic flush (every digestEvery beats) — "surface what you've been
// holding". Returns the number of items flushed (0 if the digest was empty). Safe to
// call from any goroutine: the digest is swapped under the lock, so a concurrent
// periodic flush can't double-deliver.
func (e *Engine) FlushDigest() int { return e.safeFlushDigest() }

// safeFlushDigest flushes the digest with the same panic containment as safePoll —
// a panicking briefing sink in the periodic digest must not crash the daemon (M423).
func (e *Engine) safeFlushDigest() (n int) {
	defer func() { _ = recover() }()
	return e.flushDigest()
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

	// Read the dial and quiet window under the lock — SetDial (M758) and SetQuietHours
	// (M770) can change them live from the control-plane goroutine while this scoring
	// runs on the pulse loop.
	e.mu.Lock()
	dial := e.dial
	quiet := e.quiet
	e.mu.Unlock()
	delivery := Route(dial, sc.Disposition, quiet.Active(e.now()))
	if delivery == DeliverDrop {
		return
	}

	// Initiative (M999): when an observation is ACTIONABLE and the operator has
	// enabled autonomy, emit a distinct event that a standing order can fire on —
	// reusing the governed standing→RunWith→policyHook path. The engine itself never
	// acts (it owns no permissions); it only classifies and emits. The level is read
	// live under the lock, like the dial.
	e.mu.Lock()
	initiative := e.initiative
	e.mu.Unlock()
	actionable := sc.Disposition == DispAlert || sc.Disposition == DispAct || d.Hints["actionable"] == "true"
	branch := "inform"
	switch {
	case actionable && initiative == InitiativeAct:
		branch = "act"
	case actionable && initiative == InitiativeAsk:
		branch = "ask"
	}
	e.publish(event.KindInitiativeTaken, "pulse.initiative", corr, tickID, map[string]any{
		"source": d.Source,
		"branch": branch,
		"reason": sc.Reason,
	})
	// The actionable signal: a separate subject standing orders bind to (mirrors how
	// pulse.observer.<source> already drives guardians). Subject distinguishes
	// act vs ask; the payload carries enough for the fired agent to triage.
	if branch == "act" || branch == "ask" {
		payload := map[string]any{
			"source":    d.Source,
			"kind":      d.Kind,
			"summary":   d.Summary,
			"reason":    sc.Reason,
			"score":     sc.Value,
			"issue_key": d.IssueKey(),
		}
		e.publish(event.KindInitiativeAct, "pulse.initiative."+branch, corr, tickID, payload)
		// Under ask, the act subject was NOT fired — queue the signal for the operator's
		// verdict so it isn't a silent dead-end (M1001). Approval re-emits `payload` onto
		// pulse.initiative.act, the same path act-mode takes.
		if branch == "ask" {
			e.queueAsk(&pendingAsk{
				IssueKey: d.IssueKey(),
				Source:   d.Source,
				Kind:     d.Kind,
				Summary:  d.Summary,
				Reason:   sc.Reason,
				Score:    sc.Value,
				TS:       e.now().UnixMilli(),
				payload:  payload,
			})
		}
	}

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
func (e *Engine) flushDigest() int {
	e.mu.Lock()
	items := e.digest
	e.digest = nil
	e.mu.Unlock()
	if len(items) == 0 {
		return 0
	}
	b := composeDigest(items)
	corr := "pulse-" + ulid.New()
	b.CorrelationID = corr
	_ = e.sink.Deliver(b)
	e.publish(event.KindBriefingSent, "pulse.briefing", corr, "", briefPayload(b))
	return len(items)
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
	Running       bool       `json:"running"`
	Paused        bool       `json:"paused"`
	Beats         int64      `json:"beats"`
	Observers     []string   `json:"observers"`
	Removable     []string   `json:"removable"`
	Dial          string     `json:"dial"`
	Initiative    string     `json:"initiative"` // autonomy level (off|ask|act); M999
	Quiet         QuietHours `json:"quiet"`
	CadenceMS     int64      `json:"cadence_ms"`
	LastTickMS    int64      `json:"last_tick_ms"`
	DigestPending int        `json:"digest_pending"`
}

// Status returns a snapshot of the engine for operators.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	names := make([]string, 0, len(e.observers))
	removable := make([]string, 0, len(e.removable))
	seenRemovable := map[string]bool{}
	for _, o := range e.observers {
		names = append(names, o.Name())
		// A removable observer's name is reported once even if several share it (all the
		// disk watches register as "system:disk"); RemoveObserver(name) drops them together.
		if e.removable[o] && !seenRemovable[o.Name()] {
			seenRemovable[o.Name()] = true
			removable = append(removable, o.Name())
		}
	}
	return Status{
		Running:       !e.paused,
		Paused:        e.paused,
		Beats:         e.ticks,
		Observers:     names,
		Removable:     removable,
		Dial:          string(e.dial),
		Initiative:    string(e.initiative),
		Quiet:         e.quiet,
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
		"removable":      s.Removable,
		"dial":           s.Dial,
		"initiative":     s.Initiative,
		"quiet":          map[string]any{"enabled": s.Quiet.Enabled, "start": s.Quiet.Start, "end": s.Quiet.End, "spec": s.Quiet.Spec()},
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
