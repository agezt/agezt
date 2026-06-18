// SPDX-License-Identifier: MIT

// Package skill implements "Forge v1" (SPEC-05 §4–5): the agent learns
// reusable, named procedures ("skills") from what it does, and those skills
// are governed through a journaled, reversible state machine instead of
// straight-to-active markdown. This is the "Curator-killer" — Agezt matches
// Hermes's learning loop and beats it on auditability and reversibility: every
// skill mutation is a content-addressed, hash-chained event, so you can ask
// why a skill exists, when it was promoted, and undo it (`agt skill revert`).
//
// Two layers, mirroring kernel/memory and kernel/worldmodel:
//
//   - Store (this file) is a pure, file-backed record store — no bus, no
//     journaling — owning content-addressing and the legal-transition table.
//     A CobaltDB-class engine can replace it behind the Store interface later.
//   - Forge (forge.go) wraps a Store with the kernel bus so every transition
//     (create/promote/quarantine/revert/activate) is a durable-before-publish
//     event carrying the run's correlation_id (SPEC-05 §5.3).
//
// Content-addressing is versioning: a skill's id is BLAKE3(name\0body), so
// editing the body yields a NEW record (a new version) with Lineage pointing
// at its parent — never a destructive edit (§4.3). Status and metrics are
// mutable metadata on the record; lifecycle transitions don't change the id.
//
// Concurrency: a single Store instance is safe for concurrent use.
package skill

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"lukechampine.com/blake3"
)

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

var validStatuses = map[Status]struct{}{
	StatusDraft: {}, StatusShadow: {}, StatusActive: {},
	StatusQuarantined: {}, StatusArchived: {},
}

// ValidStatus reports whether s is one of the known lifecycle states.
func ValidStatus(s Status) bool { _, ok := validStatuses[s]; return ok }

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
	// BundleStore (kernel/skill/bundle.go); this is the manifest the retrieval
	// layer, the CLI, and the agent's skill tool use to discover them.
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

// ContentID computes the content-addressed id for a (name, body) pair. The
// "skill" domain prefix + NUL separators keep ids disjoint from other content
// addresses and avoid concatenation collisions. Name is normalized (lower/trim)
// so the same skill under cosmetic name casing addresses identically; the body
// is hashed verbatim (whitespace is meaningful in instructions).
func ContentID(name, body string) string {
	h := blake3.New(32, nil)
	h.Write([]byte("skill"))
	h.Write([]byte{0})
	h.Write([]byte(strings.ToLower(strings.TrimSpace(name))))
	h.Write([]byte{0})
	h.Write([]byte(body))
	return hex.EncodeToString(h.Sum(nil))
}

// Store is the pure skill store. Implementations persist skills by id and must
// be safe for concurrent use.
type Store interface {
	// Put upserts s by its ID.
	Put(s Skill) error
	// Get returns the skill at id; the bool is false if absent.
	Get(id string) (Skill, bool, error)
	// All returns every skill, sorted deterministically by CreatedMS then ID.
	All() ([]Skill, error)
	// Count returns the number of active skills (the retrieval-pool size).
	Count() int
	// Close releases any held resources.
	Close() error
}

// ErrEmptyBody is returned when a skill is written with no body.
var ErrEmptyBody = errors.New("skill: empty body")

// FileStore is the file-backed Store. All skills live in a single
// <dir>/skills.json object keyed by id, snapshotted atomically on every
// mutation (write-temp + rename). Mirrors kernel/worldmodel's FileStore.
type FileStore struct {
	path string

	mu   sync.RWMutex
	data map[string]Skill
}

// Open opens (or creates) a FileStore under dir, loading <dir>/skills.json if
// present. The directory is created if absent.
func Open(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("skill: mkdir %s: %w", dir, err)
	}
	s := &FileStore{
		path: filepath.Join(dir, "skills.json"),
		data: make(map[string]Skill),
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("skill: read %s: %w", s.path, err)
	}
	if len(raw) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("skill: parse %s: %w", s.path, err)
	}
	if s.data == nil {
		s.data = make(map[string]Skill)
	}
	return s, nil
}

// Put implements Store.
func (s *FileStore) Put(sk Skill) error {
	if sk.ID == "" {
		return errors.New("skill: record id required")
	}
	if strings.TrimSpace(sk.Body) == "" {
		return ErrEmptyBody
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[sk.ID] = sk
	return s.snapshotLocked()
}

// Get implements Store.
func (s *FileStore) Get(id string) (Skill, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sk, ok := s.data[id]
	return sk, ok, nil
}

// All implements Store. Sorted by CreatedMS then ID so two consecutive calls
// produce identical output (deterministic CLI + snapshot tests).
func (s *FileStore) All() ([]Skill, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Skill, 0, len(s.data))
	for _, sk := range s.data {
		out = append(out, sk)
	}
	sortSkills(out)
	return out, nil
}

// Count implements Store — the number of active skills.
func (s *FileStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, sk := range s.data {
		if sk.Active() {
			n++
		}
	}
	return n
}

// Close implements Store. Mutations persist synchronously, so this is a no-op.
func (s *FileStore) Close() error { return nil }

// snapshotLocked writes the whole skill map atomically. Caller holds s.mu.
func (s *FileStore) snapshotLocked() error {
	body, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("skill: marshal: %w", err)
	}
	return atomicWrite(s.path, body)
}

func sortSkills(sk []Skill) {
	sort.Slice(sk, func(i, j int) bool {
		if sk[i].CreatedMS != sk[j].CreatedMS {
			return sk[i].CreatedMS < sk[j].CreatedMS
		}
		return sk[i].ID < sk[j].ID
	})
}

// atomicWrite writes data to a temp file and renames it over the target.
// os.Rename replaces atomically on POSIX and on Windows (MoveFileEx). Mirrors
// the kernel/state, kernel/memory and kernel/worldmodel helpers (kept local to
// avoid coupling the packages).
func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("skill: open temp %s: %w", tmp, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("skill: write temp: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("skill: fsync temp: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("skill: close temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("skill: rename %s: %w", path, err)
	}
	return nil
}
