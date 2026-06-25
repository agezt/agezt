// SPDX-License-Identifier: MIT

package memory

// Operator-profile distillation (M1000, the "knowing you" Jarvis pillar). The
// per-run auto-distiller (Distill) extracts TASK facts; brain consolidation
// (DistillBrain) merges duplicates. Neither ever models the OPERATOR — who they
// are, how they like to work. DistillProfile is the missing counterpart: a
// cross-task pass that reads the accumulated shared-scope knowledge and
// synthesizes a concise profile of the operator (preferences, communication
// style, expertise, recurring people/projects, current focus).
//
// It reuses the ordinary Record machinery: each facet is a TypePreference record
// on a STABLE reserved subject ("operator profile: <facet>") in SHARED scope, so
// dedupe reinforces/updates a facet across passes (never duplicates), and the
// existing Memory UI/audit/half-life all apply for free. CleanLowValue already
// protects PREFERENCE records, so the profile survives hygiene passes. The
// profile is injected into every (non-system) run so the assistant always knows
// who it works for — distinct from per-agent persona.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)

// profileSubjectPrefix namespaces the operator-profile facets. Stable subjects
// let Remember reinforce/update a facet across passes instead of duplicating.
const profileSubjectPrefix = "operator profile: "

// maxProfileInput bounds the synthesis prompt — the most recent N shared records
// carry the freshest signal; older knowledge already shaped earlier passes.
const maxProfileInput = 60

// profileFacets is the FIXED facet set the synthesizer fills (omitting any with
// no signal). Fixed so subjects stay stable for dedupe across passes.
var profileFacets = []string{
	"preferences",
	"communication style",
	"expertise",
	"people and projects",
	"current focus",
}

func validFacet(f string) bool {
	for _, x := range profileFacets {
		if x == f {
			return true
		}
	}
	return false
}

const profileSystem = `You build a concise profile of the OPERATOR — the human this assistant works for — from accumulated memory about their work. ` +
	`Synthesize durable traits of the PERSON, not facts about any single task. ` +
	`Return ONLY a JSON object: {"facets":[{"facet":"<facet>","content":"..."}]} where facet is one of exactly: ` +
	`preferences, communication style, expertise, people and projects, current focus. ` +
	`Omit any facet with no real signal. Each content is 1-3 sentences, stands alone (no references to "the records above"), ` +
	`and is written as durable knowledge about the operator. Be specific and concise; do not invent traits that aren't evidenced.`

type profileFacetResult struct {
	Facet   string `json:"facet"`
	Content string `json:"content"`
}

type profileResult struct {
	Facets []profileFacetResult `json:"facets"`
}

// ProfileReport summarizes one profile-distillation pass.
type ProfileReport struct {
	InputRecords  int      `json:"input_records"`
	FacetsWritten int      `json:"facets_written"`
	Facets        []string `json:"facets,omitempty"`
}

// DistillProfile synthesizes the operator profile from accumulated shared-scope
// memory and writes each facet as a reinforced TypePreference record. Best-effort
// like the other distillers: a provider transport error propagates; an unusable
// answer is a no-op pass. Returns a no-op report when there's nothing to learn
// from yet.
func (m *Manager) DistillProfile(ctx context.Context, corr string, provider agent.Provider, model string) (ProfileReport, error) {
	if provider == nil {
		return ProfileReport{}, errors.New("memory: profile distill requires a provider")
	}
	active, err := m.Active()
	if err != nil {
		return ProfileReport{}, err
	}
	// Input = shared-scope records that aren't themselves profile facets. The
	// operator profile is global, synthesized from shared knowledge; private
	// agent-scoped notes are agent-specific and excluded. Existing facets are fed
	// separately as the current profile to refine.
	var input, current []Record
	for _, r := range active {
		if r.Tags["scope"] != "" {
			continue // private scope — not operator-global signal
		}
		if strings.HasPrefix(r.Subject, profileSubjectPrefix) {
			current = append(current, r)
			continue
		}
		input = append(input, r)
	}
	report := ProfileReport{InputRecords: len(input)}
	if len(input) == 0 {
		return report, nil // nothing to learn from yet — no-op
	}
	sortRecords(input)
	if len(input) > maxProfileInput {
		input = input[len(input)-maxProfileInput:] // freshest signal
	}

	var b strings.Builder
	for _, r := range input {
		fmt.Fprintf(&b, "- [%s] %s: %s\n", r.Type, r.Subject, r.Content)
	}
	user := "Accumulated memory about the operator's work:\n" + b.String()
	if len(current) > 0 {
		sortRecords(current)
		var cur strings.Builder
		for _, r := range current {
			fmt.Fprintf(&cur, "- %s: %s\n", strings.TrimPrefix(r.Subject, profileSubjectPrefix), r.Content)
		}
		user += "\nCurrent operator profile (update and refine this):\n" + cur.String()
	}

	var parsed profileResult
	if _, err := agent.GenerateObject(ctx, provider, agent.CompletionRequest{
		Model:    model,
		System:   profileSystem,
		Messages: []agent.Message{{Role: agent.RoleUser, Content: user}},
		TaskType: "distill", // same budgeting/routing class as the other distillers
	}, nil, &parsed); err != nil {
		if errors.Is(err, agent.ErrNoObjectGenerated) {
			return report, nil
		}
		return report, fmt.Errorf("memory: profile completion: %w", err)
	}

	for _, f := range parsed.Facets {
		facet := strings.ToLower(strings.TrimSpace(f.Facet))
		if !validFacet(facet) || strings.TrimSpace(f.Content) == "" {
			continue
		}
		if _, _, err := m.Remember(corr, RememberSpec{
			Type:    TypePreference,
			Subject: profileSubjectPrefix + facet,
			Content: strings.TrimSpace(f.Content),
			Tags:    map[string]string{"scope": "", "source": "profile"},
			Actor:   "profile",
			Force:   true, // explicit curation — bypass low-value retention filtering
		}); err != nil {
			return report, err
		}
		report.FacetsWritten++
		report.Facets = append(report.Facets, facet)
	}
	if report.FacetsWritten > 0 {
		m.publish(event.KindMemoryProfiled, corr, map[string]any{
			"input_records":  report.InputRecords,
			"facets_written": report.FacetsWritten,
			"facets":         report.Facets,
		})
	}
	return report, nil
}

// ProfileText returns the current operator-profile facets as a compact block for
// injection into runs, or "" when no profile has been synthesized yet.
func (m *Manager) ProfileText() string {
	active, err := m.Active()
	if err != nil {
		return ""
	}
	var facets []Record
	for _, r := range active {
		if r.Tags["scope"] == "" && strings.HasPrefix(r.Subject, profileSubjectPrefix) {
			facets = append(facets, r)
		}
	}
	if len(facets) == 0 {
		return ""
	}
	sortRecords(facets)
	var b strings.Builder
	for _, r := range facets {
		fmt.Fprintf(&b, "- %s: %s\n", strings.TrimPrefix(r.Subject, profileSubjectPrefix), strings.TrimSpace(r.Content))
	}
	return strings.TrimRight(b.String(), "\n")
}
