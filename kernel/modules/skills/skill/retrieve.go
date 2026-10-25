// SPDX-License-Identifier: MIT

package skill

// Provenance: skill retrieval — the Scored row and the ranking rules. The
//             implementation moved to the contract layer
//             (kernel/contract/skillapi) during the W3.55 slice: the run-time
//             and the context selector rank skills too, and they may not name
//             this module to reach the ranking. The names stay here as
//             aliases and delegations, so every existing caller is unchanged
//             and there is one row and one spelling of it.

import "github.com/agezt/agezt/kernel/contract/skillapi"

// Scored is a skill paired with its retrieval score.
//
// The row itself lives in the contract layer (kernel/contract/skillapi); the
// name stays here as an alias, so every existing caller is unchanged.
type Scored = skillapi.Scored

// Retrieve ranks the ACTIVE skills in sks against an intent and returns the
// top `limit` by score (SPEC-05 §4.2, §7 step 4). A skill scores on keyword
// overlap between the intent tokens and the skill's name + description +
// triggers, weighted by recency. Only active skills participate — drafts,
// shadows, quarantined and archived skills are never injected into a run.
//
// Ranking is a pure function of (sks, intent, limit, nowMS): same inputs →
// same ordering, ties broken by LastUsedMS (more-used recently first) then id.
// Mirrors memory.Search / worldmodel.Resolve so retrieval is consistent across
// the three knowledge layers.
//
// The rule itself lives in the contract layer (kernel/contract/skillapi): the
// layers above the module rank skills too, and they may not name this module
// to reach the ranking.
func Retrieve(sks []Skill, intent string, limit int, nowMS int64) []Scored {
	return skillapi.Retrieve(sks, intent, limit, nowMS)
}

// RetrieveShadow ranks the SHADOW skills in sks against an intent, using the same
// scoring as Retrieve. Shadow skills are never injected into runs; this is the
// candidate set for shadow-evaluation (M400) — the same relevance ranking
// decides which shadow skills to evaluate against a completed run.
//
// The rule itself lives in the contract layer (kernel/contract/skillapi).
func RetrieveShadow(sks []Skill, intent string, limit int, nowMS int64) []Scored {
	return skillapi.RetrieveShadow(sks, intent, limit, nowMS)
}
