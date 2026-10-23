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
// The ROWS live in the contract layer (kernel/contract/reasoningapi), because
// the agent-facing tools that expose them are plugins and a plugin may only
// reach the contract and the platform layers. This package re-exports them
// under the module's names, so the module, the host and the tools all speak
// the same types — and no tool has to reach into the kernel for a type.
//
// Rules of the road:
//
//   - Types and pure helpers only. No logic that needs a provider, a tool
//     invoker or an event bus — those are the implementation package's ports.
//   - One type, one home. A row both the host and the module name is declared
//     exactly once — in the contract package — and every other side aliases
//     it.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/reasoningapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
)

// CouncilMember is one seat: a human-readable label and the model id it speaks
// with (a bare model id the Governor routes to its serving provider).
type CouncilMember = reasoningapi.CouncilMember

// Opinion is one member's position in one round.
type Opinion = reasoningapi.Opinion

// CouncilResult is the outcome: every recorded opinion, plus the chair's
// synthesized consensus (and any noted dissent).
type CouncilResult = reasoningapi.CouncilResult

// ConductorConfig parameterises one Conduct run.
type ConductorConfig = reasoningapi.ConductorConfig

// ConductorStep is one recorded action in the transcript.
type ConductorStep = reasoningapi.ConductorStep

// ConductorExec records a verifier code-execution.
type ConductorExec = reasoningapi.ConductorExec

// ConductorResult is the outcome of a Conduct run.
type ConductorResult = reasoningapi.ConductorResult

// ResearchSource is one gathered, hashed web source. ID is the citation token
// ("S1", "S2", ...) the synthesis must reference; Hash marks the fetched text so
// a later re-run can detect that a source changed.
type ResearchSource = reasoningapi.ResearchSource

// ResearchClaim is one factual claim lifted from the synthesis and put through
// adversarial verification (Faz 2). Verdict is "supported", "refuted", or
// "uncertain"; SourceIDs are the [S#] tokens the claim cited.
type ResearchClaim = reasoningapi.ResearchClaim

// ResearchReport is the outcome of a deep-research run: the sub-questions it
// explored, the sources it grounded on, the cited synthesis, the verified
// claims, and a confidence derived from verification (or citation coverage when
// verification is off).
type ResearchReport = reasoningapi.ResearchReport

// ResearchOptions tunes a research run. Zero values fall back to safe defaults.
type ResearchOptions = reasoningapi.ResearchOptions

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
