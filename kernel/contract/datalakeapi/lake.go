// SPDX-License-Identifier: MIT

package datalakeapi

// Lake is the port through which the lake is driven.
//
// The implementation — the on-disk lake in the artifacts module — satisfies
// it. The agent-facing tool (a plugin) names this port, never the
// implementation.
type Lake interface {
	// ListCollections returns every collection with its record count.
	ListCollections() []CollectionInfo
	// CreateCollection creates a collection from its schema.
	CreateCollection(sc Schema, actor string) (Schema, error)
	// DropCollection removes a collection.
	DropCollection(name string) error
	// Schema returns one collection's definition; the bool is false if absent.
	Schema(name string) (Schema, bool)
	// Insert adds one record to a collection.
	Insert(coll string, fields map[string]any, actor string) (Record, error)
	// Get returns one record by id.
	Get(coll, id string) (Record, error)
	// Update patches one record's fields.
	Update(coll, id string, patch map[string]any, actor string) (Record, error)
	// Delete removes one record.
	Delete(coll, id string) error
	// Query returns the records that match a query.
	Query(coll string, q Query) ([]Record, error)
}
