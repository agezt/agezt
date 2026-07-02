// SPDX-License-Identifier: MIT

// Package proof is AGEZT's "PROOF > VIBES" record: the durable, checkable
// evidence that a workboard task actually satisfied its acceptance criteria
// before it was allowed to reach done.
//
// It is pure data. The kernel builds a Proof by running the assure completion
// judge over a task's acceptance criteria and gathering the artifacts plus the
// hash-chained journal range produced under the task's correlation id. A task
// with no acceptance criteria has no Proof and is never gated — you opt into
// rigor by declaring criteria (default-allow posture), you are not restricted by
// default.
package proof

import "github.com/agezt/agezt/kernel/assure"

// Criterion is one acceptance criterion a task must satisfy. Text is set when
// the task is created; Met and Note are filled by the judge at prove time.
type Criterion struct {
	Text string `json:"text"`
	Met  bool   `json:"met"`
	Note string `json:"note,omitempty"`
}

// Evidence points at the concrete, verifiable outputs a proof rests on: the
// artifacts produced under the task's correlation id and the hash-chained
// journal sequence range covering the work. These refs are what make a proof
// checkable after the fact rather than a bare assertion.
type Evidence struct {
	Corr        string   `json:"corr,omitempty"`
	Artifacts   []string `json:"artifacts,omitempty"`
	JournalFrom int64    `json:"journal_from,omitempty"`
	JournalTo   int64    `json:"journal_to,omitempty"`
}

// Proof is the durable completion record attached to a task. It aggregates the
// verifier's verdict, the per-criterion outcomes, the supporting evidence, and
// provenance (which judge, how many attempts, when).
type Proof struct {
	Verdict  assure.Verdict `json:"verdict"`
	Criteria []Criterion    `json:"criteria,omitempty"`
	Evidence Evidence       `json:"evidence,omitempty"`
	Attempts int            `json:"attempts,omitempty"`
	Judge    string         `json:"judge,omitempty"`
	ProvedMS int64          `json:"proved_ms"`
}

// Satisfied reports whether the proof clears the gate: the verifier deemed the
// work complete AND every declared criterion is met. An empty criteria list is
// satisfied on the verdict alone.
func (p Proof) Satisfied() bool {
	if !p.Verdict.Complete {
		return false
	}
	for _, c := range p.Criteria {
		if !c.Met {
			return false
		}
	}
	return true
}

// UnmetCount returns how many declared criteria the judge left unmet — useful
// for a compact "3/5 met" summary in the CLI and UI.
func (p Proof) UnmetCount() int {
	n := 0
	for _, c := range p.Criteria {
		if !c.Met {
			n++
		}
	}
	return n
}

// Clone returns a deep copy so callers can hand a Proof across the store
// boundary without sharing slice backing arrays.
func (p Proof) Clone() Proof {
	p.Criteria = append([]Criterion(nil), p.Criteria...)
	p.Evidence.Artifacts = append([]string(nil), p.Evidence.Artifacts...)
	return p
}
