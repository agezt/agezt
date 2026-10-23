// SPDX-License-Identifier: MIT

package datalakeapi

import (
	"errors"
)

// ErrNotFound is returned when a collection or record id does not exist.
var ErrNotFound = errors.New("datalake: not found")

// ErrExists is returned by CreateCollection when the name is already taken.
var ErrExists = errors.New("datalake: collection already exists")

// ErrSystem is returned when an operation is refused on a system collection
// (a built-in that must not be dropped).
var ErrSystem = errors.New("datalake: system collection cannot be dropped")

// Field describes one column of a collection — its key, a coarse type the UI
// uses to render and the agent uses as a hint, and a human label.
type Field struct {
	Name  string `json:"name"`
	Type  string `json:"type,omitempty"` // text | number | money | date | bool | url | tags | note
	Label string `json:"label,omitempty"`
}

// Schema is a collection's definition. Fields are advisory — records may carry
// extra keys (the store is schemaless at heart) — but they drive validation
// hints and the Web UI rendering. View names a bespoke front-end (e.g.
// "expense", "calendar", "tasks") or "" / "table" for the generic grid.
type Schema struct {
	Name      string  `json:"name"`
	Title     string  `json:"title,omitempty"`
	Icon      string  `json:"icon,omitempty"` // lucide icon name for the Web UI
	View      string  `json:"view,omitempty"` // table (default) | expense | calendar | tasks | notes | habits | bookmarks | contacts
	Desc      string  `json:"desc,omitempty"`
	Fields    []Field `json:"fields,omitempty"`
	Builtin   bool    `json:"builtin,omitempty"` // seeded by the daemon, not user-created
	System    bool    `json:"system,omitempty"`  // must not be dropped
	CreatedMs int64   `json:"created_ms"`
	CreatedBy string  `json:"created_by,omitempty"`
}

// Record is one row: a stable id, the free-form field map, and provenance — who
// (which run/agent) created and last updated it, and when. The provenance fields
// answer the operator's "which agent put this here?".
type Record struct {
	ID        string         `json:"id"`
	Fields    map[string]any `json:"fields"`
	CreatedMs int64          `json:"created_ms"`
	UpdatedMs int64          `json:"updated_ms"`
	CreatedBy string         `json:"created_by,omitempty"`
	UpdatedBy string         `json:"updated_by,omitempty"`
}

// Query narrows List. All constraints are ANDed; empty fields impose nothing.
type Query struct {
	Search string         // case-insensitive substring across all string field values
	Equals map[string]any // exact field matches (compared by JSON-normalised value)
	SortBy string         // field name to sort by; "" → created time
	Desc   bool           // descending sort (default ascending; created-time default is newest-first)
	Limit  int            // 0 → no limit
	Offset int
}

// CollectionInfo is the listed summary of a collection (no records).
type CollectionInfo struct {
	Schema
	Count int `json:"count"`
}
