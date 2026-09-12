// SPDX-License-Identifier: MIT

// Lake schema management: CreateCollection + EnsureCollection + writeSchema + DropCollection + ListCollections + Schema + titleOf + canonicalDate + canonicalizeDateFields.
// Code extracted from datalake.go during the Day-68 god-file split. Public API unchanged.
package datalake


import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)


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

// wholeDate matches a value that is exactly ONE date and nothing else. The
// anchor matters: read-side helpers may truncate a timestamp to its day, but a
// write path that did the same would destroy the time component, so anything
// carrying a time or trailing text is deliberately left as written.
var wholeDate = regexp.MustCompile(`^(\d{4})[-/](\d{1,2})[-/](\d{1,2})$`)

// canonicalDate returns the zero-padded `YYYY-MM-DD` spelling of a value that
// is exactly one whole date, reporting whether it changed anything. A
// non-canonical but recognizable spelling ("2026-9-5", "2026/9/5") becomes
// canonical; everything else is returned untouched — an out-of-range date is
// never guessed at, and rejection is deliberately not on the menu because it
// would turn previously-accepted writes into errors.
func canonicalDate(s string) (string, bool) {
	m := wholeDate.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return s, false
	}
	mo, errMo := strconv.Atoi(m[2])
	d, errD := strconv.Atoi(m[3])
	if errMo != nil || errD != nil || mo < 1 || mo > 12 || d < 1 || d > 31 {
		return s, false
	}
	return fmt.Sprintf("%s-%02d-%02d", m[1], mo, d), true
}

// canonicalizeDateFields canonicalizes the string values of every field the
// schema DECLARES as a date. Declaration is the trigger because the schema is
// explicitly advisory — records may carry extra keys, and something that merely
// looks like a date in a text field is the operator's own content. It is
// copy-on-write: the caller's map is returned untouched unless a value actually
// changed, because Insert historically aliased the caller's map straight into
// the stored Record, and mutating a caller's data as a side effect is its own
// bug.
func canonicalizeDateFields(schema Schema, fields map[string]any) map[string]any {
	// out stays nil until a value actually changes (map-to-nil comparison is
	// legal, map-to-map is not), which is what makes this copy-on-write.
	var out map[string]any
	for _, f := range schema.Fields {
		if f.Type != "date" {
			continue
		}
		s, ok := fields[f.Name].(string)
		if !ok {
			continue
		}
		if c, changed := canonicalDate(s); changed {
			if out == nil {
				out = maps.Clone(fields)
			}
			out[f.Name] = c
		}
	}
	if out == nil {
		return fields
	}
	return out
}

// Insert adds a record. Fields are stored verbatim, with one deliberate
// exception: a value written to a field the schema declares as `date` is
// canonicalized to `YYYY-MM-DD` when it is exactly one whole date (see