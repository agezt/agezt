// SPDX-License-Identifier: MIT

// Package providerboot: liveEligible map + eligibleSet — the cross-provider
// down-route eligibility set, threaded through Boot + Reload so the set is
// rebuilt on every reload (not frozen at boot — the 2026-08 live-drift survey
// caught this). has + set are the two operations on it. Extracted from
// providerboot.go during the Day-211 god-file split. Public API unchanged.
package providerboot


import (
	"sync"
)
type eligibleSet struct {
	mu sync.RWMutex
	m  map[string]bool
}

func (s *eligibleSet) has(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.m[id]
}

func (s *eligibleSet) set(m map[string]bool) {
	s.mu.Lock()
	s.m = m
	s.mu.Unlock()
}
// liveEligible maps a governor's *Registry to its eligibleSet so Reload —
// which only receives the *governor.Governor — can refresh the set Boot's
// altFinder closure reads. Entries live as long as the process (one governor
// per daemon; test governors leak a map entry each, which is fine).
var liveEligible sync.Map // *governor.Registry → *eligibleSet
