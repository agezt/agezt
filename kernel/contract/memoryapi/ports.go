// SPDX-License-Identifier: MIT

package memoryapi

// Provenance: the memory ports (W3.57). The knowledge module's record store,
//             its publisher, its manager and the construction seam the daemon
//             hands over moved here, so the layers above the module speak to
//             a PORT and never to the module's concrete types.
//
// The ports are SPLIT, not widened: a reader that could also write, retire
// or distil would hand every read-only consumer the whole memory lifecycle.
//
// The publisher is a PORT, not the bus: the bus answers with the durable
// envelope, and the envelope carries the hash chain the contract layer may
// not name. The port answers with the id of the event it wrote — that is
// exactly what a record keeps as its provenance (SourceEvent), and what
// `agt why` reads.
//
// The generator is a PORT too: the structured-output path lives in another
// module, and a module may not name another module. The host injects it, and
// the distillers use it to turn a model's answer into a typed value.

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// Store is the pure record store. Implementations persist records by id and
// must be safe for concurrent use.
//
// The store is the daemon's file: the host opens it and hands it over.
type Store interface {
	// Put upserts r by its ID (overwrites any existing record with that id).
	Put(r Record) error
	// Get returns the record at id; the bool is false if absent.
	Get(id string) (Record, bool, error)
	// Delete hard-removes the record at id (returns false if absent). Unlike
	// Forget (which tombstones), this reclaims the row — used only by the prune
	// pass on already-soft-deleted records (M857).
	Delete(id string) (bool, error)
	// All returns every record (including tombstoned/superseded ones),
	// sorted deterministically by CreatedMS then ID.
	All() ([]Record, error)
	// Count returns the number of records currently stored (all states).
	Count() int
	// Close releases any held resources.
	Close() error
}

// Publisher is the port through which the manager journals a mutation.
//
// It is NOT the bus: the bus returns the durable envelope, and the contract
// layer may not name it (the envelope is hashed into the event chain). The
// port answers with the id of the event it wrote — that is the record's
// provenance (SourceEvent).
type Publisher interface {
	// Publish writes one durable event and answers its id.
	Publish(spec eventapi.Spec) (id string, err error)
}

// Generator produces one schema-valid JSON value from a provider and
// unmarshals it into out. It is the structured-output path, injected by the
// host: the module that owns it is not the memory module's to name.
//
// A nil generator means "no structured output available" — the distillers
// then report that, rather than pretending they distilled something.
//
// The path is not the memory module's: it is the provider contract's
// (`llm.Generator`), and this name is an ALIAS of it, so there is one
// spelling of the structured-output path in the tree (W3.66 — the pulse
// module needed the same path, and a second identical type would have been
// a second name for one thing).
type Generator = llm.Generator

// Reader is the read-only half of the memory: what an agent may look up, and
// what a health view may count.
type Reader interface {
	// Recall searches the shared memory and journals the lookup.
	Recall(corr, query string, limit int) ([]Scored, error)
	// RecallScoped searches the shared memory AND the given scope.
	RecallScoped(corr, query string, limit int, scope string) ([]Scored, error)
	// Get returns one record by id.
	Get(id string) (Record, bool, error)
	// Active returns the records that take part in retrieval.
	Active() ([]Record, error)
	// All returns every record, in every state.
	All() ([]Record, error)
	// Count returns how many records the store holds.
	Count() int
	// Search ranks the usable records against a query.
	Search(query string, limit int) ([]Scored, error)
	// SearchScoped ranks the usable records of one scope against a query.
	SearchScoped(query string, limit int, scope string) ([]Scored, error)
	// ProfileText renders the operator profile block.
	ProfileText() string
}

// Writer is the mutating half of the memory: the verbs that change what a
// later run will recall.
type Writer interface {
	// Remember stores (or reinforces) a record.
	Remember(corr string, spec RememberSpec) (Record, bool, error)
	// Forget soft-deletes a record (the row is retained).
	Forget(corr, id string) (bool, error)
	// Promote revives a suspended record.
	Promote(corr, id string) (Record, bool, error)
	// Supersede replaces a record with a newer one (the old row is retained).
	Supersede(corr, oldID string, spec RememberSpec) (Record, error)
	// Suspend bars a record from active use after a hygiene check.
	Suspend(corr, id, reason string) (bool, error)
}

// Maintainer is the hygiene half: the verbs that keep the store small and
// honest. They are the operator's, not an agent's.
type Maintainer interface {
	// Hygiene reports the store's health.
	Hygiene(olderThanMs int64) (HygieneStats, error)
	// Prune reclaims the soft-deleted rows older than the cutoff.
	Prune(corr string, olderThanMs int64, dryRun bool) (int, error)
	// CleanLowValue hard-deletes the records that never belonged in memory.
	CleanLowValue(corr string, dryRun bool) (CleanReport, error)
	// Audit reports stale and competing memories.
	Audit() (AuditReport, error)
	// DedupeDistilled collapses the backlog of distilled duplicates.
	DedupeDistilled(corr string, dryRun bool) (int, error)
}

// Distiller is the LLM-bound half: the passes that turn a transcript, a
// cluster or a corpus into new records.
//
// The provider is a PARAMETER, not a field: the caller owns the model and
// the credentials, and the module never holds a provider of its own.
type Distiller interface {
	// Distill extracts durable facts from a finished run's transcript.
	Distill(ctx context.Context, corr string, provider llm.Provider, model, intent, transcript string) ([]string, error)
	// DistillBrain consolidates related records into one.
	DistillBrain(ctx context.Context, corr string, provider llm.Provider, model string) (BrainDistillReport, error)
	// DistillProfile synthesises the operator profile.
	DistillProfile(ctx context.Context, corr string, provider llm.Provider, model string) (ProfileReport, error)
}

// ReaderWriter is the read-and-write half of the memory — what an
// agent-facing surface needs: it looks notes up and it writes them, and it
// never maintains or distils.
//
// It exists because both of those surfaces exist: the agent gateway and the
// in-process memory tool. Neither may maintain the store, and neither may
// spend a provider on a distillation pass.
type ReaderWriter interface {
	Reader
	Writer
}

// Tooler is the agent-facing surface of the memory: the in-process tool the
// model calls to remember, recall, forget and find related notes.
//
// It is a separate one-verb port because it answers a different question —
// not "what does the store hold" but "what may the agent call".
type Tooler interface {
	// Tool returns the memory tool.
	Tool() toolapi.Tool
}

// Manager is the whole memory surface the host drives — the composition of
// the halves above.
//
// It exists as one name so a host that legitimately drives every half (the
// control plane's maintenance commands) may hold one port; a host that only
// reads holds the Reader, and a host that only recalls holds the Reader too.
type Manager interface {
	Reader
	Writer
	Maintainer
	Distiller
	Tooler
}

// Deps are the construction-time dependencies of the memory manager.
//
// They are gathered in ONE struct rather than a long parameter list: the
// store is the host's file, the publisher is the kernel's, the embedder and
// the generator are the host's seams — and a builder that takes four
// positional values would invite the wrong order at the call site.
type Deps struct {
	// Store is the record store the host opened.
	Store Store
	// Publisher is how the manager journals its mutations.
	Publisher Publisher
	// Embedder is the optional provider embedder (nil = local hashing).
	Embedder Embedder
	// Generator is the structured-output path the distillers use.
	Generator Generator
}

// Builder builds the memory manager over the dependencies the host collected.
//
// The daemon hands the builder over: it names the module, the kernel names
// only the contract. The kernel calls the builder with its own publisher, so
// every mutation journals through the kernel's bus without either side
// naming the other's implementation.
type Builder func(deps Deps) Manager
