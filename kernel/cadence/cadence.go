// SPDX-License-Identifier: MIT

// Package cadence is the scheduled-intents resident (autonomy): it fires
// operator-configured intents on a recurring timer through the normal governed
// kernel loop, so the system acts on its own ("every morning, summarise new
// commits and brief me") rather than only reacting. It is the timer companion
// to Pulse's event-driven proactivity.
//
// Each job is an (interval, intent) pair. A single ticker fires every job whose
// next-run time has arrived; a still-running job is skipped (no overlap) so a
// slow run can't pile up. Every firing is journaled (schedule.fired) and the run
// it launches goes through Edict + the journal like any other — a scheduled run
// is governed exactly like `agt run`, with no extra authority.
//
// Configuration is env-driven (AGEZT_SCHEDULE); like the webhook and reflect
// residents it has no CLI/control-plane surface of its own in this MVP.
package cadence

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MinInterval guards against a busy-loop from a misconfigured tiny interval.
const MinInterval = time.Second

// DefaultResolution is how often the ticker wakes to check for due jobs.
const DefaultResolution = 30 * time.Second

// Job is one scheduled intent.
type Job struct {
	Interval time.Duration
	Intent   string
	Model    string // optional; empty → daemon default
}

// RunFunc executes one scheduled intent through the governed loop. The engine
// calls it on its own goroutine; a returned error is logged, not fatal.
type RunFunc func(ctx context.Context, intent, model string) error

type jobState struct {
	Job
	next    time.Time
	running atomic.Bool
}

// Engine fires the configured jobs on a timer.
type Engine struct {
	jobs []*jobState
	run  RunFunc
	res  time.Duration
	log  io.Writer

	mu      sync.Mutex
	started bool
}

// New builds an Engine. resolution <= 0 uses DefaultResolution (clamped to the
// smallest interval so short intervals still fire promptly). log receives one
// line per firing (nil = discard).
func New(jobs []Job, run RunFunc, resolution time.Duration, log io.Writer) *Engine {
	if log == nil {
		log = io.Discard
	}
	states := make([]*jobState, 0, len(jobs))
	smallest := time.Duration(0)
	for _, j := range jobs {
		if j.Interval < MinInterval {
			j.Interval = MinInterval
		}
		states = append(states, &jobState{Job: j})
		if smallest == 0 || j.Interval < smallest {
			smallest = j.Interval
		}
	}
	res := resolution
	if res <= 0 {
		res = DefaultResolution
	}
	if smallest > 0 && smallest < res {
		res = smallest
	}
	return &Engine{jobs: states, run: run, res: res, log: log}
}

// Start arms every job (first fire one interval from now) and runs the ticker
// until ctx is done. It returns immediately; the loop runs on its own goroutine.
func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	if e.started || len(e.jobs) == 0 {
		e.mu.Unlock()
		return
	}
	e.started = true
	e.mu.Unlock()

	now := time.Now()
	for _, j := range e.jobs {
		j.next = now.Add(j.Interval)
	}
	go func() {
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

// fireDue launches every job whose next-run time has arrived and that is not
// already running, rescheduling each. Exported behaviour is tested directly with
// a controlled clock (no ticker, no flakiness).
func (e *Engine) fireDue(ctx context.Context, now time.Time) {
	for _, j := range e.jobs {
		if j.next.IsZero() {
			j.next = now.Add(j.Interval) // lazy arm (used by tests that skip Start)
			continue
		}
		if now.Before(j.next) {
			continue
		}
		j.next = now.Add(j.Interval)
		if !j.running.CompareAndSwap(false, true) {
			fmt.Fprintf(e.log, "schedule: skip %q (previous run still in progress)\n", short(j.Intent))
			continue
		}
		job := j
		go func() {
			defer job.running.Store(false)
			fmt.Fprintf(e.log, "schedule: firing %q (every %s)\n", short(job.Intent), job.Interval)
			if err := e.run(ctx, job.Intent, job.Model); err != nil {
				fmt.Fprintf(e.log, "schedule: %q failed: %v\n", short(job.Intent), err)
			}
		}()
	}
}

func short(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 48 {
		return s[:48] + "…"
	}
	return s
}

// ParseJobs parses the AGEZT_SCHEDULE spec: a semicolon-separated list of jobs
// (semicolon, not comma, because intents commonly contain commas), each
// "interval=intent". The interval is a Go duration (e.g. 30m, 1h, 24h); the
// intent is the rest of the entry verbatim. A malformed entry is a hard error so
// a misconfigured schedule is caught at startup, not silently dropped.
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
			return nil, fmt.Errorf("cadence: entry %q must be interval=intent", entry)
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

// Describe renders a one-line banner summary of the jobs.
func Describe(jobs []Job) string {
	if len(jobs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(jobs))
	for _, j := range jobs {
		parts = append(parts, fmt.Sprintf("every %s → %q", j.Interval, short(j.Intent)))
	}
	return fmt.Sprintf("%d job(s): %s", len(jobs), strings.Join(parts, ", "))
}
