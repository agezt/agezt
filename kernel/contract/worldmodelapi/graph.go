// SPDX-License-Identifier: MIT

package worldmodelapi

import (
	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// Publisher is the port through which the graph journals a mutation.
//
// It is NOT the bus: the bus returns the durable envelope, and the contract
// layer may not name it (the envelope is hashed into the event chain). The
// port answers with the id of the event it wrote — that is what the graph
// records as the entity's provenance (SourceEvent), and `agt why` reads it.
//
// The kernel holds the port; the daemon builds the adapter over the kernel's
// bus. A graph built without one journals nothing (the store-only tests).
type Publisher interface {
	// Publish writes one durable event and answers its id.
	Publish(spec eventapi.Spec) (id string, err error)
}

// Graph is the port the kernel holds for the world model: the journaled,
// content-addressed graph of the operator's world.
//
// The port lives in the contract layer because four layers have to speak it
// and none of them may name the module's implementation:
//
//   - the kernel resolves entities for a run and registers the world tool;
//   - the reflection engine decays what the operator stopped mentioning;
//   - the app layer curates the graph through typed operations;
//   - the control plane reaches it through the kernel's accessor.
//
// The daemon opens the file and hands the kernel this port; a host that was
// given none holds a nil port and the world surfaces report that the graph is
// unavailable rather than calling into nothing.
//
// The port is the graph's whole public surface — the fourteen verbs the layers
// above actually drive, no more. A wider port would let a caller reach past
// the graph's own rules (it content-addresses, normalizes, journals
// durable-before-publish and soft-deletes); a narrower one would force a
// second shape to be kept in step with this one.
type Graph interface {
	// Upsert creates (or reinforces) an entity and journals the write. The
	// bool says whether it was newly created.
	Upsert(corr string, spec UpsertSpec) (Entity, bool, error)
	// EditEntity merges new aliases and attrs into an entity and journals
	// the write.
	EditEntity(corr, id string, aliases []string, attrs map[string]string) (Entity, bool, error)
	// Relate asserts a directed edge between two entities and journals it.
	Relate(corr, fromName string, verb Verb, toName string) (Relation, error)
	// Resolve answers the entities matching a phrase, ranked.
	Resolve(corr, phrase string, limit int) ([]ScoredEntity, error)
	// ResolveQuiet answers the same matches without journaling the lookup.
	ResolveQuiet(phrase string, limit int) ([]ScoredEntity, error)
	// IsActiveSubject reports whether text refers to a known active entity,
	// answering that entity's name when it does.
	IsActiveSubject(text string) (string, bool)
	// Neighbors answers the entities adjacent to an entity, with the verbs
	// that connect them.
	Neighbors(entityID string) ([]Neighbor, error)
	// Forget soft-deletes an entity and journals it. The bool says whether
	// it was there.
	Forget(corr, id string) (bool, error)
	// Get returns one entity by id; the bool says whether it was there.
	Get(id string) (Entity, bool, error)
	// Entities returns every entity.
	Entities() ([]Entity, error)
	// Relations returns every relation.
	Relations() ([]Relation, error)
	// Count returns the number of entities (all states).
	Count() int
	// Decay lowers the weight of the entities the operator stopped
	// mentioning and journals every change; it answers how many it touched.
	Decay(corr string, opts DecayOptions) (int, error)
	// Tool returns the graph's agent-facing tool.
	Tool() toolapi.Tool
}
