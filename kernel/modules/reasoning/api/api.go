// SPDX-License-Identifier: MIT

// Package api is the reasoning module's public vocabulary: the rows and
// results the council, the conductor and the research harness produce, and the
// pure helpers both sides share.
//
// It exists because another module may import a module's api package and
// nothing else (archcheck's principle P5). The runtime keeps its own names as
// aliases of these types, so the host's public API is unchanged while the
// implementation lives in the module.
//
// Rules of the road:
//
//   - Types and pure helpers only. No logic that needs a provider, a tool
//     invoker or an event bus — those are the implementation package's ports.
//   - One type, one home. A row both the host and the module name is declared
//     here exactly once; the other side aliases it.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/types"
	"github.com/agezt/agezt/kernel/event"
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

// Dependencies are the module's ports: everything the reasoning module needs
// from its host.
//
// The module is stateless. It holds no provider, no tool registry and no event
// bus of its own — the host implements these ports and injects them at
// construction, so the council, the conductor and the research harness keep
// running on the host's policy, audit and lookup machinery.
//
// A module may not reach into another module's implementation (archcheck's
// principle P5), so the host builds these ports and hands them over. That is
// also why the ports live here, in the api package, and not in the
// implementation package: the host may import the api package, nothing else.
type Dependencies struct {
	// Complete is the host's auxiliary-completion funnel. Every aux call the
	// module makes goes through it, so the governor's task routing, the task
	// budgets and the per-run spend attribution keep working.
	Complete func(ctx context.Context, corr, taskType string, req llm.CompletionRequest) (*llm.CompletionResponse, error)
	// HasProvider reports whether the host has a model provider wired. The
	// research harness refuses to start without one.
	HasProvider func() bool
	// RunTool invokes a registered tool through the host's policy and audit
	// pipeline.
	RunTool func(ctx context.Context, corr, callID, tool string, args json.RawMessage) (toolapi.Result, error)
	// ToolAvailable reports whether a tool is registered on the host.
	ToolAvailable func(name string) bool
	// Publish emits one event on the host's bus.
	Publish func(spec event.Spec)
	// Members returns the default panel: the seats and the models they speak
	// with. The council asks it when no panel is given, and the conductor
	// resolves its role models from it.
	Members func() []CouncilMember
	// WebSearch enables the council's dated web-research brief.
	WebSearch bool
	// CodeExecAvailable reports whether the host can execute code. The
	// conductor's verifier prefers a live run over an LLM critique, and falls
	// back to the critique when the host cannot run code.
	CodeExecAvailable func() bool
	// RunCode runs one snippet in the host's sandbox, through the host's policy
	// and audit pipeline. The bool reports whether the snippet actually ran.
	RunCode func(ctx context.Context, corr, callPrefix, language, code, input string) (toolapi.Result, bool, error)
}

// Service is the reasoning module's surface: the council, the conductor and
// the research harness. The host holds it as an interface and delegates to it,
// so the module's implementation can move without the host's API changing.
type Service interface {
	// Council convenes the panel on a question and returns the recorded
	// opinions plus the chair's consensus.
	Council(ctx context.Context, corr, question string, members []CouncilMember, rounds int) (CouncilResult, error)
	// Conduct runs the thinker-worker-verifier loop on a task.
	Conduct(ctx context.Context, corr string, cfg ConductorConfig) (ConductorResult, error)
	// Research runs the deep-research harness and returns a cited report.
	Research(ctx context.Context, corr, question string, opts ResearchOptions) (ResearchReport, error)
}

// NormalizeMembers trims the seats and models, drops the seats without a
// model, names the blank seats by position and removes the duplicates — the
// panel the council and the conductor actually use.
//
// It lives here because both the module and the host (which reports the
// default panel before the council is asked) need the same rule.
func NormalizeMembers(members []CouncilMember) []CouncilMember {
	out := make([]CouncilMember, 0, len(members))
	seen := map[string]bool{}
	for i, m := range members {
		m.Seat = strings.TrimSpace(m.Seat)
		m.Model = strings.TrimSpace(m.Model)
		if m.Model == "" {
			continue
		}
		if m.Seat == "" {
			m.Seat = fmt.Sprintf("Elder %d", i+1)
		}
		key := m.Seat + "\x00" + m.Model
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, m)
	}
	return out
}

// OrPlaceholder is the stand-in for an empty answer.
func OrPlaceholder(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(no answer)"
	}
	return s
}

// Clip bounds a string for a journal payload (rune-safe).
func Clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
