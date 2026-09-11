// SPDX-License-Identifier: MIT

package cadence

// Engine runtime + helper utilities (RunFunc type, Engine struct,
// NewEngine / Start / Wait / RunningCount / WaitIdle / fireDue / fireOne /
// short / ParseJobs / Describe). Carved out of cadence.go during the
// Day 30 god file split #1.

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)

type RunFunc func(ctx context.Context, id, intent, model string) error

// Engine fires the store's due entries on a timer.
type Engine struct {
	store *Store
	run   RunFunc
	res   time.Duration
	log   io.Writer

	// RunTimeout is a backstop deadline applied to each firing's RunFunc context.
	// Zero means no deadline (the historical behavior). Without it, a single run
	// that hangs (a wedged provider/tool that ignores its own bounds) never lets
	// fireOne return, so its in-flight guard in `running` is never cleared and that
	// entry NEVER fires again — a silent, permanent stall of one schedule. With it
	// set, a ctx-respecting run is cancelled at the deadline, fireOne returns, the
	// guard clears, and the schedule recovers on its next slot. Set before Start.
	RunTimeout time.Duration

	// Bus, when set before Start, receives an anomaly.detected WARNING each
	// time a legacy agent/intent schedule trips the injection scan (M886). The
	// schedule still runs — default-allow; the tripwire makes unattended
	// suspicious agent-task text visible to the alerter/cockpit, it never gates.
	// Typed workflow/system-task/tool labels are metadata, so they are not
	// prompt-injection scanned.
	Bus *bus.Bus

	running sync.Map // entry ID -> struct{} while a run is in flight
	mu      sync.Mutex
	wg      sync.WaitGroup
	started bool
}

// NewEngine builds an Engine over a store. resolution <= 0 uses
// DefaultResolution. log receives one line per firing (nil = discard).
func NewEngine(store *Store, run RunFunc, resolution time.Duration, log io.Writer) *Engine {
	if log == nil {
		log = io.Discard
	}
	res := resolution
	if res <= 0 {
		res = DefaultResolution
	}
	return &Engine{store: store, run: run, res: res, log: log}
}

// Start runs the ticker until ctx is done. It returns immediately; the loop runs
// on its own goroutine.
func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	if e.started {
		e.mu.Unlock()
		return
	}
	e.started = true
	e.wg.Add(1)
	e.mu.Unlock()

	go func() {
		defer e.wg.Done()
		t := time.NewTicker(e.res)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				e.fireDue(ctx, time.Now())
			}
		}
	}()
}

// Wait blocks until the Start loop has observed cancellation and exited. It does
// not wait for already-fired schedule runs; those are intentionally independent
// and bounded by the engine's in-flight guard and optional RunTimeout.
func (e *Engine) Wait() {
	e.wg.Wait()
}

// RunningCount returns how many schedule firings are currently executing. It is
// intentionally small and race-safe so doctor/UI surfaces can distinguish "the
// engine is asleep" from "the engine has work in flight".
func (e *Engine) RunningCount() int {
	c := 0
	e.running.Range(func(_, _ any) bool {
		c++
		return true
	})
	return c
}

// WaitIdle blocks until no schedule firing is in flight or ctx is cancelled.
// It is an observation helper, not a kill switch: hung work should still be
// bounded with RunTimeout.
func (e *Engine) WaitIdle(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	t := time.NewTicker(5 * time.Millisecond)
	defer t.Stop()
	for {
		if e.RunningCount() == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// fireDue launches every due entry that is not already running. Tested directly
// with a controlled clock (no ticker, no flakiness).
func (e *Engine) fireDue(ctx context.Context, now time.Time) {
	for _, entry := range e.store.Due(now) {
		if _, busy := e.running.LoadOrStore(entry.ID, struct{}{}); busy {
			fmt.Fprintf(e.log, "schedule: skip %q (previous run still in progress)\n", short(entry.Intent))
			continue
		}
		ent := entry
		go e.fireOne(ctx, ent)
	}
}

// fireOne runs one due entry and clears its in-flight guard. It recovers from any
// panic so a buggy run — or, more realistically, a panic in the post-run answer
// delivery over a channel plugin, which executes after RunWith's own recover has
// returned, on this goroutine — can never crash the whole daemon. This mirrors the
// containment guarantee kernel/standing makes via safeFire (M420). Synchronous so it
// is directly testable.
func (e *Engine) fireOne(ctx context.Context, ent Entry) {
	defer e.running.Delete(ent.ID)
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(e.log, "schedule: %q panicked (contained): %v\n", short(ent.Intent), r)
		}
	}()
	fmt.Fprintf(e.log, "schedule: firing %q (%s)\n", short(ent.Intent), ent.Cadence())
	// Injection tripwire (M886): journal a warning when legacy agent-task text
	// looks like a prompt-injection payload, then fire anyway (default-allow).
	// Scanned at fire time — the single choke point every creation path funnels
	// through, including schedules that predate the scan. Typed targets keep
	// their executable semantics outside the schedule label, so scanning them
	// would create noise without reducing prompt risk.
	var markers []string
	if ent.Target == TargetIntent {
		markers = SuspiciousIntent(ent.Intent)
	}
	if len(markers) > 0 {
		fmt.Fprintf(e.log, "schedule: %q trips injection markers %v (firing anyway)\n", short(ent.Intent), markers)
		if e.Bus != nil {
			_, _ = e.Bus.Publish(event.Spec{
				Subject: "cadence.injection",
				Kind:    event.KindAnomalyDetected,
				Actor:   "cadence",
				Payload: map[string]any{
					"anomaly":     "schedule_intent_injection_suspect",
					"schedule_id": ent.ID,
					"markers":     markers,
					"intent":      short(ent.Intent),
					"severity":    "warning",
				},
			})
		}
	}
	runCtx := ctx
	if e.RunTimeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, e.RunTimeout)
		defer cancel()
	}
	if err := e.run(runCtx, ent.ID, ent.Intent, ent.Model); err != nil {
		fmt.Fprintf(e.log, "schedule: %q failed: %v\n", short(ent.Intent), err)
	}
	// A one-shot is removed only after its run completes, so a crash mid-run leaves
	// it in the store to re-fire on restart (M199). This runs before the deferred
	// running.Delete, so no tick can re-fire it in the gap between removal and
	// clearing the in-flight guard.
	if _, err := e.store.CompleteFiring(ent.ID, time.Now()); err != nil {
		fmt.Fprintf(e.log, "schedule: completing %q failed: %v\n", short(ent.Intent), err)
	}
}

func short(s string) string {
	s = strings.TrimSpace(s)
	// Truncate on a rune boundary (48 characters, not bytes) so a multi-byte
	// rune — e.g. a Turkish ç/ş/ğ in a schedule label — is never split into
	// invalid UTF-8 in the log / `describe` output.
	if r := []rune(s); len(r) > 48 {
		return string(r[:48]) + "…"
	}
	return s
}

// --- env parsing ---

// ParseJobs parses the legacy AGEZT_SCHEDULE spec: a semicolon-separated list
// of jobs (semicolon, not comma, because labels commonly contain commas), each
// "interval=agent-task". The interval is a Go duration (e.g. 30m, 1h, 24h); the
// task label is the rest of the entry verbatim. A malformed entry is a hard
// error so a misconfigured schedule is caught at startup, not silently dropped.
func ParseJobs(spec string) ([]Job, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	var jobs []Job
	for _, raw := range strings.Split(spec, ";") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		durStr, intent, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("cadence: entry %q must be interval=agent-task", entry)
		}
		d, err := time.ParseDuration(strings.TrimSpace(durStr))
		if err != nil {
			return nil, fmt.Errorf("cadence: bad interval %q: %w", durStr, err)
		}
		if d < MinInterval {
			return nil, fmt.Errorf("cadence: interval %s is below the %s minimum", d, MinInterval)
		}
		intent = strings.TrimSpace(intent)
		if intent == "" {
			return nil, fmt.Errorf("cadence: entry %q has an empty intent", entry)
		}
		jobs = append(jobs, Job{Interval: d, Intent: intent})
	}
	return jobs, nil
}

// Describe renders a one-line banner summary of the store's entries.
func Describe(entries []Entry) string {
	if len(entries) == 0 {
		return ""
	}
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, fmt.Sprintf("%s → %q", e.Cadence(), short(e.Intent)))
	}
	return fmt.Sprintf("%d schedule(s): %s", len(entries), strings.Join(parts, ", "))
}

