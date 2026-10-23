// SPDX-License-Identifier: MIT

package datalake

// Provenance: Lake core: Open + loadCollection + validName. The rows moved to
//             the contract layer (kernel/contract/datalakeapi) in W3.30 — the
//             module keeps their names as aliases (see aliases.go).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// collection is the lake's private per-collection state.
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
