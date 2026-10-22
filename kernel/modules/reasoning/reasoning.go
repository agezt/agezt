// SPDX-License-Identifier: MIT

// Package reasoning is the reasoning module: the Council of Elders (a
// multi-model panel that deliberates and reconciles), the Conductor (a
// verify-driven Thinker→Worker→Verifier loop) and the deep-research harness.
//
// The module is stateless — it holds no store and no session. Everything it
// needs from the host arrives through Dependencies: the attributable
// auxiliary-completion funnel, the governed tool invoker, the event bus, the
// default panel and the sandboxed code runner.
//
// The rows and results it produces live in the sibling api package
// (kernel/modules/reasoning/api), the one place another module may import.
package reasoning

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/reasoning/api"
)

// The rows the module produces are the api package's types; the aliases below
// keep the module's own code reading like the vocabulary it publishes.
type (
	// CouncilMember is one seat of the panel.
	CouncilMember = api.CouncilMember
	// Opinion is one member's position in one round.
	Opinion = api.Opinion
	// CouncilResult is the panel's deliberation and the chair's consensus.
	CouncilResult = api.CouncilResult
	// ConductorConfig parameterises one Conduct run.
	ConductorConfig = api.ConductorConfig
	// ConductorStep is one recorded action in the Conductor's transcript.
	ConductorStep = api.ConductorStep
	// ConductorExec records a verifier code-execution.
	ConductorExec = api.ConductorExec
	// ConductorResult is the outcome of a Conduct run.
	ConductorResult = api.ConductorResult
	// ResearchOptions tunes a research run.
	ResearchOptions = api.ResearchOptions
	// ResearchSource is one gathered, hashed web source.
	ResearchSource = api.ResearchSource
	// ResearchClaim is one verified factual claim.
	ResearchClaim = api.ResearchClaim
	// ResearchReport is the outcome of a deep-research run.
	ResearchReport = api.ResearchReport
)

// Dependencies are the module's ports: what it needs from the host.
//
// They are declared once, in the api package, because a module may import
// another module's api package and nothing else (archcheck's principle P5) —
// the host has to be able to build these ports without importing this
// package.
type Dependencies = api.Dependencies

// errNoProvider is the module's own guard: the host did not wire the
// completion funnel. The host's funnel reports "runtime: no provider
// configured" for a missing model provider, so this only fires on a
// half-wired host.
var errNoProvider = errors.New("reasoning: no provider configured")

// Service is the reasoning module: the council, the conductor and the research
// harness over one set of ports.
type Service struct {
	deps Dependencies
}

// New builds the module over the host's ports.
func New(deps Dependencies) *Service { return &Service{deps: deps} }

// publish sends one event on the host's bus.
func (s *Service) publish(spec event.Spec) {
	if s.deps.Publish == nil {
		return
	}
	s.deps.Publish(spec)
}

// complete runs one attributable auxiliary completion.
func (s *Service) complete(ctx context.Context, corr, taskType string, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	if s.deps.Complete == nil {
		return nil, errNoProvider
	}
	return s.deps.Complete(ctx, corr, taskType, req)
}

// runTool invokes one tool through the host's governed path, so the module's
// web grounding and its research fetches keep the host's policy checks and
// their audit arc. A host without a tool invoker yields an empty result.
func (s *Service) runTool(ctx context.Context, corr, callID, tool string, args json.RawMessage) (toolapi.Result, error) {
	if s.deps.RunTool == nil {
		return toolapi.Result{}, errors.New("reasoning: no tool invoker configured")
	}
	return s.deps.RunTool(ctx, corr, callID, tool, args)
}
