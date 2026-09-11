// SPDX-License-Identifier: MIT

// Pulse engine core: constants + Config + New + Start + Beat + setters (SetCadence/Dial/Initiative/QuietHours) + queueAsk.
// Code extracted from engine.go during the Day-54 god-file split. Public API unchanged.
package pulse


import (
	"context"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/state"
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
