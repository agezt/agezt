// SPDX-License-Identifier: MIT

package runtime

// reasoning.go is the runtime's face of the reasoning module. The Council of
// Elders, the Conductor and the deep-research harness live in
// kernel/modules/reasoning; this file keeps the host's public names, hands the
// module the ports it needs and hands the module's face over.
//
// A module may import another module's api package and nothing else
// (archcheck's principle P5), so the runtime never imports the module's
// implementation: it talks to it through api.Service and builds its ports
// through api.Dependencies. The composition root (the daemon) owns the concrete
// service and injects it with SetReasoning.
//
// The module's OPERATIONS are the module's (W3.76v): the daemon used to hold
// three forwarders over them — `Council`, `Conduct` and `Research` — a second
// spelling of every verb the contract's runner port already names. The module
// answers in the contract's names now, and the daemon keeps the setter, the
// wiring and the module's FACE.

import (
	"context"

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

// SetReasoning injects the reasoning module. Called once after Open by the
// composition root (the daemon), which builds the service over the host's ports
// with reasoning.New(k.ReasoningDependencies()).
func (k *Kernel) SetReasoning(s api.Service) {
	k.configMu.Lock()
	k.reasoning = s
	k.configMu.Unlock()
}

// reasoningService returns the injected reasoning module, or nil.
//
// The service is the REASONING MODULE's: the module keeps the Council, the
// Conductor and the deep-research harness. The composition root hands the
// BUILDER over (see runtime.Config.ReasoningService) and the kernel calls it
// while opening itself — so the daemon never needs a setter for it.
func (k *Kernel) reasoningService() api.Service {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.reasoning
}

// ReasoningService returns the reasoning module's service — the module's FACE.
//
// It is the port the council, the conductor and the research tool drive, and
// it is nil when the daemon holds no module: the callers refuse then, rather
// than calling into nothing.
//
// The daemon does not speak FOR the module (W3.76v): it used to hold three
// forwarders over the module's own operations — `Council`, `Conduct` and
// `Research` — a second spelling of every verb the contract's runner port
// already names. The module answers in the contract's names now.
func (k *Kernel) ReasoningService() api.Service { return k.reasoningService() }

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
