// SPDX-License-Identifier: MIT

// Package worldmodel implements "World Model v1" (SPEC-05 §3): a journaled,
// content-addressed graph of the operator's world. The Kind + Verb + Entity
// + Relation + Store + graphData + FileStore types + Open + EntityID +
// RelationID + NormalizeKind + NormalizeVerb + Active + sortEntities +
// sortRelations helpers live here. The FileStore method implementations
// (PutEntity + GetEntity + AllEntities + PutRelation + GetRelation +
// AllRelations + Count + Close + snapshotLocked) moved to
// worldmodel_store.go. Day-211 god-file split. Public API unchanged.
package worldmodel


import (
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/jsonstore"
	"lukechampine.com/blake3"
)
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

// EntityID computes the content-addressed id for a (kind, name) pair. The
// "entity" domain prefix and NUL separators keep it disjoint from relation
// ids and avoid (kind,name) concatenation collisions. Kind and name are
// normalized (lower/trim) so "Lictor" and "lictor" address the same node.
func EntityID(kind Kind, name string) string {
	h := blake3.New(32, nil)
	h.Write([]byte("entity"))
	h.Write([]byte{0})
	h.Write([]byte(string(NormalizeKind(kind))))
	h.Write([]byte{0})
	h.Write([]byte(strings.ToLower(strings.TrimSpace(name))))
	return hex.EncodeToString(h.Sum(nil))
}

// RelationID computes the content-addressed id for a (from, verb, to) triple.
func RelationID(from string, verb Verb, to string) string {
	h := blake3.New(32, nil)
	h.Write([]byte("rel"))
	h.Write([]byte{0})
	h.Write([]byte(from))
	h.Write([]byte{0})
	h.Write([]byte(string(NormalizeVerb(verb))))
	h.Write([]byte{0})
	h.Write([]byte(to))
	return hex.EncodeToString(h.Sum(nil))
}

// Store is the pure graph store. Implementations persist entities and
// relations by id and must be safe for concurrent use.
type Store interface {
	PutEntity(e Entity) error
	GetEntity(id string) (Entity, bool, error)
	AllEntities() ([]Entity, error)
	PutRelation(r Relation) error
	GetRelation(id string) (Relation, bool, error)
	AllRelations() ([]Relation, error)
	// Count returns the number of entities currently stored (all states).
	Count() int
	Close() error
}

// ErrEmptyName is returned when an entity is written with no name.
var ErrEmptyName = errors.New("worldmodel: empty entity name")

// graphData is the on-disk shape: a single object holding both maps so the
// whole graph snapshots atomically in one file.
type graphData struct {
	Entities  map[string]Entity   `json:"entities"`
	Relations map[string]Relation `json:"relations"`
}

// FileStore is the file-backed Store. The whole graph lives in a single
// <dir>/worldmodel.json object, snapshotted atomically on every mutation
// (write-temp + rename). Simple and crash-safe; adequate for v1 (DECISIONS D2
// names a CobaltDB adjacency backend for scale later).
type FileStore struct {
	path string

	mu        sync.RWMutex
	entities  map[string]Entity
	relations map[string]Relation
}

// Open opens (or creates) a FileStore under dir, loading <dir>/worldmodel.json
// if present. The directory is created if absent.
func Open(dir string) (*FileStore, error) {
	s := &FileStore{
		entities:  make(map[string]Entity),
		relations: make(map[string]Relation),
	}
	var gd graphData
	path, err := jsonstore.LoadFrom(dir, "worldmodel.json", &gd)
	if err != nil {
		return nil, fmt.Errorf("worldmodel: %w", err)
	}
	s.path = path
	if gd.Entities != nil {
		s.entities = gd.Entities
	}
	if gd.Relations != nil {
		s.relations = gd.Relations
	}
	return s, nil
}

// PutEntity implements Store.

// snapshotLocked writes the whole graph atomically. Caller holds s.mu.
func (s *FileStore) snapshotLocked() error {
	if err := jsonstore.Save(s.path, graphData{
		Entities:  s.entities,
		Relations: s.relations,
	}); err != nil {
		return fmt.Errorf("worldmodel: %w", err)
	}
	return nil
}

func sortEntities(es []Entity) {
	sort.Slice(es, func(i, j int) bool {
		if es[i].CreatedMS != es[j].CreatedMS {
			return es[i].CreatedMS < es[j].CreatedMS
		}
		return es[i].ID < es[j].ID
	})
}

func sortRelations(rs []Relation) {
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].CreatedMS != rs[j].CreatedMS {
			return rs[i].CreatedMS < rs[j].CreatedMS
		}
		return rs[i].ID < rs[j].ID
	})
}
