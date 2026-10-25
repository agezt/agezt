// SPDX-License-Identifier: MIT

// Package worldmodelapi is the contract layer's home for the world-model rows:
// the entity, the relation, the scored hit, the neighbour, the input that
// writes one, and the options the reflection loop decays with.
//
// The rows live here because more than one layer has to speak them and none
// of them may name the module's implementation: the kernel resolves entities
// for a run, the reflection engine decays them, the app layer curates them,
// and the daemon owns the file they live in. The implementation — the
// file-backed graph and its store — stays in the module
// (kernel/modules/knowledge/worldmodel); the module keeps the names its own
// callers use as aliases, so there is one row and one spelling of it.
//
// The rules of the road:
//
//   - Rows, constants and pure functions only. No store, no file, no clock.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
//
// The content-addressing helpers (EntityID, RelationID) are NOT here: they
// hash with BLAKE3, and the contract layer may not reach for a third-party
// module. Nothing above the module needs them — the module computes the ids
// itself.
package worldmodelapi

import (
	"errors"
	"strings"
)

// ErrEmptyName is returned when an entity is written with no name.
//
// It lives in the contract layer because the layers above the module compare
// against it, and a module may not be named by the layers above it (W3.54).
var ErrEmptyName = errors.New("worldmodel: empty entity name")

// Kind classifies an entity (SPEC-05 §3.2). It is an open string — the set
// below is the validated, well-known vocabulary, but an unknown kind is
// accepted (NormalizeKind keeps it verbatim) so the graph never refuses to
// learn something just because it is a new category.
type Kind string

const (
	KindProject Kind = "project"
	KindRepo    Kind = "repo"
	KindPerson  Kind = "person"
	KindOrg     Kind = "org"
	KindAccount Kind = "account"
	KindDevice  Kind = "device"
	KindChannel Kind = "channel"
	KindTopic   Kind = "topic"
	KindTask    Kind = "task"
)

// DefaultKind is used when an entity is written with no kind.
const DefaultKind = KindTopic

// NormalizeKind lowercases/trims a kind and falls back to DefaultKind when the
// input is empty. Unknown-but-non-empty kinds are kept verbatim (permissive):
// the graph records the operator's vocabulary rather than rejecting it.
func NormalizeKind(k Kind) Kind {
	s := Kind(strings.ToLower(strings.TrimSpace(string(k))))
	if s == "" {
		return DefaultKind
	}
	return s
}

// Verb classifies a relation (SPEC-05 §3.2). Like Kind it is an open string
// with a well-known set; NormalizeVerb defaults the empty verb to VerbRelatesTo.
type Verb string

const (
	VerbOwns        Verb = "owns"
	VerbDependsOn   Verb = "depends_on"
	VerbMemberOf    Verb = "member_of"
	VerbPrefers     Verb = "prefers"
	VerbRelatesTo   Verb = "relates_to"
	VerbAssignedTo  Verb = "assigned_to"
	VerbDerivedFrom Verb = "derived_from"
)

// DefaultVerb is used when a relation is written with no verb.
const DefaultVerb = VerbRelatesTo

// NormalizeVerb lowercases/trims a verb and falls back to DefaultVerb when
// empty. Unknown-but-non-empty verbs are kept verbatim.
func NormalizeVerb(v Verb) Verb {
	s := Verb(strings.ToLower(strings.TrimSpace(string(v))))
	if s == "" {
		return DefaultVerb
	}
	return s
}

// Entity is a node in the world model. The JSON tags are stable so the
// on-disk file and the CLI/`--json` shape stay compatible across releases;
// field order is not load-bearing (entities are not part of the event hash
// chain — their provenance events are).
type Entity struct {
	// ID is content-addressed: hex(BLAKE3("entity" \0 kind \0 name)).
	ID string `json:"id"`
	// Kind classifies the entity (project, repo, person, ...).
	Kind Kind `json:"kind"`
	// Name is the canonical display name (also the primary resolve key).
	Name string `json:"name"`
	// Aliases are alternative phrases that resolve to this entity
	// ("the portfolio", "the repos"). Lower-cased and deduped on write.
	Aliases []string `json:"aliases,omitempty"`
	// Attrs hold preferences/habits/constraints ("brief":"morning,terse").
	Attrs map[string]string `json:"attrs,omitempty"`
	// Weight is an "active-ness" score; reinforced on reference, decayed by
	// the reflection loop later. Drives resolve ranking and salience.
	Weight float64 `json:"weight"`
	// SourceEvent is the journal event id that produced this entity —
	// provenance for `agt why`.
	SourceEvent string `json:"source_event,omitempty"`
	// CreatedMS / LastSeenMS drive recency in ranking and decay.
	CreatedMS  int64 `json:"created_ms"`
	LastSeenMS int64 `json:"last_seen_ms"`
	// SupersededBy points at a newer entity when this one was replaced.
	SupersededBy string `json:"superseded_by,omitempty"`
	// Tombstoned marks a soft-forgotten entity: excluded from resolve but
	// retained on disk and in the journal (reversibility).
	Tombstoned bool `json:"tombstoned,omitempty"`
}

// Active reports whether the entity participates in resolve/neighbors:
// neither forgotten nor superseded.
//
// The RULE travels with the ROW: a Go type alias cannot carry methods, so a
// rule left in the module could not be reached by the layers above it — and
// this rule decides whether an entity still counts.
func (e Entity) Active() bool { return !e.Tombstoned && e.SupersededBy == "" }

// Relation is a directed, weighted edge between two entity ids.
type Relation struct {
	// ID is content-addressed: hex(BLAKE3("rel" \0 from \0 verb \0 to)).
	ID   string `json:"id"`
	From string `json:"from"` // source entity id
	Verb Verb   `json:"verb"` // owns, depends_on, ...
	To   string `json:"to"`   // target entity id
	// Weight strengthens on re-assertion; decayed by reflection later.
	Weight       float64 `json:"weight"`
	SourceEvent  string  `json:"source_event,omitempty"`
	CreatedMS    int64   `json:"created_ms"`
	LastSeenMS   int64   `json:"last_seen_ms"`
	SupersededBy string  `json:"superseded_by,omitempty"`
	Tombstoned   bool    `json:"tombstoned,omitempty"`
}

// Active reports whether the relation participates in neighbors.
func (r Relation) Active() bool { return !r.Tombstoned && r.SupersededBy == "" }

// UpsertSpec is the input to Upsert.
type UpsertSpec struct {
	Kind    Kind
	Name    string
	Aliases []string
	Attrs   map[string]string
	Weight  float64
}

// ScoredEntity is an entity paired with its resolve score.
type ScoredEntity struct {
	Entity Entity  `json:"entity"`
	Score  float64 `json:"score"`
}

// Neighbor is one edge incident to an entity together with the adjacent
// entity on the other end (nil-safe: Other is the zero Entity if the adjacent
// node is missing/forgotten).
type Neighbor struct {
	Relation Relation `json:"relation"`
	// Outgoing is true when the queried entity is the edge's From.
	Outgoing bool   `json:"outgoing"`
	Other    Entity `json:"other"`
}

// DecayOptions tune Decay. Zero values fall back to sane defaults in Decay.
type DecayOptions struct {
	// StaleAfterMS: an entity not referenced (LastSeenMS) within this window
	// is considered stale and decayed. Default 14 days.
	StaleAfterMS int64
	// Factor multiplies a stale entity's weight (0<Factor<1). Default 0.8.
	Factor float64
	// Floor is the minimum weight decay will drive toward; weight never goes
	// below it (so a decayed entity still resolves, just ranks lower).
	// Default 0.1.
	Floor float64
}
