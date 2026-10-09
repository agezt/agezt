// SPDX-License-Identifier: MIT

package journal

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"

	"github.com/agezt/agezt/kernel/event"
)

const (
	defaultChangelogLimit = 20
	maxChangelogLimit     = 1_000
)

// changelogKinds maps each material-change event kind to a stable human label.
// Membership in this map is what makes an event part of the system changelog.
var changelogKinds = map[event.Kind]string{
	event.KindHalt:                      "system HALTED",
	event.KindAnomalyDetected:           "anomaly auto-halt",
	event.KindResume:                    "system resumed",
	event.KindPolicyChanged:             "policy changed",
	event.KindSkillCreated:              "skill created",
	event.KindSkillPromoted:             "skill promoted",
	event.KindSkillQuarantined:          "skill quarantined",
	event.KindSkillReverted:             "skill reverted",
	event.KindSkillRestored:             "skill restored",
	event.KindWorkflowRestored:          "workflow restored",
	event.KindReflectionCompleted:       "reflection completed",
	event.KindCatalogSynced:             "model catalog synced",
	event.KindCatalogSyncFailed:         "model catalog sync FAILED",
	event.KindCatalogDiscoveryCompleted: "provider discovery completed",
	event.KindCatalogDiscoveryFailed:    "provider discovery FAILED",
	event.KindPulsePaused:               "pulse paused",
	event.KindPulseResumed:              "pulse resumed",
}

// Cost prices a call with every input token at the full input rate.
type Cost func(model string, inputTokens, outputTokens int) int64

// WithCost sets the no-cache baseline pricing the cache statistics compare with.
func (s *Service) WithCost(cost Cost) *Service {
	s.cost = cost
	return s
}

type WindowRequest struct {
	Limit   json.RawMessage `json:"limit,omitempty"`
	SinceMS json.RawMessage `json:"since_ms,omitempty"`
}

type ChangelogEntry struct {
	TSUnixMS      int64  `json:"ts_unix_ms"`
	Kind          string `json:"kind"`
	Label         string `json:"label"`
	Detail        string `json:"detail"`
	EventID       string `json:"event_id"`
	CorrelationID string `json:"correlation_id"`
}

type ChangelogOutput struct {
	Entries []ChangelogEntry `json:"entries"`
	Count   int              `json:"count"`
}

type CacheStatsOutput struct {
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	CacheWriteInputTokens int64 `json:"cache_write_input_tokens"`
	SavedMicrocents       int64 `json:"saved_microcents"`
	Calls                 int64 `json:"calls"`
	WindowMS              int64 `json:"window_ms"`
}

// lenientInt64 reads a JSON number truncated toward zero; anything else is 0.
func lenientInt64(raw json.RawMessage) int64 {
	n, _ := rawValue(raw).(float64)
	return int64(n)
}

// cutoff is now minus a positive since_ms, or 0 for no window.
func (s *Service) cutoff(raw json.RawMessage) int64 {
	if since := lenientInt64(raw); since > 0 {
		return s.now().UnixMilli() - since
	}
	return 0
}

// trimFloat renders a JSON number without a trailing ".0" for whole values.
func trimFloat(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	b, _ := json.Marshal(f)
	return string(b)
}

// changelogDetail probes a few common payload keys for a short human detail,
// so the timeline reads meaningfully without per-kind decoding.
func changelogDetail(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p map[string]any
	if json.Unmarshal(payload, &p) != nil {
		return ""
	}
	for _, key := range []string{"summary", "name", "skill_id", "id", "rule", "change", "reason", "subject", "provider", "model", "count"} {
		switch v := p[key].(type) {
		case string:
			if v != "" {
				return v
			}
		case float64:
			return trimFloat(v)
		}
	}
	return ""
}

// Changelog is the journal filtered to material system changes, newest first
// (journal seq breaks a same-millisecond tie), each with its event id so it can
// be proven and explained.
func (s *Service) Changelog(_ context.Context, in WindowRequest) (ChangelogOutput, error) {
	limit := defaultChangelogLimit
	if n := int(lenientInt64(in.Limit)); n > 0 {
		limit = n
	}
	limit = min(limit, maxChangelogLimit)
	cutoff := s.cutoff(in.SinceMS)
	type row struct {
		entry ChangelogEntry
		seq   int64
	}
	rows := make([]row, 0)
	if err := s.journal.Range(func(e *event.Event) error {
		label, ok := changelogKinds[e.Kind]
		if !ok || cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		rows = append(rows, row{ChangelogEntry{TSUnixMS: e.TSUnixMS, Kind: string(e.Kind), Label: label, Detail: changelogDetail(e.Payload), EventID: e.ID, CorrelationID: e.CorrelationID}, e.Seq})
		return nil
	}); err != nil {
		return ChangelogOutput{}, err
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].entry.TSUnixMS != rows[j].entry.TSUnixMS {
			return rows[i].entry.TSUnixMS > rows[j].entry.TSUnixMS
		}
		return rows[i].seq > rows[j].seq
	})
	rows = rows[:min(len(rows), limit)]
	out := ChangelogOutput{Entries: make([]ChangelogEntry, 0, len(rows)), Count: len(rows)}
	for _, r := range rows {
		out.Entries = append(out.Entries, r.entry)
	}
	return out, nil
}

// CacheStats folds the budget.consumed events into prompt-cache reads, writes
// and the saving versus the full input rate, floored at zero per call.
// Malformed payloads are skipped rather than aborting the fold.
func (s *Service) CacheStats(_ context.Context, in WindowRequest) (CacheStatsOutput, error) {
	cutoff := s.cutoff(in.SinceMS)
	out := CacheStatsOutput{WindowMS: lenientInt64(in.SinceMS)}
	if err := s.journal.Range(func(e *event.Event) error {
		if e.Kind != event.KindBudgetConsumed || cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		var p struct {
			Model                 string `json:"model"`
			InputTokens           int    `json:"input_tokens"`
			OutputTokens          int    `json:"output_tokens"`
			CachedInputTokens     int    `json:"cached_input_tokens"`
			CacheWriteInputTokens int    `json:"cache_write_input_tokens"`
			CostMicrocents        int64  `json:"cost_microcents"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return nil
		}
		out.Calls++
		out.CachedInputTokens += int64(p.CachedInputTokens)
		out.CacheWriteInputTokens += int64(p.CacheWriteInputTokens)
		if d := s.cost(p.Model, p.InputTokens, p.OutputTokens) - p.CostMicrocents; d > 0 {
			out.SavedMicrocents += d
		}
		return nil
	}); err != nil {
		return CacheStatsOutput{}, err
	}
	return out, nil
}
