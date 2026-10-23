// SPDX-License-Identifier: MIT

// Package reasoningapi is the contract the reasoning module and the
// agent-facing tools that drive it agree on.
//
// The council, the conductor and the research harness produce rows — a
// consensus, a transcript, a cited report — and the tools that expose them to
// an agent need to name those rows. A tool is a plugin (L6) and a plugin may
// only reach the contract and the platform layers, so the rows live HERE, at
// the bottom, instead of in the module that produces them or in the kernel
// that hosts it.
//
// The module's own api package re-exports these types under its names, and the
// kernel keeps its names as aliases of those — so the module, the host and the
// tools all speak the same types, and nobody has to reach into the kernel for
// a type.
//
// Rules of the road:
//
//   - Rows only. No logic, no ports, no dependencies: this package is data
//     and nothing else.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package reasoningapi

import (
	"github.com/agezt/agezt/kernel/contract/types"
)

// CouncilMember is one seat: a human-readable label and the model id it speaks
// with (a bare model id the Governor routes to its serving provider).
type CouncilMember = types.CouncilMember

// Opinion is one member's position in one round.
type Opinion = types.Opinion

// CouncilResult is the outcome: every recorded opinion, plus the chair's
// synthesized consensus (and any noted dissent).
type CouncilResult struct {
	Question  string          `json:"question"`
	Members   []CouncilMember `json:"members"`
	Rounds    int             `json:"rounds"`
	Opinions  []Opinion       `json:"opinions"`
	Consensus string          `json:"consensus"`
	Dissent   string          `json:"dissent,omitempty"`
	// AsOf is the date the council convened on (YYYY-MM-DD), given to every seat so
	// the panel reasons with a current clock instead of a stale training cutoff.
	AsOf string `json:"as_of,omitempty"`
	// Brief is the dated web research brief that grounded the panel (empty when web
	// search is disabled, has no tool, or returned nothing). Surfaced so an operator
	// can see what evidence the council was given.
	Brief string `json:"brief,omitempty"`
}

// ConductorConfig parameterises one Conduct run.
type ConductorConfig struct {
	Task      string
	Thinker   string // model id or "@chain"; empty → filled from default members
	Worker    string
	Verifier  string
	MaxRounds int  // worker↔verifier retry cap; <=0 → the default rounds
	Plan      bool // run the optional instruction-tailoring call first
}

// ConductorStep is one recorded action in the transcript.
type ConductorStep struct {
	Round   int            `json:"round"`
	Role    string         `json:"role"` // thinker|worker|verifier
	Model   string         `json:"model"`
	Text    string         `json:"text,omitempty"`
	Verdict string         `json:"verdict,omitempty"` // verifier: pass|fail
	Reason  string         `json:"reason,omitempty"`
	Exec    *ConductorExec `json:"exec,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// ConductorExec records a verifier code-execution.
type ConductorExec struct {
	Ran      bool   `json:"ran"`
	OK       bool   `json:"ok"`
	Language string `json:"language,omitempty"`
	Output   string `json:"output,omitempty"`
}

// ConductorResult is the outcome of a Conduct run.
type ConductorResult struct {
	Task   string            `json:"task"`
	Roles  map[string]string `json:"roles"` // role→resolved model label
	Plan   string            `json:"plan,omitempty"`
	Rounds int               `json:"rounds"` // worker attempts made
	Steps  []ConductorStep   `json:"steps"`
	Answer string            `json:"answer"`
	Passed bool              `json:"passed"`
}

// ResearchSource is one gathered, hashed web source. ID is the citation token
// ("S1", "S2", ...) the synthesis must reference; Hash marks the fetched text so
// a later re-run can detect that a source changed.
type ResearchSource struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
	Text  string `json:"text,omitempty"`
	Hash  string `json:"hash"`
	Rank  int    `json:"rank"`
}

// ResearchClaim is one factual claim lifted from the synthesis and put through
// adversarial verification (Faz 2). Verdict is "supported", "refuted", or
// "uncertain"; SourceIDs are the [S#] tokens the claim cited.
type ResearchClaim struct {
	Text      string   `json:"text"`
	SourceIDs []string `json:"source_ids"`
	Verdict   string   `json:"verdict"`
	Note      string   `json:"note,omitempty"`
}

// ResearchReport is the outcome of a deep-research run: the sub-questions it
// explored, the sources it grounded on, the cited synthesis, the verified
// claims, and a confidence derived from verification (or citation coverage when
// verification is off).
type ResearchReport struct {
	Question     string           `json:"question"`
	SubQuestions []string         `json:"sub_questions"`
	Sources      []ResearchSource `json:"sources"`
	Markdown     string           `json:"markdown"`
	Claims       []ResearchClaim  `json:"claims,omitempty"`
	Confidence   float64          `json:"confidence"`
	CitedSources int              `json:"cited_sources"`
	Verified     bool             `json:"verified"`
	Notes        []string         `json:"notes,omitempty"`
}

// ResearchOptions tunes a research run. Zero values fall back to safe defaults.
type ResearchOptions struct {
	Model           string // optional provider model override; empty => routed default
	MaxSubQuestions int    // default 3, capped at 8
	ResultsPerQuery int    // default 4
	MaxSources      int    // default 8, capped at 20
	Verify          bool   // run the adversarial claim-verification pass (Faz 2)
	MaxVerifyClaims int    // default 6, capped at 12; only meaningful when Verify
}
