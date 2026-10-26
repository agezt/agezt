// SPDX-License-Identifier: MIT

package memory

// Package documentation lives in doc.go.

import (
	"encoding/hex"

	"lukechampine.com/blake3"

	"github.com/agezt/agezt/kernel/contract/memoryapi"
)

// Type is the canonical memory-record discriminator. The set mirrors
// SPEC-05 §2 MemoryType, minus SKILL_REF (skills land with Forge).
//
// The row lives in the CONTRACT (W3.57): the layers above the module compare
// against these names, and they may not name this module.
type Type = memoryapi.Type

const (
	TypeFact        = memoryapi.TypeFact        // a durable fact worth recalling
	TypeSummary     = memoryapi.TypeSummary     // a distilled summary of work done
	TypeRelation    = memoryapi.TypeRelation    // a relation between entities
	TypePreference  = memoryapi.TypePreference  // a stated/learned user preference
	TypeObservation = memoryapi.TypeObservation // something noticed, lower confidence
)

// DefaultType is used when a writer leaves the type unset.
const DefaultType = memoryapi.DefaultType

// Evidence records how a memory entered the store. It is deliberately
// orthogonal to Type: "OBSERVATION" as a Type means the content is a note;
// EvidenceObserved means the note was grounded in a direct observation.
type Evidence = memoryapi.Evidence

const (
	EvidenceUnknown    = memoryapi.EvidenceUnknown
	EvidenceObserved   = memoryapi.EvidenceObserved
	EvidenceInferred   = memoryapi.EvidenceInferred
	EvidenceCurated    = memoryapi.EvidenceCurated
	EvidenceConstraint = memoryapi.EvidenceConstraint
)

// ValidType reports whether t is one of the known memory types.
//
// The RULE travels with the row: the membership set is part of the type's
// meaning, and a rule left in the implementation could not be reached by the
// layers above.
func ValidType(t Type) bool { return memoryapi.ValidType(t) }

// Record is one unit of memory.
//
// The row lives in the CONTRACT (W3.57) — with the rules that judge it
// (Active, Suspended, Expired, Usable), because a Go type alias cannot carry
// methods: a rule that stayed here could not be reached by the layers above.
type Record = memoryapi.Record

// Scored is a record paired with its retrieval score.
type Scored = memoryapi.Scored

// RememberSpec is the input to Remember.
type RememberSpec = memoryapi.RememberSpec

// ErrRecordNotFound is returned when a record id names nothing in the store.
//
// The sentinel lives in the contract (W3.57): the layers above the module
// compare against it, and they may not name this module.
var ErrRecordNotFound = memoryapi.ErrRecordNotFound

// ContentID computes the content-addressed id for a (type, subject, content)
// triple. NUL separators avoid ("ab","c") colliding with ("a","bc").
//
// It stays HERE, in the module: it hashes with BLAKE3, and the contract layer
// may not name a third-party package (principle P4). No layer above the
// module computes an id — the module does, and stores it on the row.
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
