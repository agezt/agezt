// SPDX-License-Identifier: MIT

package catalog

// Provenance: Catalog merge + serialization: ParseAPIFile. The merge and
//             the marshal rules moved to the contract layer with the row they
//             are about (W3.51) — the store that writes the catalog's files
//             is above this module and may not name it. Code extracted from
//             types.go during the Day-57 god-file split. Public API unchanged.

import (
	"encoding/json"
	"fmt"

	"github.com/agezt/agezt/kernel/contract/catalogapi"
)

// ParseAPIFile parses a models.dev-shaped api.json into a Catalog.
// Returns an error if the JSON is malformed or has the wrong shape.
//
// The parsing stays here — it is this module's FILE FORMAT — but the row it
// builds is the contract's (W3.51).
func ParseAPIFile(raw []byte) (*Catalog, error) {
	var byID map[string]*Provider
	if err := json.Unmarshal(raw, &byID); err != nil {
		return nil, fmt.Errorf("catalog: parse api.json: %w", err)
	}
	c := catalogapi.NewEmpty()
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
