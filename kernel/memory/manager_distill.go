// SPDX-License-Identifier: MIT

// Memory distillation: Distill, distillKey, normalizeEvidence, defaultHalfLifeMS, contradictionTrackedType, contradictionKey, normalize helpers, DedupeDistilled, strongerNote, parseDistill.
// Code extracted from manager.go during the Day-44 god-file split. Public API unchanged.
package memory


import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)


func (m *Manager) Distill(ctx context.Context, corr string, provider agent.Provider, model, intent, transcript string) ([]string, error) {
	if provider == nil {
		return nil, errors.New("memory: distill requires a provider")
	}
	user := fmt.Sprintf("Task intent:\n%s\n\nWhat happened:\n%s", intent, transcript)
	resp, err := provider.Complete(ctx, agent.CompletionRequest{
		Model:    model,
		System:   distillSystem,
		Messages: []agent.Message{{Role: agent.RoleUser, Content: user}},
		TaskType: "distill",
	})
	if err != nil {
		return nil, fmt.Errorf("memory: distill completion: %w", err)
	}
	parsed, ok := parseDistill(resp.Message.Content)
	if !ok {
		// Non-JSON answer (e.g. the mock provider) → nothing to store. Not
		// an error; distillation is opportunistic.
		return nil, nil
	}
	// A named agent's distilled facts stay its private notes (M915): the run
	// ctx carries the agent's scope, so per-run distillation doesn't flood the
	// shared brain. An unscoped run (operator chat) distills shared, as before.
	// Promotion or consolidation can share the keepers later.
	scope := ScopeFrom(ctx)
	tags := map[string]string{"source": "distill"}
	if scope != "" {
		tags["scope"] = scope
	}

	// Subject-level dedupe (M993): auto-distillation fires after most multi-tool
	// runs, so without this the SAME topic gets re-extracted run after run with
	// slightly reworded content — each a new content-hash, so they pile up into
	// thousands of near-duplicate notes ("her işlemde memory'ye bir şey ekliyor").
	// Index the existing active records by (type, normalized-subject, scope); when
	// a distilled fact lands on a subject we already hold in this scope, REINFORCE
	// the existing record (bump recency/confidence) instead of creating another.
	// New subjects are still recorded. The explicit `memory` tool is unaffected —
	// only opportunistic distillation is gated, so deliberate writes still stand.
	index := map[string]Record{}
	if active, err := m.Active(); err == nil {
		for _, r := range active {
			if r.Tags["source"] != "distill" {
				continue // only collapse onto prior distilled notes, not curated ones
			}
			index[distillKey(r.Type, r.Subject, scopeOf(r.Tags))] = r
		}
	}

	var ids []string
	for _, f := range parsed.Facts {
		if strings.TrimSpace(f.Content) == "" {
			continue
		}
		t := f.Type
		if !ValidType(t) {
			t = TypeSummary
		}
		key := distillKey(t, f.Subject, scope)
		if ex, ok := index[key]; ok {
			// Already noted this subject in this scope — reinforce the existing
			// record rather than adding a near-duplicate.
			if _, _, err := m.Remember(corr, RememberSpec{Type: ex.Type, Subject: ex.Subject, Content: ex.Content, Actor: "distill", Tags: ex.Tags}); err != nil {
				return ids, err
			}
			ids = append(ids, ex.ID)
			continue
		}
		rec, _, err := m.Remember(corr, RememberSpec{
			Type:    t,
			Subject: f.Subject,
			Content: f.Content,
			Actor:   "distill",
			Tags:    tags,
		})
		if err != nil {
			return ids, err
		}
		// Record it so two facts about the same new subject in one pass also collapse.
		index[key] = rec
		ids = append(ids, rec.ID)
	}
	return ids, nil
}

// distillKey is the subject-level identity used to collapse repeated
// auto-distilled notes: type + normalized subject + scope. Subject is lowercased
// and whitespace-collapsed so "Project structure" and "project  structure" map
// together.
func distillKey(t Type, subject, scope string) string {
	return string(t) + "\x00" + strings.Join(strings.Fields(strings.ToLower(subject)), " ") + "\x00" + scope
}

func normalizeEvidence(e Evidence, tags map[string]string, t Type) Evidence {
	switch e {
	case EvidenceObserved, EvidenceInferred, EvidenceCurated, EvidenceConstraint:
		return e
	}
	if tags != nil {
		switch tags["source"] {
		case "operator":
			return EvidenceCurated
		case "distill", "brain-distill", "agent":
			return EvidenceInferred
		}
	}
	if t == TypeObservation {
		return EvidenceObserved
	}
	return EvidenceInferred
}

func defaultHalfLifeMS(t Type, e Evidence) int64 {
	const dayMS = int64(24 * time.Hour / time.Millisecond)
	switch e {
	case EvidenceConstraint:
		return 3650 * dayMS
	case EvidenceCurated:
		return 180 * dayMS
	case EvidenceObserved:
		return 30 * dayMS
	}
	switch t {
	case TypePreference:
		return 180 * dayMS
	case TypeSummary:
		return 90 * dayMS
	case TypeObservation:
		return 30 * dayMS
	default:
		return 60 * dayMS
	}
}

func contradictionTrackedType(t Type) bool {
	switch t {
	case TypeFact, TypePreference, TypeRelation, TypeObservation:
		return true
	default:
		return false
	}
}

func contradictionKey(r Record) string {
	return string(r.Type) + "\x00" + normalizeTopic(r.Subject) + "\x00" + scopeOf(r.Tags)
}

func normalizeTopic(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func normalizeContent(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func sameNormalizedContent(rs []Record) bool {
	if len(rs) < 2 {
		return true
	}
	first := normalizeContent(rs[0].Content)
	for _, r := range rs[1:] {
		if normalizeContent(r.Content) != first {
			return false
		}
	}
	return true
}

// DedupeDistilled retroactively collapses the near-duplicate auto-distilled notes
// that accumulated before the write-time subject gate (M993) existed — the "1000+
// nonsense entries" the owner saw. It groups active source=distill records by
// distillKey and, for each group with more than one, keeps the strongest note
// (highest confidence, then most-recently-seen) and FORGETS the rest (soft
// tombstone — reversible, and prunable later). Curated memories (the explicit
// `memory` tool, source≠distill) are never touched. With dryRun it only reports
// how many would be collapsed. Returns the number removed (or that would be).
func (m *Manager) DedupeDistilled(corr string, dryRun bool) (int, error) {
	active, err := m.Active()
	if err != nil {
		return 0, err
	}
	groups := map[string][]Record{}
	for _, r := range active {
		if r.Tags["source"] != "distill" {
			continue
		}
		k := distillKey(r.Type, r.Subject, scopeOf(r.Tags))
		groups[k] = append(groups[k], r)
	}
	collapsed := 0
	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		keep := 0
		for i := 1; i < len(g); i++ {
			if strongerNote(g[i], g[keep]) {
				keep = i
			}
		}
		for i := range g {
			if i == keep {
				continue
			}
			if dryRun {
				collapsed++
				continue
			}
			ok, ferr := m.Forget(corr, g[i].ID)
			if ferr != nil {
				return collapsed, ferr
			}
			if ok {
				collapsed++
			}
		}
	}
	return collapsed, nil
}

// strongerNote ranks two same-subject distilled notes for which to keep: higher
// confidence wins (it was reinforced more), then the most-recently-seen.
func strongerNote(a, b Record) bool {
	if a.Confidence != b.Confidence {
		return a.Confidence > b.Confidence
	}
	return a.LastSeenMS > b.LastSeenMS
}

// parseDistill extracts the JSON object from a model response, tolerating
// surrounding prose or markdown fences by scanning for the outermost braces.
func parseDistill(s string) (distillResult, bool) {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return distillResult{}, false
	}
	var r distillResult
	if err := json.Unmarshal([]byte(s[start:end+1]), &r); err != nil {
		return distillResult{}, false
	}
	return r, true
}
