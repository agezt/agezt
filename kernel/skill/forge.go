// SPDX-License-Identifier: MIT

package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
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
func (f *Forge) Create(corr string, spec CreateSpec) (Skill, bool, error) {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return Skill{}, false, errors.New("skill: empty name")
	}
	if strings.TrimSpace(spec.Body) == "" {
		return Skill{}, false, ErrEmptyBody
	}
	nowMS := f.now().UnixMilli()
	id := ContentID(name, spec.Body)

	f.mu.Lock()
	defer f.mu.Unlock()

	// Materialize any bundle first: resources are keyed by name, so they attach
	// to both a fresh skill and a refreshed-existing one. A bundle-write failure
	// fails the create (a half-installed skill that points at missing files is
	// worse than no skill).
	resources, err := f.writeBundle(name, spec.Resources)
	if err != nil {
		return Skill{}, false, err
	}

	if existing, found, err := f.store.Get(id); err != nil {
		return Skill{}, false, err
	} else if found {
		existing.LastSeenMS = nowMS
		if resources != nil {
			existing.Resources = resources
		}
		if err := f.store.Put(existing); err != nil {
			return Skill{}, false, err
		}
		return existing, false, nil
	}

	sk := Skill{
		ID:            id,
		Name:          name,
		Description:   spec.Description,
		Triggers:      normalizeList(spec.Triggers),
		Body:          spec.Body,
		ToolsRequired: normalizeList(spec.ToolsRequired),
		Resources:     resources,
		Agent:         strings.TrimSpace(spec.Agent),
		Version:       DefaultVersion,
		Lineage:       f.lineageFor(name),
		Status:        StatusDraft,
		CreatedMS:     nowMS,
		LastSeenMS:    nowMS,
	}
	ev := f.publish(event.KindSkillCreated, corr, map[string]any{
		"action": "create", "id": id, "name": name, "status": string(StatusDraft), "agent": sk.Agent,
	})
	if ev != nil {
		sk.SourceEvent = ev.ID
	}
	if err := f.store.Put(sk); err != nil {
		return Skill{}, false, err
	}
	// Auto-stage a well-formed draft to shadow (SPEC-05 §5.2), when enabled. The
	// returned skill reflects the post-staging status, so callers see "shadow".
	f.maybeAutoShadow(corr, sk)
	if cur, found, gerr := f.store.Get(sk.ID); gerr == nil && found {
		sk = cur
	}
	return sk, true, nil
}

// maybeAutoShadow advances a freshly-created draft to shadow when auto-staging is
// enabled and the draft passes the deterministic ShadowTest (SPEC-05 §5.2). Only
// drafts are affected; the promotion is journaled with the gate reason and is
// reversible via the normal lifecycle. Best-effort: a staging failure leaves the
// skill a draft (no worse than auto-staging being off).
func (f *Forge) maybeAutoShadow(corr string, sk Skill) {
	if !f.autoShadow || sk.Status != StatusDraft {
		return
	}
	if ok, _ := ShadowTest(sk); !ok {
		return
	}
	_, _ = f.promoteWithReason(corr, sk.ID, "auto-shadow: shadow-test passed")
}

// writeBundle materializes a skill's resource bundle when both resources and a
// bundle store are present. It returns the manifest (sorted relative paths) to
// record on the skill, or nil when there is nothing to attach (no resources, or
// no bundle store wired). Caller holds f.mu.
func (f *Forge) writeBundle(name string, resources map[string][]byte) ([]string, error) {
	if len(resources) == 0 || f.bundles == nil {
		return nil, nil
	}
	return f.bundles.Write(name, resources)
}

// lineageFor returns the ids of non-archived skills sharing name — the
// versions a new body evolves from.
func (f *Forge) lineageFor(name string) []string {
	all, err := f.store.All()
	if err != nil {
		return nil
	}
	folded := strings.ToLower(strings.TrimSpace(name))
	var out []string
	for _, sk := range all {
		if sk.Status == StatusArchived {
			continue
		}
		if strings.ToLower(strings.TrimSpace(sk.Name)) == folded {
			out = append(out, sk.ID)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Promote advances a skill along draft→shadow→active (or un-quarantines back
// to active), journaling skill.promoted. Returns the new status.
func (f *Forge) Promote(corr, id string) (Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.promoteWithReason(corr, id, "")
}

// promoteWithReason is Promote with an optional reason recorded on the event —
// used by auto-staging (M399) to mark the gate that advanced the skill. An empty
// reason omits the field, keeping a manual promote's payload unchanged.
func (f *Forge) promoteWithReason(corr, id, reason string) (Status, error) {
	sk, _, err := f.get(id)
	if err != nil {
		return "", err
	}
	target, ok := PromoteTarget(sk.Status)
	if !ok {
		return sk.Status, fmt.Errorf("%w: %s cannot be promoted", ErrIllegalTransition, sk.Status)
	}
	if !CanTransition(sk.Status, target) {
		return sk.Status, fmt.Errorf("%w: %s→%s", ErrIllegalTransition, sk.Status, target)
	}
	from := sk.Status
	sk.Status = target
	sk.LastSeenMS = f.now().UnixMilli()
	if err := f.store.Put(sk); err != nil {
		return "", err
	}
	payload := map[string]any{
		"id": id, "name": sk.Name, "from": string(from), "to": string(target),
	}
	if reason != "" {
		payload["reason"] = reason
	}
	f.publish(event.KindSkillPromoted, corr, payload)
	return target, nil
}

// Reassign changes a skill's owning agent (M942), the ownership analogue of the
// per-agent memory promote valve (M915). newAgent == "" shares the skill with
// the whole pool (clears the private-to-one-agent wall); a non-empty slug makes
// it private to that agent. It is a no-op (found=true) when the owner is already
// newAgent. Emits skill.shared when sharing, skill.reassigned otherwise. The
// caller (controlplane) validates that a non-empty target slug exists in the
// roster before calling. Ownership is orthogonal to the draft→active lifecycle,
// so Status is untouched and any skill may be reassigned.
func (f *Forge) Reassign(corr, id, newAgent string) (Skill, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sk, found, err := f.get(id)
	if err != nil {
		return Skill{}, found, err
	}
	if sk.Agent == newAgent {
		return sk, true, nil // already there — no event, no churn
	}
	fromAgent := sk.Agent
	sk.Agent = newAgent
	sk.LastSeenMS = f.now().UnixMilli()
	if err := f.store.Put(sk); err != nil {
		return Skill{}, true, err
	}
	if newAgent == "" {
		f.publish(event.KindSkillShared, corr, map[string]any{
			"id": id, "name": sk.Name, "from_agent": fromAgent,
		})
	} else {
		f.publish(event.KindSkillReassigned, corr, map[string]any{
			"id": id, "name": sk.Name, "from_agent": fromAgent, "to_agent": newAgent,
		})
	}
	return sk, true, nil
}

// Quarantine pulls an active or shadow skill out of production, journaling
// skill.quarantined with the reason.
func (f *Forge) Quarantine(corr, id, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.quarantineLocked(corr, id, reason)
}

// quarantineLocked is Quarantine's body; the caller must hold f.mu. Used directly by
// maybeAutoQuarantine (already under the lock via RecordOutcome) to avoid re-locking.
func (f *Forge) quarantineLocked(corr, id, reason string) error {
	sk, _, err := f.get(id)
	if err != nil {
		return err
	}
	if !CanTransition(sk.Status, StatusQuarantined) {
		return fmt.Errorf("%w: %s→quarantined", ErrIllegalTransition, sk.Status)
	}
	from := sk.Status
	sk.Status = StatusQuarantined
	sk.LastSeenMS = f.now().UnixMilli()
	if err := f.store.Put(sk); err != nil {
		return err
	}
	f.publish(event.KindSkillQuarantined, corr, map[string]any{
		"id": id, "name": sk.Name, "from": string(from), "reason": reason,
	})
	return nil
}

// Revert appends a reversal (SPEC-05 §5.3): it archives the target skill and
// re-activates its most recent non-archived lineage parent if there is one, so
// reverting a bad new version restores the previous good one. History is never
// edited — the prior states remain in the journal; this just moves the records
// forward and emits skill.reverted. Returns the id of the restored parent (or
// "" if none).
func (f *Forge) Revert(corr, id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sk, _, err := f.get(id)
	if err != nil {
		return "", err
	}
	if sk.Status == StatusArchived {
		return "", fmt.Errorf("%w: already archived", ErrIllegalTransition)
	}
	nowMS := f.now().UnixMilli()
	sk.Status = StatusArchived
	sk.LastSeenMS = nowMS
	if err := f.store.Put(sk); err != nil {
		return "", err
	}

	restored := ""
	for i := len(sk.Lineage) - 1; i >= 0; i-- {
		parent, found, err := f.store.Get(sk.Lineage[i])
		if err != nil {
			return "", err
		}
		if !found || parent.Status == StatusArchived {
			continue
		}
		// Respect the state machine: only restore a parent that may legally become
		// active (already active, or shadow/quarantined → active). A draft parent
		// must NOT be force-activated — that would skip the shadow gate (M424). Try
		// the next-older lineage parent instead.
		if parent.Status != StatusActive && !CanTransition(parent.Status, StatusActive) {
			continue
		}
		parent.Status = StatusActive
		parent.LastSeenMS = nowMS
		if err := f.store.Put(parent); err != nil {
			return "", err
		}
		restored = parent.ID
		break
	}
	f.publish(event.KindSkillReverted, corr, map[string]any{
		"id": id, "name": sk.Name, "restored": restored,
	})
	return restored, nil
}

// visibleTo reports whether an acting agent may retrieve a skill (M932):
// shared skills (no owner) are everyone's; a private skill is its owner's
// alone — the default persona (empty slug) sees only the shared pool. The
// same scope wall per-agent memory draws (M915).
func visibleTo(sk Skill, agentSlug string) bool {
	return sk.Agent == "" || sk.Agent == agentSlug
}

// Activate ranks active skills against intent and journals skill.activated
// (under corr) when anything matched, bumping each matched skill's use metrics
// — so `agt why` shows which skills shaped a run. Returns the ranked results.
// Equivalent to ActivateFor with no acting agent (shared pool only).
func (f *Forge) Activate(corr, intent string, limit int) ([]Scored, error) {
	return f.ActivateFor(corr, "", intent, limit)
}

// ActivateFor is Activate scoped to the acting agent (M932): the retrieval
// pool is the shared skills plus the agent's own private ones, so an agent
// plans with what IT learned without leaking another agent's procedures.
func (f *Forge) ActivateFor(corr, agentSlug, intent string, limit int) ([]Scored, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all, err := f.store.All()
	if err != nil {
		return nil, err
	}
	pool := all[:0:0]
	for _, sk := range all {
		if visibleTo(sk, agentSlug) {
			pool = append(pool, sk)
		}
	}
	nowMS := f.now().UnixMilli()
	hits := Retrieve(pool, intent, limit, nowMS)
	if len(hits) == 0 {
		return hits, nil
	}
	ids := make([]string, 0, len(hits))
	for _, h := range hits {
		ids = append(ids, h.Skill.ID)
		sk := h.Skill
		sk.Metrics.Uses++
		sk.Metrics.LastUsedMS = nowMS
		_ = f.store.Put(sk)
	}
	f.publish(event.KindSkillActivated, corr, map[string]any{
		"intent": intent, "matched": len(hits), "ids": ids,
	})
	return hits, nil
}

// RecordOutcome bumps success/failure metrics for the given skill ids and, on a
// failure, auto-quarantines a skill whose record has crossed the threshold
// (SPEC-05 §5). The runtime calls it after a run with the skills that run
// activated; corr ties any resulting skill.quarantined event back to that run.
func (f *Forge) RecordOutcome(corr string, ids []string, success bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range ids {
		sk, found, err := f.store.Get(id)
		if err != nil || !found {
			continue
		}
		if success {
			sk.Metrics.Successes++
		} else {
			sk.Metrics.Failures++
		}
		if err := f.store.Put(sk); err != nil {
			continue
		}
		if !success {
			f.maybeAutoQuarantine(corr, sk)
		}
	}
}

// maybeAutoQuarantine pulls an ACTIVE skill from production when its failure
// record crosses the configured threshold (SPEC-05 §5: "pulled from production by
// a regression or repeated failure"). Requires BOTH a minimum failure count and a
// failure rate, so a few failures amid many successes don't yank a good skill.
// Only ACTIVE skills are affected (shadow skills are still under evaluation); the
// action is journaled and reversible (`agt skill promote` re-activates).
func (f *Forge) maybeAutoQuarantine(corr string, sk Skill) {
	if f.aqMinFailures <= 0 || sk.Status != StatusActive {
		return
	}
	total := sk.Metrics.Successes + sk.Metrics.Failures
	if total == 0 || sk.Metrics.Failures < f.aqMinFailures {
		return
	}
	rate := float64(sk.Metrics.Failures) / float64(total)
	if rate < f.aqFailureRate {
		return
	}
	reason := fmt.Sprintf("auto-quarantine: %d/%d runs failed (%.0f%%)", sk.Metrics.Failures, total, rate*100)
	_ = f.quarantineLocked(corr, sk.ID, reason) // caller (RecordOutcome) holds f.mu
}

// shadowJudgeSystem instructs the model to decide whether a shadow skill would
// have helped a just-completed run (SPEC-05 §5.2). The verdict is a single word
// so it parses robustly across providers; "be conservative" biases toward NO.
const shadowJudgeSystem = `You evaluate whether a candidate "skill" (a reusable procedure) would have helped an agent complete a task it just finished.
You are given the task intent, what actually happened, and the skill's instructions.
Reply with exactly one word: YES if the skill's guidance would plausibly have improved or sped up the outcome, otherwise NO. Be conservative — reply NO if unsure.`

// parseShadowVerdict reads the model's YES/NO leniently: helped only when the
// first meaningful word is affirmative. Anything ambiguous or non-conforming
// (e.g. the offline mock's canned text) defaults to false — conservative, so a
// skill is never credited toward promotion on a vague answer.
func parseShadowVerdict(text string) bool {
	for _, line := range strings.Split(strings.ToLower(text), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		word := line
		if i := strings.IndexFunc(line, func(r rune) bool {
			return r == ' ' || r == '.' || r == ',' || r == ':' || r == '!' || r == ';'
		}); i >= 0 {
			word = line[:i]
		}
		return word == "yes" || word == "true"
	}
	return false
}

// ShadowEvaluate judges the shadow skills relevant to a just-completed run and
// records the verdicts (SPEC-05 §5.2: shadow "runs alongside real execution
// without affecting outcomes ... compared to what actually happened"). It runs
// NO tools — the shadow skill is never executed, so evaluation cannot affect
// outcomes; the model judges whether the skill WOULD have helped. Best-effort:
// a provider error on one candidate is skipped, not fatal. limit bounds how many
// shadow candidates are judged per run (cost control).
func (f *Forge) ShadowEvaluate(ctx context.Context, corr string, provider agent.Provider, model, intent, outcome string, limit int) error {
	if provider == nil {
		return errors.New("skill: shadow eval requires a provider")
	}
	all, err := f.store.All()
	if err != nil {
		return err
	}
	for _, c := range RetrieveShadow(all, intent, limit, f.now().UnixMilli()) {
		user := fmt.Sprintf("Task intent:\n%s\n\nWhat actually happened:\n%s\n\nCandidate skill %q:\n%s",
			intent, outcome, c.Skill.Name, c.Skill.Body)
		resp, cerr := provider.Complete(ctx, agent.CompletionRequest{
			Model:         model,
			System:        shadowJudgeSystem,
			Messages:      []agent.Message{{Role: agent.RoleUser, Content: user}},
			CorrelationID: corr,
			TaskType:      "shadow-eval",
			MaxTokens:     16,
		})
		if cerr != nil {
			continue
		}
		f.RecordShadowOutcome(corr, c.Skill.ID, parseShadowVerdict(resp.Message.Content))
	}
	return nil
}

// RecordShadowOutcome bumps a shadow skill's evaluation counters and journals
// skill.shadow_evaluated under the run's correlation. Only shadow skills are
// affected. The shadow→active auto-promotion gate (M401) reads these counters.
func (f *Forge) RecordShadowOutcome(corr, id string, helped bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sk, found, err := f.store.Get(id)
	if err != nil || !found || sk.Status != StatusShadow {
		return
	}
	sk.Metrics.ShadowEvals++
	if helped {
		sk.Metrics.ShadowWins++
	}
	if err := f.store.Put(sk); err != nil {
		return
	}
	f.publish(event.KindSkillShadowEval, corr, map[string]any{
		"id": id, "name": sk.Name, "helped": helped,
		"evals": sk.Metrics.ShadowEvals, "wins": sk.Metrics.ShadowWins,
	})
	if helped {
		f.maybeAutoPromote(corr, sk)
	}
}

// maybeAutoPromote promotes a SHADOW skill to active when its shadow-evaluation
// record crosses the configured win threshold (SPEC-05 §5.2 shadow→active, "N
// successful real uses, gated"). Requires BOTH a minimum win count and a win
// rate, so a shadow skill judged unhelpful as often as not is not promoted. Only
// shadow skills are affected; the promotion is journaled with the gate reason and
// is reversible (auto-quarantine can later pull it if it regresses).
func (f *Forge) maybeAutoPromote(corr string, sk Skill) {
	if f.apMinWins <= 0 || sk.Status != StatusShadow {
		return
	}
	if sk.Metrics.ShadowEvals == 0 || sk.Metrics.ShadowWins < f.apMinWins {
		return
	}
	rate := float64(sk.Metrics.ShadowWins) / float64(sk.Metrics.ShadowEvals)
	if rate < f.apWinRate {
		return
	}
	reason := fmt.Sprintf("auto-promote: %d/%d shadow evals judged helpful (%.0f%%)",
		sk.Metrics.ShadowWins, sk.Metrics.ShadowEvals, rate*100)
	_, _ = f.promoteWithReason(corr, sk.ID, reason)
}

// Get returns a single skill by id.
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

// CorrelationFrom extracts the correlation id set by WithCorrelation.
func CorrelationFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyCorrelation).(string); ok {
		return v
	}
	return ""
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
func (f *Forge) Propose(ctx context.Context, corr string, provider agent.Provider, model, intent, transcript string) ([]string, error) {
	if provider == nil {
		return nil, errors.New("skill: propose requires a provider")
	}
	user := fmt.Sprintf("Task intent:\n%s\n\nWhat happened:\n%s", intent, transcript)
	resp, err := provider.Complete(ctx, agent.CompletionRequest{
		Model:    model,
		System:   proposeSystem,
		Messages: []agent.Message{{Role: agent.RoleUser, Content: user}},
		TaskType: "forge",
	})
	if err != nil {
		return nil, fmt.Errorf("skill: propose completion: %w", err)
	}
	parsed, ok := parsePropose(resp.Message.Content)
	if !ok || parsed.Skill == nil {
		// Non-JSON (e.g. the mock provider) or an explicit decline → nothing
		// to author. Not an error; proposal is opportunistic.
		return nil, nil
	}
	if strings.TrimSpace(parsed.Skill.Body) == "" || strings.TrimSpace(parsed.Skill.Name) == "" {
		return nil, nil
	}
	// A skill learned while a named agent acted belongs to that agent (M932):
	// private-by-default self-learning, mirroring per-agent memory. Default-
	// persona runs (no agent on ctx) author into the shared pool as before.
	sk, _, err := f.Create(corr, CreateSpec{
		Name:          parsed.Skill.Name,
		Description:   parsed.Skill.Description,
		Triggers:      parsed.Skill.Triggers,
		Body:          parsed.Skill.Body,
		ToolsRequired: parsed.Skill.Tools,
		Agent:         agent.AgentFromContext(ctx),
	})
	if err != nil {
		return nil, err
	}
	return []string{sk.ID}, nil
}

// parsePropose extracts the JSON object from a model response, tolerating
// surrounding prose or markdown fences by scanning for the outermost braces.
func parsePropose(s string) (proposeResult, bool) {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return proposeResult{}, false
	}
	var r proposeResult
	if err := json.Unmarshal([]byte(s[start:end+1]), &r); err != nil {
		return proposeResult{}, false
	}
	return r, true
}
