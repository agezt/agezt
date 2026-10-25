// SPDX-License-Identifier: MIT

package worldmodel

// Provenance: Package worldmodel implements "World Model v1" (SPEC-05 §3): a
//             journaled, content-addressed graph of the operator's world. The Kind +
//             Verb + Entity + Relation + Store + graphData + FileStore types + Open
//             + EntityID + RelationID + NormalizeKind + NormalizeVerb + Active +
//             sortEntities + sortRelations helpers live here. The FileStore method
//             implementations (PutEntity + GetEntity + AllEntities + PutRelation +
//             GetRelation + AllRelations + Count + Close + snapshotLocked) moved to
//             worldmodel_store.go. Day-211 god-file split. Public API unchanged.

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
	"github.com/agezt/agezt/kernel/platform/filestore"
	"lukechampine.com/blake3"
)

// Kind classifies an entity (SPEC-05 §3.2). It is an open string — the set
// below is the validated, well-known vocabulary, but an unknown kind is
// accepted (NormalizeKind keeps it verbatim) so the graph never refuses to
// learn something just because it is a new category.
//
// The row itself lives in the contract layer (kernel/contract/worldmodelapi);
// the name stays here as an alias, so every existing caller is unchanged.
type Kind = worldmodelapi.Kind

const (
	KindProject = worldmodelapi.KindProject
	KindRepo    = worldmodelapi.KindRepo
	KindPerson  = worldmodelapi.KindPerson
	KindOrg     = worldmodelapi.KindOrg
	KindAccount = worldmodelapi.KindAccount
	KindDevice  = worldmodelapi.KindDevice
	KindChannel = worldmodelapi.KindChannel
	KindTopic   = worldmodelapi.KindTopic
	KindTask    = worldmodelapi.KindTask
)

// DefaultKind is used when an entity is written with no kind.
const DefaultKind = worldmodelapi.DefaultKind

// NormalizeKind lowercases/trims a kind and falls back to DefaultKind when the
// input is empty. Unknown-but-non-empty kinds are kept verbatim (permissive):
// the graph records the operator's vocabulary rather than rejecting it.
func NormalizeKind(k Kind) Kind { return worldmodelapi.NormalizeKind(k) }

// Verb classifies a relation (SPEC-05 §3.2). Like Kind it is an open string
// with a well-known set; NormalizeVerb defaults the empty verb to VerbRelatesTo.
//
// The row itself lives in the contract layer (kernel/contract/worldmodelapi);
// the name stays here as an alias, so every existing caller is unchanged.
type Verb = worldmodelapi.Verb

const (
	VerbOwns        = worldmodelapi.VerbOwns
	VerbDependsOn   = worldmodelapi.VerbDependsOn
	VerbMemberOf    = worldmodelapi.VerbMemberOf
	VerbPrefers     = worldmodelapi.VerbPrefers
	VerbRelatesTo   = worldmodelapi.VerbRelatesTo
	VerbAssignedTo  = worldmodelapi.VerbAssignedTo
	VerbDerivedFrom = worldmodelapi.VerbDerivedFrom
)

// DefaultVerb is used when a relation is written with no verb.
const DefaultVerb = worldmodelapi.DefaultVerb

// NormalizeVerb lowercases/trims a verb and falls back to DefaultVerb when
// empty. Unknown-but-non-empty verbs are kept verbatim.
func NormalizeVerb(v Verb) Verb { return worldmodelapi.NormalizeVerb(v) }

// Entity is a node in the world model. The JSON tags are stable so the
// on-disk file and the CLI/`--json` shape stay compatible across releases;
// field order is not load-bearing (entities are not part of the event hash
// chain — their provenance events are).
//
// The row itself — with its Active RULE — lives in the contract layer
// (kernel/contract/worldmodelapi); the name stays here as an alias, so every
// existing caller is unchanged.
type Entity = worldmodelapi.Entity

// Relation is a directed, weighted edge between two entity ids.
//
// The row itself — with its Active RULE — lives in the contract layer
// (kernel/contract/worldmodelapi); the name stays here as an alias, so every
// existing caller is unchanged.
type Relation = worldmodelapi.Relation

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
//
// The port lives in the contract layer (kernel/contract/worldmodelapi); the
// name stays here as an alias, so every existing caller is unchanged.
type Store = worldmodelapi.Store

// ErrEmptyName is returned when an entity is written with no name.
//
// The sentinel itself lives in the contract layer
// (kernel/contract/worldmodelapi); the name stays here as an alias, so every
// existing caller is unchanged.
var ErrEmptyName = worldmodelapi.ErrEmptyName

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
//
// It satisfies the contract's store port, so the layers above it never name
// this module.
var _ worldmodelapi.Store = (*FileStore)(nil)

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
	path, err := filestore.LoadFrom(dir, "worldmodel.json", &gd)
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
	if err := filestore.Save(s.path, graphData{
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
