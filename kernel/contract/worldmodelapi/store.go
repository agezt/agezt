// SPDX-License-Identifier: MIT

package worldmodelapi

// Store is the port the daemon's world-model file implements: the durable
// graph of entities and relations, one JSON file snapshotted atomically on
// every mutation.
//
// The port lives in the contract layer because the graph and the layers above
// it have to speak it and none of them may name the module's implementation:
// the daemon opens the file, the graph writes through it, and the kernel holds
// the graph behind its own port.
//
// The daemon opens the file and hands the store over; a host that was given
// none holds a nil port rather than a port wrapping nothing.
//
// The port is the store's whole public surface — eight verbs, no more.
type Store interface {
	// PutEntity writes an entity by id.
	PutEntity(e Entity) error
	// GetEntity returns one entity by id; the bool says whether it was there.
	GetEntity(id string) (Entity, bool, error)
	// AllEntities returns every entity, in a deterministic order.
	AllEntities() ([]Entity, error)
	// PutRelation writes a relation by id.
	PutRelation(r Relation) error
	// GetRelation returns one relation by id; the bool says whether it was
	// there.
	GetRelation(id string) (Relation, bool, error)
	// AllRelations returns every relation, in a deterministic order.
	AllRelations() ([]Relation, error)
	// Count returns the number of entities currently stored (all states).
	Count() int
	// Close releases the store's resources.
	Close() error
}
