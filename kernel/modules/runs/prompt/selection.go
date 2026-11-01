// SPDX-License-Identifier: MIT

package prompt

import (
	"github.com/agezt/agezt/kernel/contextselect"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
)

// The candidate adapters: the stores' scored hits become the context
// selection's candidates, so a manifest can say what was chosen, what was
// rejected and why.
//
// The types (Candidate, Manifest) and the pure helpers (SplitCandidates,
// TokenCost, Freshness, Risk, ...) live in kernel/contextselect: import that
// package directly when only the data types or the pure computation are
// needed. These three adapters stay here because they name the contract rows
// the prompt reads.

// memoryContextCandidates converts memory search hits to context candidates.
func memoryContextCandidates(hits []memoryapi.Scored, nowMS int64) []contextselect.Candidate {
	return contextselect.MemoryCandidates(hits, nowMS)
}

// worldContextCandidates converts world-model search hits to context candidates.
func worldContextCandidates(hits []worldmodelapi.ScoredEntity, nowMS int64) []contextselect.Candidate {
	return contextselect.WorldCandidates(hits, nowMS)
}

// skillContextCandidates converts skill search hits to context candidates.
func skillContextCandidates(hits []skillapi.Scored, nowMS int64) []contextselect.Candidate {
	return contextselect.SkillCandidates(hits, nowMS)
}
