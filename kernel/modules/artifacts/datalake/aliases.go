// SPDX-License-Identifier: MIT

package datalake

// The rows and the errors now live in the contract layer
// (kernel/contract/datalakeapi) — W3.30. The module keeps its names as
// ALIASES, so there is one row and one spelling of it: the lake here, the
// agent-facing tool there, and the app in between all speak the same
// collection.
//
// The lake itself — the on-disk store — stays here: it is the implementation.

import "github.com/agezt/agezt/kernel/contract/datalakeapi"

// The rows.
type (
	// Field describes one column of a collection.
	Field = datalakeapi.Field
	// Schema is a collection's definition.
	Schema = datalakeapi.Schema
	// Record is one row.
	Record = datalakeapi.Record
	// Query narrows a listing.
	Query = datalakeapi.Query
	// CollectionInfo is the listed summary of a collection.
	CollectionInfo = datalakeapi.CollectionInfo
)

// The errors the lake answers with.
var (
	// ErrNotFound is returned when a collection or record id does not exist.
	ErrNotFound = datalakeapi.ErrNotFound
	// ErrExists is returned by CreateCollection when the name is taken.
	ErrExists = datalakeapi.ErrExists
	// ErrSystem is returned when a system collection must not be dropped.
	ErrSystem = datalakeapi.ErrSystem
)

// The lake answers the contract's port, so the agent-facing tool can be
// handed it without ever naming this module.
var _ datalakeapi.Lake = (*Lake)(nil)
