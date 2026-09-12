// SPDX-License-Identifier: MIT

// Lake record operations: Insert + Get + Update + Delete + Query + Count + matchEquals + matchSearch + lessRecords + toFloat + jsonEqual + writeRecord + writeJSON.
// Code extracted from datalake.go during the Day-68 god-file split. Public API unchanged.
package datalake


import (
	"encoding/json"
	"fmt"
	"github.com/agezt/agezt/internal/atomicfile"
	"github.com/agezt/agezt/kernel/ulid"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
)


// canonicalizeDateFields). id/timestamps/provenance are stamped here.
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
	fields = canonicalizeDateFields(c.schema, fields)
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
// key) and bumps UpdatedMs/UpdatedBy. Only the keys being written pass through
// date canonicalization (see canonicalizeDateFields) — an unrelated edit must
// not quietly rewrite a stored value the operator never touched.
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
	patch = canonicalizeDateFields(c.schema, patch)
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
	ab, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bb, err := json.Marshal(b)
	if err != nil {
		return false
	}
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
	if err := atomicfile.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("datalake: write %s: %w", path, err)
	}
	return nil
}
