// SPDX-License-Identifier: MIT

// Catalog merge + serialization: Merge, ParseAPIFile, MarshalAPI.
// Code extracted from types.go during the Day-57 god-file split. Public API unchanged.
package catalog


import (
	"encoding/json"
	"fmt"
)


func (dst *Catalog) Merge(src *Catalog) {
	for id, sp := range src.Providers {
		if existing, ok := dst.Providers[id]; ok {
			// Provider exists; merge model maps and prefer src for
			// non-model fields if they're populated.
			if sp.Name != "" {
				existing.Name = sp.Name
			}
			if len(sp.Env) > 0 {
				existing.Env = sp.Env
			}
			if sp.NPM != "" {
				existing.NPM = sp.NPM
			}
			if sp.API != "" {
				existing.API = sp.API
			}
			if sp.Doc != "" {
				existing.Doc = sp.Doc
			}
			if existing.Models == nil {
				existing.Models = map[string]*Model{}
			}
			for mid, m := range sp.Models {
				existing.Models[mid] = m
			}
		} else {
			// Copy so callers can't mutate src.
			cp := *sp
			cp.Models = map[string]*Model{}
			for mid, m := range sp.Models {
				cp.Models[mid] = m
			}
			dst.Providers[id] = &cp
		}
	}
}

// ParseAPIFile parses a models.dev-shaped api.json into a Catalog.
// Returns an error if the JSON is malformed or has the wrong shape.
func ParseAPIFile(raw []byte) (*Catalog, error) {
	var byID map[string]*Provider
	if err := json.Unmarshal(raw, &byID); err != nil {
		return nil, fmt.Errorf("catalog: parse api.json: %w", err)
	}
	c := NewEmpty()
	for id, p := range byID {
		if p == nil {
			continue
		}
		if p.ID == "" {
			p.ID = id
		}
		c.Providers[p.ID] = p
	}
	return c, nil
}

// MarshalAPI returns the JSON form (models.dev shape) for the catalog,
// suitable for writing back to disk.
func (c *Catalog) MarshalAPI() ([]byte, error) {
	return json.MarshalIndent(c.Providers, "", "  ")
}
