// SPDX-License-Identifier: MIT

package runtime

// reasoning.go is the runtime's face of the reasoning module. The Council of
// Elders, the Conductor and the deep-research harness live in
// kernel/modules/reasoning; this file keeps the host's public names, hands the
// module the ports it needs and forwards the three calls.
//
// A module may import another module's api package and nothing else
// (archcheck's principle P5), so the runtime never imports the module's
// implementation: it talks to it through api.Service and builds its ports
// through api.Dependencies. The composition root (the daemon) owns the concrete
// service and injects it with SetReasoning.

import (
	"context"
	"errors"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/reasoning/api"
)

// The rows the host and the module both name are declared once, in the
// module's api package. The runtime keeps its own names as aliases, so every
// importer compiles unchanged while the implementation lives in the module.
type (
	// CouncilMember is one seat: a human-readable label and the model id it
	// speaks with (a bare model id the Governor routes to its serving provider).
	CouncilMember = api.CouncilMember
	// Opinion is one member's position in one round.
	Opinion = api.Opinion
	// CouncilResult is the outcome: every recorded opinion, plus the chair's
	// synthesized consensus (and any noted dissent).
	CouncilResult = api.CouncilResult
	// ConductorConfig parameterises one Conduct run.
	ConductorConfig = api.ConductorConfig
	// ConductorStep is one recorded action in the transcript.
	ConductorStep = api.ConductorStep
	// ConductorExec records a verifier code-execution.
	ConductorExec = api.ConductorExec
	// ConductorResult is the outcome of a Conduct run.
	ConductorResult = api.ConductorResult
	// ResearchOptions tunes a research run. Zero values fall back to safe
	// defaults.
	ResearchOptions = api.ResearchOptions
	// ResearchSource is one gathered, hashed web source.
	ResearchSource = api.ResearchSource
	// ResearchClaim is one factual claim put through adversarial verification.
	ResearchClaim = api.ResearchClaim
	// ResearchReport is the outcome of a deep-research run.
	ResearchReport = api.ResearchReport
)

// errReasoningUnwired is the host's guard for a kernel whose composition root
// forgot to inject the reasoning module.
var errReasoningUnwired = errors.New("runtime: reasoning module not wired")

// SetReasoning injects the reasoning module. Called once after Open by the
// composition root (the daemon), which builds the service over the host's ports
// with reasoning.New(k.ReasoningDependencies()).
func (k *Kernel) SetReasoning(s api.Service) {
	k.configMu.Lock()
	k.reasoning = s
	k.configMu.Unlock()
}

// reasoningService returns the injected reasoning module, or nil.
func (k *Kernel) reasoningService() api.Service {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.reasoning
}

// ReasoningDependencies returns the host's ports for the reasoning module: the
// attributable completion funnel, the governed tool invoker, the event bus, the
// default panel and the sandboxed code runner.
//
// It is the runtime's side of the module boundary — the module holds no
// provider, no tool registry and no bus of its own, so every call it makes
// keeps the host's policy checks, its audit arc and its spend attribution.
func (k *Kernel) ReasoningDependencies() api.Dependencies {
	return api.Dependencies{
		Complete: k.completeAux,
		HasProvider: func() bool {
			return k.cfg.Provider != nil
		},
		RunTool: k.RunTool,
		ToolAvailable: func(name string) bool {
			t, ok := k.LookupTool(name)
			return ok && t != nil
		},
		Publish: func(spec event.Spec) {
			if k.bus == nil {
				return
			}
			_, _ = k.bus.Publish(spec)
		},
		Members:   k.CouncilDefaultMembers,
		WebSearch: k.cfg.CouncilWebSearch,
		CodeExecAvailable: func() bool {
			return k.conductorExec != nil
		},
		RunCode: func(ctx context.Context, corr, callPrefix, language, code, input string) (toolapi.Result, bool, error) {
			exec := k.conductorExec
			if exec == nil {
				return toolapi.Result{}, false, nil
			}
			return k.runCode(ctx, corr, callPrefix, exec, language, code, input)
		},
	}
}

// CouncilDefaultMembers returns the daemon-configured default membership (one
// seat per keyed provider), or nil. Lets the control plane / Web UI show which
// models the council will convene with before asking.
func (k *Kernel) CouncilDefaultMembers() []CouncilMember {
	members := k.CouncilMembers()
	if members == nil {
		return nil
	}
	return api.NormalizeMembers(members())
}

// Council convenes the panel on a question and returns the deliberation + the
// chair's consensus. When members is nil/empty the daemon-configured default
// membership is used. rounds<=0 uses the module's default.
// One member failing is recorded as a dissent-less opinion error, not a council
// failure; only an empty panel or a total wipe-out errors.
func (k *Kernel) Council(ctx context.Context, corr, question string, members []CouncilMember, rounds int) (CouncilResult, error) {
	svc := k.reasoningService()
	if svc == nil {
		return CouncilResult{}, errReasoningUnwired
	}
	return svc.Council(ctx, corr, question, members, rounds)
}

// Conduct runs the Thinker→Worker→Verifier loop on a task. Empty role models
// are filled from the Council's default membership so the roles run on
// DIFFERENT models out of the box. rounds<=0 uses the module's default.
// A role completion failing is recorded on its step (Error) rather than
// aborting the run; only an empty task or no available models errors.
func (k *Kernel) Conduct(ctx context.Context, corr string, cfg ConductorConfig) (ConductorResult, error) {
	svc := k.reasoningService()
	if svc == nil {
		return ConductorResult{}, errReasoningUnwired
	}
	return svc.Conduct(ctx, corr, cfg)
}

// Research runs the deep-research harness for a question and returns a cited
// report. It never hard-fails on a flaky search or fetch — those degrade the
// report (fewer sources, a note) rather than aborting the run. It returns an
// error only when there is no provider or the synthesis model call fails.
func (k *Kernel) Research(ctx context.Context, corr, question string, opts ResearchOptions) (ResearchReport, error) {
	svc := k.reasoningService()
	if svc == nil {
		return ResearchReport{}, errReasoningUnwired
	}
	return svc.Research(ctx, corr, question, opts)
}
