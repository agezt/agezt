// SPDX-License-Identifier: MIT

// Memory housekeeping: Suspend, Audit, CleanLowValue, publish helper, clampConf.
// Code extracted from manager.go during the Day-44 god-file split. Public API unchanged.
package memory


import (
	"strings"

	"github.com/agezt/agezt/kernel/event"
)


func (m *Manager) Suspend(corr, id, reason string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, found, err := m.store.Get(id)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	nowMS := m.now().UnixMilli()
	rec.SuspendedMS = nowMS
	rec.SuspendedReason = strings.TrimSpace(reason)
	rec.LastSeenMS = nowMS
	if err := m.store.Put(rec); err != nil {
		return false, err
	}
	m.publish(event.KindMemorySuspended, corr, map[string]any{
		"id":      id,
		"subject": rec.Subject,
		"reason":  rec.SuspendedReason,
	})
	return true, nil
}

// AuditReport is the memory hygiene view used to detect stale or competing
// memories before they contaminate retrieval.
type AuditReport struct {
	Total             int                  `json:"total"`
	Usable            int                  `json:"usable"`
	Suspended         int                  `json:"suspended"`
	Expired           int                  `json:"expired"`
	ContradictionLoad int                  `json:"contradiction_load"`
	ExpiredIDs        []string             `json:"expired_ids,omitempty"`
	SuspendedIDs      []string             `json:"suspended_ids,omitempty"`
	Contradictions    []ContradictionGroup `json:"contradictions,omitempty"`
}

// CleanReport summarizes low-value retention cleanup. Removed means hard-deleted:
// clean targets records that never belonged in memory, not records worth a
// reversible tombstone.
type CleanReport struct {
	DryRun      bool               `json:"dry_run"`
	HardDeleted bool               `json:"hard_deleted"`
	Scanned     int                `json:"scanned"`
	Rejected    int                `json:"rejected"`
	Removed     int                `json:"removed"`
	Decisions   []CleanDecisionRow `json:"decisions,omitempty"`
}

type CleanDecisionRow struct {
	ID      string `json:"id"`
	Subject string `json:"subject,omitempty"`
	Reason  string `json:"reason"`
}

type ContradictionGroup struct {
	Key     string   `json:"key"`
	IDs     []string `json:"ids"`
	Subject string   `json:"subject,omitempty"`
	Type    Type     `json:"type,omitempty"`
	Scope   string   `json:"scope,omitempty"`
}

// Audit finds records that are barred by expiration/suspension and same-topic
// active records that compete with different content. It does not claim which
// record is true; it only exposes the contradiction load.
func (m *Manager) Audit() (AuditReport, error) {
	all, err := m.store.All()
	if err != nil {
		return AuditReport{}, err
	}
	nowMS := m.now().UnixMilli()
	report := AuditReport{Total: len(all)}
	byKey := map[string][]Record{}
	for _, r := range all {
		if !r.Active() {
			continue
		}
		switch {
		case r.Suspended():
			report.Suspended++
			report.SuspendedIDs = append(report.SuspendedIDs, r.ID)
			continue
		case r.Expired(nowMS):
			report.Expired++
			report.ExpiredIDs = append(report.ExpiredIDs, r.ID)
			continue
		default:
			report.Usable++
		}
		if contradictionTrackedType(r.Type) {
			byKey[contradictionKey(r)] = append(byKey[contradictionKey(r)], r)
		}
	}
	for key, rs := range byKey {
		if len(rs) < 2 || sameNormalizedContent(rs) {
			continue
		}
		ids := make([]string, 0, len(rs))
		for _, r := range rs {
			ids = append(ids, r.ID)
		}
		report.Contradictions = append(report.Contradictions, ContradictionGroup{
			Key:     key,
			IDs:     ids,
			Subject: rs[0].Subject,
			Type:    rs[0].Type,
			Scope:   scopeOf(rs[0].Tags),
		})
		report.ContradictionLoad += len(rs) - 1
	}
	return report, nil
}

// CleanLowValue applies the retention filter to the store and hard-deletes
// records that do not belong in memory at all: execution logs, transient sweep
// notes, and automatic low-value records. This is deliberately stricter than
// Forget/Prune: clean is the "this was never memory" path. Dry-run is the
// default at the control-plane/CLI layer; execute mode reclaims the rows
// immediately.
func (m *Manager) CleanLowValue(corr string, dryRun bool) (CleanReport, error) {
	all, err := m.store.All()
	if err != nil {
		return CleanReport{}, err
	}
	report := CleanReport{DryRun: dryRun, HardDeleted: !dryRun, Scanned: len(all)}
	var victims []string
	for _, r := range all {
		if sourceOf(r.Tags) == "operator" || r.AddedBy == "operator" || r.UpdatedBy == "operator" ||
			r.Evidence == EvidenceCurated || r.Evidence == EvidenceConstraint || r.Type == TypePreference {
			continue
		}
		decision := AssessRetention(r)
		if decision.Keep {
			continue
		}
		report.Rejected++
		report.Decisions = append(report.Decisions, CleanDecisionRow{ID: r.ID, Subject: r.Subject, Reason: decision.Reason})
		if dryRun {
			continue
		}
		victims = append(victims, r.ID)
	}
	if !dryRun && len(victims) > 0 {
		m.mu.Lock()
		for _, id := range victims {
			ok, derr := m.store.Delete(id)
			if derr != nil {
				m.mu.Unlock()
				return report, derr
			}
			if ok {
				report.Removed++
			}
		}
		m.mu.Unlock()
	}
	if !dryRun && report.Removed > 0 {
		m.publish(event.KindMemoryCleaned, corr, map[string]any{
			"scanned":      report.Scanned,
			"rejected":     report.Rejected,
			"removed":      report.Removed,
			"hard_deleted": true,
		})
	}
	return report, nil
}

// publish writes one event through the bus, returning the persisted event (or
// nil when no bus is wired, e.g. store-only tests). Subject groups memory
// events under "memory.<suffix>" so subscribers can scope-filter.
func (m *Manager) publish(kind event.Kind, corr string, payload any) *event.Event {
	if m.bus == nil {
		return nil
	}
	suffix := strings.TrimPrefix(string(kind), "memory.")
	ev, _ := m.bus.Publish(event.Spec{
		Subject:       "memory." + suffix,
		Kind:          kind,
		Actor:         "memory",
		CorrelationID: corr,
		Payload:       payload,
	})
	return ev
}

func clampConf(c float64) float64 {
	if c < 0 {
		return 0
	}
	if c > 1 {
		return 1
	}
	return c
}

// --- run-time context plumbing -------------------------------------------

type ctxKey int

const ctxKeyCorrelation ctxKey = iota

// WithCorrelation returns a child context carrying corr so the in-process
// memory Tool can journal its writes under the originating run. The runtime
// sets this on every run's context; without it the tool falls back to an
// empty correlation (still journaled, just not linked).