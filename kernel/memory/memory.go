// SPDX-License-Identifier: MIT

// Package memory implements the memory store (ROADMAP §2.3): a journaled,
// content-addressed knowledge store that the agent loop reads as injected
// context and that the operator, the agent, and an auto-distiller can write
// to. Retrieval is hybrid (M803): exact keyword overlap blended with local
// hashed-n-gram embeddings (vector.go) for typo/morphology recall —
// DECISIONS C5's "local embeddings by default"; provider embeddings remain
// the documented opt-in.
//
// Two layers, mirroring how kernel/state and kernel/runtime split:
//
//   - Store (this file) is a pure, file-backed record store — no bus, no
//     journaling — so it is trivially testable and a CobaltDB-class engine
//     (DECISIONS D2) can replace it behind the interface. It also owns the
//     content-addressing and the keyword retrieval ranking, both pure
//     functions.
//   - Manager (manager.go) wraps a Store with the kernel bus so every
//     mutation is a durable-before-publish event carrying the run's
//     correlation_id — which is what makes `agt why` able to explain every
//     belief (SPEC-05 §2).
//
// Records are content-addressed (BLAKE3 of type\0subject\0content) so
// identical knowledge dedupes; updates are soft (SupersededBy) and forgets
// are soft (Tombstoned) — history is never destructively edited.
//
// Concurrency: a single Store instance is safe for concurrent use.
package memory

import (
	"encoding/hex"

	"lukechampine.com/blake3"
)


// Type is the canonical memory-record discriminator. The set mirrors
// SPEC-05 §2 MemoryType, minus SKILL_REF (skills land with Forge).
type Type string

const (
	TypeFact        Type = "FACT"        // a durable fact worth recalling
	TypeSummary     Type = "SUMMARY"     // a distilled summary of work done
	TypeRelation    Type = "RELATION"    // a relation between entities
	TypePreference  Type = "PREFERENCE"  // a stated/learned user preference
	TypeObservation Type = "OBSERVATION" // something noticed, lower confidence
)

// DefaultType is used when a writer leaves the type unset.
const DefaultType = TypeFact

// Evidence records how a memory entered the store. It is deliberately
// orthogonal to Type: "OBSERVATION" as a Type means the content is a note;
// EvidenceObserved means the note was grounded in a direct observation.
type Evidence string

const (
	EvidenceUnknown    Evidence = ""
	EvidenceObserved   Evidence = "observed"
	EvidenceInferred   Evidence = "inferred"
	EvidenceCurated    Evidence = "curated"
	EvidenceConstraint Evidence = "constraint"
)

// validTypes is the membership set for ValidType.
var validTypes = map[Type]struct{}{
	TypeFact: {}, TypeSummary: {}, TypeRelation: {},
	TypePreference: {}, TypeObservation: {},
}

// ValidType reports whether t is one of the known memory types.
func ValidType(t Type) bool {
	_, ok := validTypes[t]
	return ok
}

// Record is one unit of memory. Field order is not load-bearing (records are
// not part of the event hash chain), but the JSON tags are stable so the
// on-disk file and the CLI/`--json` shape stay compatible across releases.
type Record struct {
	// ID is content-addressed: hex(BLAKE3(type \0 subject \0 content)).
	ID string `json:"id"`
	// Type classifies the knowledge (FACT, SUMMARY, ...).
	Type Type `json:"type"`
	// Subject is the entity/topic this is about (used for retrieval).
	Subject string `json:"subject"`
	// Content is the text the model sees when this record is injected.
	Content string `json:"content"`
	// Tags are free-form labels (e.g. source=distill, project=lictor).
	Tags map[string]string `json:"tags,omitempty"`
	// SourceEvent is the journal event id that produced this record —
	// provenance for `agt why`.
	SourceEvent string `json:"source_event,omitempty"`
	// AddedBy / UpdatedBy record WHO wrote this (M851): the acting agent's slug,
	// or "operator" for a direct console/CLI write, or "distill" for an
	// auto-distilled summary. AddedBy is first-writer-wins (preserved on
	// reinforce, like SourceEvent); UpdatedBy is the most recent writer. Empty on
	// legacy records written before provenance.
	AddedBy   string `json:"added_by,omitempty"`
	UpdatedBy string `json:"updated_by,omitempty"`
	// Confidence is 0..1; ranking weights it and it can strengthen on
	// re-observation. Defaults to 1.0 for explicit writes.
	Confidence float64 `json:"confidence"`
	// Evidence distinguishes observed, inferred, curated, and constraint-like
	// memories. Retrieval treats this as metadata, not as truth authority.
	Evidence Evidence `json:"evidence,omitempty"`
	// CreatedMS / LastSeenMS drive recency in ranking and decay.
	CreatedMS  int64 `json:"created_ms"`
	LastSeenMS int64 `json:"last_seen_ms"`
	// HalfLifeMS is the mechanical expiration budget. Once LastSeenMS+HalfLifeMS
	// is in the past, the record is retained but barred from active use until it
	// is reconstructed/reinforced.
	HalfLifeMS int64 `json:"half_life_ms,omitempty"`
	// SuspendedMS marks a record that failed an epistemic hygiene check. It is
	// retained and gettable, but excluded from recall/search.
	SuspendedMS     int64  `json:"suspended_ms,omitempty"`
	SuspendedReason string `json:"suspended_reason,omitempty"`
	// SupersededBy points at a newer record when this one was replaced
	// (soft update — the old record is retained, not edited away).
	SupersededBy string `json:"superseded_by,omitempty"`
	// Tombstoned marks a soft-forgotten record: excluded from recall but
	// retained on disk and in the journal (reversibility).
	Tombstoned bool `json:"tombstoned,omitempty"`
}

// Active reports whether the record should participate in retrieval:
// neither forgotten nor superseded.
func (r Record) Active() bool { return !r.Tombstoned && r.SupersededBy == "" }

// Suspended reports whether the record is explicitly barred from active use.
func (r Record) Suspended() bool { return r.SuspendedMS > 0 }

// Expired reports whether the record's reconstruction budget has elapsed.
// A zero HalfLifeMS means legacy/no-expiry records remain usable.
func (r Record) Expired(nowMS int64) bool {
	return r.HalfLifeMS > 0 && nowMS > r.LastSeenMS+r.HalfLifeMS
}

// Usable reports whether the record may participate in retrieval at nowMS.
func (r Record) Usable(nowMS int64) bool {
	return r.Active() && !r.Suspended() && !r.Expired(nowMS)
}

// ContentID computes the content-addressed id for a (type, subject, content)
// triple. NUL separators avoid ("ab","c") colliding with ("a","bc").
func ContentID(t Type, subject, content string) string {
	h := blake3.New(32, nil)
	h.Write([]byte(string(t)))
	h.Write([]byte{0})
	h.Write([]byte(subject))
	h.Write([]byte{0})
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

// ScopedID extends ContentID with the record's scope (M915): the same fact
// written privately by two different agents must yield two records, not one
// record whose scope tag flips to the latest writer (silently hiding the note
// from the first agent). An empty scope hashes identically to ContentID, so
// every pre-existing shared record keeps its id.
func ScopedID(t Type, subject, content, scope string) string {
	if scope == "" {
		return ContentID(t, subject, content)
	}
	h := blake3.New(32, nil)
	h.Write([]byte(string(t)))
	h.Write([]byte{0})
	h.Write([]byte(subject))
	h.Write([]byte{0})
	h.Write([]byte(content))
	h.Write([]byte{0})
	h.Write([]byte(scope))
	return hex.EncodeToString(h.Sum(nil))
}

