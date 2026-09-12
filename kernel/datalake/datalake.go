// SPDX-License-Identifier: MIT

// Lake core: types + Open + loadCollection + validName.
// Code extracted from datalake.go during the Day-68 god-file split. Public API unchanged.
package datalake


import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

type collection struct {
	schema  Schema
	records map[string]Record
}

// Lake is the on-disk structured store. Safe for concurrent use.
type Lake struct {
	dir   string
	mu    sync.Mutex
	colls map[string]*collection
	now   func() int64
}

// Open loads (creating if needed) the data lake rooted at <baseDir>/datalake.
func Open(baseDir string, now func() int64) (*Lake, error) {
	dir := filepath.Join(baseDir, "datalake")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("datalake: open %s: %w", dir, err)
	}
	l := &Lake{dir: dir, colls: map[string]*collection{}, now: now}
	des, _ := os.ReadDir(dir)
	for _, de := range des {
		if !de.IsDir() {
			continue
		}
		c, err := l.loadCollection(de.Name())
		if err != nil {
			continue // skip an unreadable collection rather than fail the whole lake
		}
		l.colls[c.schema.Name] = c
	}
	return l, nil
}

func (l *Lake) loadCollection(name string) (*collection, error) {
	cdir := filepath.Join(l.dir, name)
	b, err := os.ReadFile(filepath.Join(cdir, "_schema.json"))
	if err != nil {
		return nil, err
	}
	var sc Schema
	if err := json.Unmarshal(b, &sc); err != nil || sc.Name == "" {
		return nil, fmt.Errorf("datalake: bad schema for %s", name)
	}
	c := &collection{schema: sc, records: map[string]Record{}}
	rdes, _ := os.ReadDir(filepath.Join(cdir, "rec"))
	for _, rde := range rdes {
		if rde.IsDir() || !strings.HasSuffix(rde.Name(), ".json") {
			continue
		}
		rb, err := os.ReadFile(filepath.Join(cdir, "rec", rde.Name()))
		if err != nil {
			continue
		}
		var r Record
		if json.Unmarshal(rb, &r) == nil && r.ID != "" {
			c.records[r.ID] = r
		}
	}
	return c, nil
}

// validName allows safe single path segments only.
func validName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 || name == "_schema" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// CreateCollection records a new collection. It errors if the name is taken