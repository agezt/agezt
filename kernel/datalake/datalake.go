// SPDX-License-Identifier: MIT

// Package datalake is AGEZT's file-based structured store — the "Personal Data
// Lake" (M834). It gives agents (and the operator, via the Web UI) real
// databases without a database: a Lake holds named COLLECTIONS (tables), each a
// set of JSON RECORDS with an optional SCHEMA describing its fields and how the
// UI should render it (a generic table, or a bespoke app view like an expense
// tracker or calendar).
//
// It is deliberately dependency-free and on-disk, matching AGEZT's single-static-
// binary / no-DB architecture: every collection is a directory, every record a
// JSON file, with an in-memory index loaded at Open and guarded by one mutex.
// Collections are shared across all agents on the daemon, so one agent can file
// data another (or the human in chat) later reads.
//
// Layout:
//
//	<base>/datalake/<collection>/_schema.json   — the collection's schema
//	<base>/datalake/<collection>/rec/<id>.json  — one record per file
package datalake

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/ulid"
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
// (ErrExists) or invalid. CreatedMs/CreatedBy are stamped here.
func (l *Lake) CreateCollection(sc Schema, actor string) (Schema, error) {
	sc.Name = strings.TrimSpace(sc.Name)
	if !validName(sc.Name) {
		return Schema{}, fmt.Errorf("datalake: invalid collection name %q (use letters, digits, - or _)", sc.Name)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.colls[sc.Name]; ok {
		return Schema{}, ErrExists
	}
	sc.CreatedMs = l.now()
	if sc.CreatedBy == "" {
		sc.CreatedBy = actor
	}
	if err := l.writeSchema(sc); err != nil {
		return Schema{}, err
	}
	l.colls[sc.Name] = &collection{schema: sc, records: map[string]Record{}}
	return sc, nil
}

// EnsureCollection creates the collection only if it does not exist yet, leaving
// an existing one (and its data) untouched. Used to seed the built-ins at boot.
func (l *Lake) EnsureCollection(sc Schema, actor string) (Schema, bool, error) {
	l.mu.Lock()
	if c, ok := l.colls[sc.Name]; ok {
		out := c.schema
		l.mu.Unlock()
		return out, false, nil
	}
	l.mu.Unlock()
	out, err := l.CreateCollection(sc, actor)
	if errors.Is(err, ErrExists) {
		// Raced with another caller; fetch the winner.
		if s, ok := l.Schema(sc.Name); ok {
			return s, false, nil
		}
	}
	return out, err == nil, err
}

func (l *Lake) writeSchema(sc Schema) error {
	cdir := filepath.Join(l.dir, sc.Name)
	if err := os.MkdirAll(filepath.Join(cdir, "rec"), 0o700); err != nil {
		return fmt.Errorf("datalake: mkdir %s: %w", cdir, err)
	}
	return writeJSON(filepath.Join(cdir, "_schema.json"), sc)
}

// DropCollection deletes a collection and all its records. System collections
// are protected (ErrSystem).
func (l *Lake) DropCollection(name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.colls[name]
	if !ok {
		return ErrNotFound
	}
	if c.schema.System {
		return ErrSystem
	}
	delete(l.colls, name)
	return os.RemoveAll(filepath.Join(l.dir, name))
}

// ListCollections returns every collection's schema + record count, sorted by
// title/name.
func (l *Lake) ListCollections() []CollectionInfo {
	l.mu.Lock()
	out := make([]CollectionInfo, 0, len(l.colls))
	for _, c := range l.colls {
		out = append(out, CollectionInfo{Schema: c.schema, Count: len(c.records)})
	}
	l.mu.Unlock()
	sort.Slice(out, func(a, b int) bool {
		ta, tb := titleOf(out[a].Schema), titleOf(out[b].Schema)
		if ta != tb {
			return ta < tb
		}
		return out[a].Name < out[b].Name
	})
	return out
}

func titleOf(s Schema) string {
	if s.Title != "" {
		return strings.ToLower(s.Title)
	}
	return strings.ToLower(s.Name)
}

// Schema returns a collection's schema.
func (l *Lake) Schema(name string) (Schema, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.colls[name]
	if !ok {
		return Schema{}, false
	}
	return c.schema, true
}

// Insert adds a record. Fields are stored verbatim; id/timestamps/provenance are
// stamped here.
func (l *Lake) Insert(coll string, fields map[string]any, actor string) (Record, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.colls[coll]
	if !ok {
		return Record{}, ErrNotFound
	}
	if fields == nil {
		fields = map[string]any{}
	}
	now := l.now()
	r := Record{
		ID:        "rec-" + ulid.New(),
		Fields:    fields,
		CreatedMs: now,
		UpdatedMs: now,
		CreatedBy: actor,
		UpdatedBy: actor,
	}
	if err := l.writeRecord(coll, r); err != nil {
		return Record{}, err
	}
	c.records[r.ID] = r
	return r, nil
}

// Get returns one record.
func (l *Lake) Get(coll, id string) (Record, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.colls[coll]
	if !ok {
		return Record{}, ErrNotFound
	}
	r, ok := c.records[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	return r, nil
}

// Update merges patch into an existing record's fields (a nil value deletes a
// key) and bumps UpdatedMs/UpdatedBy.
func (l *Lake) Update(coll, id string, patch map[string]any, actor string) (Record, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.colls[coll]
	if !ok {
		return Record{}, ErrNotFound
	}
	r, ok := c.records[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	merged := make(map[string]any, len(r.Fields)+len(patch))
	maps.Copy(merged, r.Fields)
	for k, v := range patch {
		if v == nil {
			delete(merged, k)
			continue
		}
		merged[k] = v
	}
	r.Fields = merged
	r.UpdatedMs = l.now()
	r.UpdatedBy = actor
	if err := l.writeRecord(coll, r); err != nil {
		return Record{}, err
	}
	c.records[id] = r
	return r, nil
}

// Delete removes one record.
func (l *Lake) Delete(coll, id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.colls[coll]
	if !ok {
		return ErrNotFound
	}
	if _, ok := c.records[id]; !ok {
		return ErrNotFound
	}
	delete(c.records, id)
	return os.Remove(filepath.Join(l.dir, coll, "rec", id+".json"))
}

// Query returns the matching records (a copy), filtered/sorted/paged.
func (l *Lake) Query(coll string, q Query) ([]Record, error) {
	l.mu.Lock()
	c, ok := l.colls[coll]
	if !ok {
		l.mu.Unlock()
		return nil, ErrNotFound
	}
	all := make([]Record, 0, len(c.records))
	for _, r := range c.records {
		all = append(all, r)
	}
	l.mu.Unlock()

	search := strings.ToLower(strings.TrimSpace(q.Search))
	out := make([]Record, 0, len(all))
	for _, r := range all {
		if !matchEquals(r, q.Equals) {
			continue
		}
		if search != "" && !matchSearch(r, search) {
			continue
		}
		out = append(out, r)
	}

	sort.Slice(out, func(a, b int) bool {
		less := lessRecords(out[a], out[b], q.SortBy)
		if q.Desc {
			return !less
		}
		return less
	})

	if q.Offset > 0 {
		if q.Offset >= len(out) {
			return []Record{}, nil
		}
		out = out[q.Offset:]
	}
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

// Count returns how many records a collection holds.
func (l *Lake) Count(coll string) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.colls[coll]
	if !ok {
		return 0, ErrNotFound
	}
	return len(c.records), nil
}

func matchEquals(r Record, eq map[string]any) bool {
	for k, want := range eq {
		got, ok := r.Fields[k]
		if !ok || !jsonEqual(got, want) {
			return false
		}
	}
	return true
}

func matchSearch(r Record, lowerNeedle string) bool {
	for _, v := range r.Fields {
		if s, ok := v.(string); ok && strings.Contains(strings.ToLower(s), lowerNeedle) {
			return true
		}
	}
	return false
}

// lessRecords orders by the named field when set (numbers numerically, else
// string-wise), falling back to created time (newest first by default — callers
// flip with Desc).
func lessRecords(a, b Record, sortBy string) bool {
	if sortBy == "" {
		// Default: newest first under ascending order is unintuitive, so the
		// created-time default sorts NEWEST first (most-recent on top) — the
		// common "show me my latest entries" case. Desc flips it.
		return a.CreatedMs > b.CreatedMs
	}
	av, aok := a.Fields[sortBy]
	bv, bok := b.Fields[sortBy]
	if !aok || !bok {
		// Missing keys sort last (after present ones).
		if aok != bok {
			return aok
		}
		return a.CreatedMs > b.CreatedMs
	}
	an, aNum := toFloat(av)
	bn, bNum := toFloat(bv)
	if aNum && bNum {
		return an < bn
	}
	return fmt.Sprintf("%v", av) < fmt.Sprintf("%v", bv)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func jsonEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

func (l *Lake) writeRecord(coll string, r Record) error {
	return writeJSON(filepath.Join(l.dir, coll, "rec", r.ID+".json"), r)
}

// writeJSON writes v as indented JSON atomically (temp + rename).
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("datalake: write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("datalake: write %s: %w", path, err)
	}
	return nil
}
