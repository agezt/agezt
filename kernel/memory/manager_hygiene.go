// SPDX-License-Identifier: MIT

// manager_hygiene.go owns the memory-maintenance surface:
// HygieneStats (summary type for `agt memory hygiene`) and
// the two scan-then-act methods (Hygiene / Prune). Prune is
// the ONLY destructive memory op — by construction it only
// removes records already soft-deleted and aged out. The
// read + write surface lives in manager_recall.go.
package memory

import (

	"github.com/agezt/agezt/kernel/event"
)

// HygieneStats summarizes the store's health for the maintenance view (M857).
type HygieneStats struct {
	Total      int `json:"total"`
	Active     int `json:"active"`
	Tombstoned int `json:"tombstoned"`
	Superseded int `json:"superseded"`
	Suspended  int `json:"suspended"`
	Expired    int `json:"expired"`
	// Prunable is how many soft-deleted (tombstoned or superseded) records are
	// older than the given cutoff — the dead weight a Prune would reclaim.
	Prunable int `json:"prunable"`
}

// Hygiene reports store health, counting how many soft-deleted records are older
// than olderThanMs (the prune candidates). olderThanMs <= 0 counts all
// soft-deleted records as prunable.
func (m *Manager) Hygiene(olderThanMs int64) (HygieneStats, error) {
	all, err := m.store.All()
	if err != nil {
		return HygieneStats{}, err
	}
	var st HygieneStats
	st.Total = len(all)
	for _, r := range all {
		switch {
		case r.Tombstoned:
			st.Tombstoned++
		case r.SupersededBy != "":
			st.Superseded++
		case r.Suspended():
			st.Suspended++
		case r.Expired(m.now().UnixMilli()):
			st.Expired++
		default:
			st.Active++
		}
		if (r.Tombstoned || r.SupersededBy != "") && (olderThanMs <= 0 || r.LastSeenMS < olderThanMs) {
			st.Prunable++
		}
	}
	return st, nil
}

// Prune hard-removes soft-deleted records (tombstoned or superseded) whose last
// activity predates olderThanMs — reclaiming the dead weight that consolidation
// and forgets leave behind, so memory can't grow without bound ("no memory-bomb",
// M857). Active records are never touched. With dryRun, nothing is deleted and
// the count of candidates is returned. The deletion is journaled (one
// memory.pruned event) and is the ONLY destructive memory op — by construction it
// only removes records already soft-deleted and aged out.
func (m *Manager) Prune(corr string, olderThanMs int64, dryRun bool) (int, error) {
	all, err := m.store.All()
	if err != nil {
		return 0, err
	}
	var victims []string
	for _, r := range all {
		if (r.Tombstoned || r.SupersededBy != "") && r.LastSeenMS < olderThanMs {
			victims = append(victims, r.ID)
		}
	}
	if dryRun || len(victims) == 0 {
		return len(victims), nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	pruned := 0
	for _, id := range victims {
		if ok, derr := m.store.Delete(id); derr != nil {
			return pruned, derr
		} else if ok {
			pruned++
		}
	}
	m.publish(event.KindMemoryPruned, corr, map[string]any{"pruned": pruned})
	return pruned, nil
}

// Supersede replaces an existing record with a new one, linking the old
// record's SupersededBy to the new id (soft update — the old record is
// retained). Returns the new record. If oldID is unknown, the new record is
