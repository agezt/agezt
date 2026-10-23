// SPDX-License-Identifier: MIT

// Package skillapi is the contract layer's home for the skill rows: the
// lifecycle state of a reusable procedure, the record itself, the input that
// authors one, the rules that govern the lifecycle, and the ports through
// which the agent-facing tool reads them.
//
// The rows live here because the tool that publishes them is a PLUGIN, and a
// plugin may reach the contract and the platform layers and nothing else. The
// implementation that keeps them — the Forge, the store, the bundle store on
// disk — stays in the module (kernel/modules/skills/skill); the module keeps
// the names its own callers use as aliases, so there is one row and one
// spelling of it.
//
// The rules of the road:
//
//   - Rows, constants and pure functions only. No store, no file, no clock.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package skillapi

// Status is the lifecycle state of a skill (SPEC-05 §5.2).
type Status string

const (
	// StatusDraft is a freshly authored skill (by Forge or an operator);
	// never used in production.
	StatusDraft Status = "draft"
	// StatusShadow runs alongside real execution for evaluation; not yet in
	// the retrieval pool. (v1: a manual gate; auto shadow-testing deferred.)
	StatusShadow Status = "shadow"
	// StatusActive is in the retrieval/injection pool.
	StatusActive Status = "active"
	// StatusQuarantined was pulled from production by a regression or repeated
	// failure — operator-driven (`agt skill quarantine`) or automatic once an
	// active skill crosses the failure threshold (M387, Forge.RecordOutcome).
	// Quarantined skills are excluded from the retrieval pool (retrieve.go).
	StatusQuarantined Status = "quarantined"
	// StatusArchived is retired; retained for lineage/audit.
	StatusArchived Status = "archived"
)

// DefaultVersion is the semver a freshly proposed skill starts at.
const DefaultVersion = "0.1.0"

// legalTransitions encodes the state machine (SPEC-05 §5.2). promote walks
// draft→shadow→active; quarantine pulls an active/shadow skill; archive retires
// a draft/shadow/quarantined skill; revert is handled specially by Forge
// (it appends a reversal rather than being a plain edge). A skill can always be
// archived from any non-archived state for operator cleanup.
var legalTransitions = map[Status]map[Status]struct{}{
	StatusDraft:       {StatusShadow: {}, StatusArchived: {}},
	StatusShadow:      {StatusActive: {}, StatusQuarantined: {}, StatusArchived: {}},
	StatusActive:      {StatusQuarantined: {}, StatusArchived: {}},
	StatusQuarantined: {StatusActive: {}, StatusArchived: {}},
	StatusArchived:    {},
}

// CanTransition reports whether from→to is a legal lifecycle edge.
func CanTransition(from, to Status) bool {
	dests, ok := legalTransitions[from]
	if !ok {
		return false
	}
	_, ok = dests[to]
	return ok
}

// ValidStatus reports whether st is one of the persisted lifecycle states.
func ValidStatus(st Status) bool {
	switch st {
	case StatusDraft, StatusShadow, StatusActive, StatusQuarantined, StatusArchived:
		return true
	default:
		return false
	}
}

// PromoteTarget returns the next status `promote` advances to from the given
// state (draft→shadow→active), and whether a promotion is defined there.
func PromoteTarget(from Status) (Status, bool) {
	switch from {
	case StatusDraft:
		return StatusShadow, true
	case StatusShadow:
		return StatusActive, true
	case StatusQuarantined:
		return StatusActive, true // un-quarantine back into production
	default:
		return from, false
	}
}

// Metrics tracks how a skill has performed (SPEC-05 §4.1).
type Metrics struct {
	Uses       int   `json:"uses"`
	Successes  int   `json:"successes"`
	Failures   int   `json:"failures"`
	LastUsedMS int64 `json:"last_used_ms,omitempty"`
	// ShadowEvals / ShadowWins track shadow-evaluation outcomes (M400, SPEC-05
	// §5.2) — how many times a shadow skill was judged against a completed run,
	// and how many of those judged it would have helped. Kept separate from
	// Successes/Failures (which are real production outcomes) so the shadow→active
	// gate reads only its own evidence.
	ShadowEvals int `json:"shadow_evals,omitempty"`
	ShadowWins  int `json:"shadow_wins,omitempty"`
}

// Skill is one reusable, named procedure. JSON tags are stable so the on-disk
// file and the CLI/`--json` shape stay compatible across releases.
type Skill struct {
	// ID is content-addressed: hex(BLAKE3("skill" \0 name \0 body)).
	ID string `json:"id"`
	// Name identifies the skill ("diagnose-failing-ci").
	Name string `json:"name"`
	// Description is the retrieval/activation matching key (§4.2).
	Description string `json:"description"`
	// Triggers are tags/conditions hinting when the skill is relevant.
	Triggers []string `json:"triggers,omitempty"`
	// Body is the instructions/steps injected into the planning context.
	Body string `json:"body"`
	// ToolsRequired lists tools the skill expects to be available.
	ToolsRequired []string `json:"tools_required,omitempty"`
	// Resources lists the relative paths of the skill's on-disk bundle —
	// reference files and scripts that travel with it (agentskills.io shape,
	// SPEC-13). Empty for a plain body-only skill. The files live under the
	// BundleStore; this is the manifest the retrieval layer, the CLI, and the
	// agent's skill tool use to discover them.
	Resources []string `json:"resources,omitempty"`
	// Agent is the owning roster slug (M932): a skill an agent learned in its
	// own runs belongs to that agent and is retrieved only when IT acts — the
	// same private-by-default wall per-agent memory draws (M915). Empty means
	// shared: visible to every agent and the default daemon identity.
	Agent string `json:"agent,omitempty"`
	// Version is semver; a new body is a new version (§4.3).
	Version string `json:"version"`
	// Lineage is the ids this skill evolved from (parent-first).
	Lineage []string `json:"lineage,omitempty"`
	// Status is the lifecycle state.
	Status Status `json:"status"`
	// Metrics record usage/outcomes.
	Metrics Metrics `json:"metrics"`
	// SourceEvent is the journal event id that produced this record —
	// provenance for `agt why`.
	SourceEvent string `json:"source_event,omitempty"`
	// CreatedMS / LastSeenMS drive recency in ranking and history ordering.
	CreatedMS  int64 `json:"created_ms"`
	LastSeenMS int64 `json:"last_seen_ms"`
}

// Active reports whether the skill is in the retrieval pool.
func (s Skill) Active() bool { return s.Status == StatusActive }

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

// Forge is the port through which the agent's skill tool drives the skill
// state machine: author, promote, quarantine, read. The implementation — the
// journaled, reversible Forge in the skills module — satisfies it; the tool
// never names the implementation, because it is a plugin.
type Forge interface {
	// Create authors a new draft skill and journals it. Returns the skill and
	// whether it was newly created.
	Create(corr string, spec CreateSpec) (Skill, bool, error)
	// Promote advances a skill toward the retrieval pool (draft→shadow→active)
	// and returns the status it landed on.
	Promote(corr, id string) (Status, error)
	// Quarantine pulls a skill out of production.
	Quarantine(corr, id, reason string) error
	// Get returns the skill at id; the bool is false if absent.
	Get(id string) (Skill, bool, error)
	// List returns every skill.
	List() ([]Skill, error)
}

// BundleStore is the port for a skill's on-disk bundle — the reference files
// and scripts that travel with it (agentskills.io shape, SPEC-13).
type BundleStore interface {
	// List returns the bundle-relative paths of a skill's resources.
	List(name string) ([]string, error)
	// Read returns one resource's bytes.
	Read(name, rel string) ([]byte, error)
	// Dir returns the directory a skill's bundle lives in — the tool reports
	// it so the agent can run a bundled script from there.
	Dir(name string) string
}
