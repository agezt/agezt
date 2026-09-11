// SPDX-License-Identifier: MIT

// Forge: skill-store wrapper, constructors, setters, read ops, and small helpers.
// Code extracted from forge.go during the Day-36 god-file split. Public API unchanged.
package skill


import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)


// Forge wraps a Store with the kernel bus so every skill transition is
// journaled (durable-before-publish) under the originating run's correlation.
// This is the journaling boundary — the Store stays pure. It mirrors
// kernel/memory.Manager and kernel/worldmodel.Graph; the state machine
// (skill.go) is the only thing that makes Forge more than a third copy.
type Forge struct {
	store Store
	bus   *bus.Bus
	// bundles holds the on-disk resources (reference files + scripts) that travel
	// with a skill (agentskills.io shape, SPEC-13). Optional: nil in store-only
	// tests and on daemons without a bundle store; when nil, Create simply ignores
	// any Resources and a skill stays body-only. Injected via SetBundles.
	bundles *BundleStore
	// now is the clock, injectable for deterministic tests.
	now func() time.Time
	// mu serialises the read-modify-write lifecycle mutators so two concurrent runs
	// (each run calls Activate then RecordOutcome on the shared Forge, and the
	// control plane serves connections concurrently) cannot interleave their Get→Put
	// pairs — losing a metric update or, worse, resurrecting a just-quarantined skill
	// to active by writing back a stale snapshot (M424). Mirrors kernel/memory.Manager.
	// The exported mutators take it; the unexported helpers (promoteWithReason,
	// quarantineLocked, maybeAuto*) assume it is already held.
	mu sync.Mutex
	// Auto-quarantine thresholds (SPEC-05 §5): an ACTIVE skill whose failure
	// record crosses BOTH a minimum failure COUNT and a failure RATE is pulled
	// from production automatically. Conservative-by-design so a mostly-successful
	// skill with a few failures is not yanked. aqMinFailures <= 0 disables it.
	aqMinFailures int
	aqFailureRate float64
	// autoShadow, when true, auto-advances a freshly-created draft to shadow once
	// it passes the deterministic ShadowTest (SPEC-05 §5.2 draft→shadow). Off by
	// default — staging is a step toward production, so the operator opts in.
	autoShadow bool
	// Auto-promote thresholds (SPEC-05 §5.2 shadow→active, "N successful real uses,
	// gated"): a SHADOW skill whose shadow-evaluation record crosses BOTH a minimum
	// WIN count and a win RATE is promoted to active automatically. Conservative by
	// design. apMinWins <= 0 disables it. Inert unless shadow evaluation (opt-in)
	// is feeding wins, so this can default on without surprising anyone.
	apMinWins int
	apWinRate float64
}

// DefaultAutoQuarantineMinFailures / Rate are the conservative defaults: a skill
// needs at least 3 failures AND a >=50% failure rate before it is auto-pulled.
const (
	DefaultAutoQuarantineMinFailures = 3
	DefaultAutoQuarantineRate        = 0.5
)

// DefaultAutoPromoteMinWins / Rate gate shadow→active auto-promotion: a shadow
// skill needs at least 3 shadow-evaluation wins AND a >=50% win rate before it is
// promoted to active. Mirrors the auto-quarantine thresholds.
const (
	DefaultAutoPromoteMinWins = 3
	DefaultAutoPromoteRate    = 0.5
)

// NewForge wires a Store to a bus. bus may be nil in store-only tests;
// production callers always pass the kernel bus so transitions are auditable.
func NewForge(store Store, b *bus.Bus) *Forge {
	return &Forge{
		store: store, bus: b, now: time.Now,
		aqMinFailures: DefaultAutoQuarantineMinFailures,
		aqFailureRate: DefaultAutoQuarantineRate,
		apMinWins:     DefaultAutoPromoteMinWins,
		apWinRate:     DefaultAutoPromoteRate,
	}
}

// SetAutoQuarantine tunes (or, with minFailures <= 0, disables) the failure-driven
// auto-quarantine. The daemon calls this from config; tests use it to assert the
// disabled path.
func (f *Forge) SetAutoQuarantine(minFailures int, rate float64) {
	f.aqMinFailures = minFailures
	f.aqFailureRate = rate
}

// SetAutoShadow enables or disables draft→shadow auto-staging (SPEC-05 §5.2).
// The daemon calls this from config; off by default.
func (f *Forge) SetAutoShadow(on bool) { f.autoShadow = on }

// SetBundles wires the on-disk bundle store so Create can materialize a skill's
// reference files and scripts (agentskills.io shape). The daemon injects it at
// startup; nil leaves skills body-only. Bundles returns the wired store (nil if
// unset) for callers that need to read resources directly.
func (f *Forge) SetBundles(b *BundleStore) { f.bundles = b }

// Bundles returns the wired bundle store (nil if none). The control plane uses
// it to serve a skill's resource list and file reads.
func (f *Forge) Bundles() *BundleStore { return f.bundles }

// SetAutoPromote tunes (or, with minWins <= 0, disables) shadow→active
// auto-promotion. The daemon calls this from config; tests use it to assert the
// disabled path.
func (f *Forge) SetAutoPromote(minWins int, rate float64) {
	f.apMinWins = minWins
	f.apWinRate = rate
}

// ErrIllegalTransition is returned when a lifecycle edge isn't allowed.
var ErrIllegalTransition = errors.New("skill: illegal lifecycle transition")

// ErrNotFound is returned by operations on an unknown skill id.
var ErrNotFound = errors.New("skill: not found")

// CreateSpec is the input to Create.
type CreateSpec struct {
	Name          string
	Description   string
	Triggers      []string
	Body          string
	ToolsRequired []string
	// Resources is an optional bundle of on-disk files (relative path → content)
	// that travel with the skill — reference docs and scripts (agentskills.io
	// shape). When non-empty and a bundle store is wired (SetBundles), Create
	// materializes them and records their manifest on the skill. Ignored when no
	// bundle store is set.
	Resources map[string][]byte
	// Agent scopes the skill to one roster agent (M932). Empty = shared pool.
	// Identical content re-proposed later refreshes the EXISTING record, so
	// ownership stays with the first author.
	Agent string
}

// Create authors a new draft skill and journals it. Content-addressing by
// (name, body) means an identical proposal dedupes onto the existing record
// (its recency is refreshed) rather than duplicating. A new body for an
// existing skill name is a NEW version: lineage is set to the active/shadow
// skills sharing that name (the versions this one evolves from, §4.3). Returns
// the skill and whether it was newly created.

// ----

func (f *Forge) Get(id string) (Skill, bool, error) { return f.store.Get(id) }

// List returns every skill, sorted deterministically (all states).
func (f *Forge) List() ([]Skill, error) { return f.store.All() }

// Count returns the number of active skills. Used by `agt status`.
func (f *Forge) Count() int { return f.store.Count() }

// HygieneReport summarizes skill health for the cleanup view (M858).
type HygieneReport struct {
	Total  int     `json:"total"`
	Active int     `json:"active"`
	Idle   []Skill `json:"idle"` // active skills that look unused (see Hygiene)
}

// Hygiene reports which ACTIVE skills look idle — never used, or not used since
// idleCutoffMs — so an operator can prune dead weight from the retrieval pool
// (M858). Brand-new skills (created after the cutoff) are NOT flagged, so a
// freshly promoted skill gets a fair chance before it's called idle. idleCutoffMs
// <= 0 flags every never-used active skill. Idle skills are sorted oldest-seen
// first (the deadest weight on top).
func (f *Forge) Hygiene(idleCutoffMs int64) (HygieneReport, error) {
	all, err := f.store.All()
	if err != nil {
		return HygieneReport{}, err
	}
	var rep HygieneReport
	rep.Total = len(all)
	for _, sk := range all {
		if sk.Status != StatusActive {
			continue
		}
		rep.Active++
		neverUsed := sk.Metrics.Uses == 0
		stale := idleCutoffMs > 0 && sk.Metrics.LastUsedMS > 0 && sk.Metrics.LastUsedMS < idleCutoffMs
		// Give new skills a grace period: only flag if it predates the cutoff.
		oldEnough := idleCutoffMs <= 0 || sk.CreatedMS < idleCutoffMs
		if (neverUsed || stale) && oldEnough {
			rep.Idle = append(rep.Idle, sk)
		}
	}
	sort.Slice(rep.Idle, func(i, j int) bool {
		return rep.Idle[i].Metrics.LastUsedMS < rep.Idle[j].Metrics.LastUsedMS
	})
	return rep, nil
}

// get is the internal "must exist" lookup.
func (f *Forge) get(id string) (Skill, bool, error) {
	sk, found, err := f.store.Get(id)
	if err != nil {
		return Skill{}, false, err
	}
	if !found {
		return Skill{}, false, ErrNotFound
	}
	return sk, true, nil
}

func (f *Forge) publish(kind event.Kind, corr string, payload any) *event.Event {
	if f.bus == nil {
		return nil
	}
	suffix := strings.TrimPrefix(string(kind), "skill.")
	ev, _ := f.bus.Publish(event.Spec{
		Subject:       "skill." + suffix,
		Kind:          kind,
		Actor:         "forge",
		CorrelationID: corr,
		Payload:       payload,
	})
	return ev
}

func normalizeList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, dup := seen[strings.ToLower(v)]; dup {
			continue
		}
		seen[strings.ToLower(v)] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// --- run-time context plumbing -------------------------------------------

type ctxKey int

const ctxKeyCorrelation ctxKey = iota

// WithCorrelation returns a child context carrying corr so Forge writes journal
// under the originating run.
func WithCorrelation(ctx context.Context, corr string) context.Context {
	return context.WithValue(ctx, ctxKeyCorrelation, corr)
}

// --- Forge proposal (the self-improvement trigger) ------------------------

// proposeSystem instructs the provider to extract a reusable skill from a
// completed task. Like memory distillation it must return strict JSON so
// parsing is deterministic; a non-JSON or empty response yields no skill (the
// best-effort contract — proposal never fails a task).
const proposeSystem = `You review a completed agent task and decide whether the approach is a reusable, named procedure worth saving as a SKILL for future tasks. ` +
	`Return ONLY a JSON object: {"skill":{"name":"kebab-case-name","description":"one line for retrieval","triggers":["tag1","tag2"],"body":"the steps/instructions","tools":["shell"]}} ` +
	`or {"skill":null} if the task was too trivial or one-off to generalize. ` +
	`The body should be concrete, reusable instructions — not a recap of this specific run.`

type proposeResult struct {
	Skill *struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Triggers    []string `json:"triggers"`
		Body        string   `json:"body"`
		Tools       []string `json:"tools"`
	} `json:"skill"`
}

// Propose runs one best-effort LLM call over a task transcript and, if the
// model judges the approach reusable, stores it as a DRAFT skill (operator must
// promote it — bad skills never reach production silently, §5.3). Returns the
// created draft ids. Errors are returned for the caller to journal, but the
// caller must never let a proposal error fail the underlying task.