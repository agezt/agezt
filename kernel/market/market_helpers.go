// SPDX-License-Identifier: MIT
//
// kernel/market query + sort helpers (matchesQuery, sortEntries).
// Extracted from market.go during Day 211 god-file refactor (#87).
// Public API unchanged.
package market

import (
	"sort"
	"strings"
)

func matchesQuery(e MarketplaceEntry, q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return true
	}
	hay := strings.ToLower(e.Name + " " + e.Description + " " + e.Category + " " + strings.Join(e.Tags, " "))
	for _, tok := range strings.Fields(q) {
		if !strings.Contains(hay, tok) {
			return false
		}
	}
	return true
}
func sortEntries(entries []MarketplaceEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Category != entries[j].Category {
			return entries[i].Category < entries[j].Category
		}
		return entries[i].Name < entries[j].Name
	})
}
