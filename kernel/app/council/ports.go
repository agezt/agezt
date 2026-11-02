// SPDX-License-Identifier: MIT

package council

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/reasoningapi"
	"github.com/agezt/agezt/kernel/modules/reasoning/api"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
)

// The council's ports, filled from the kernel handle.
//
// The APPLICATION owns this: it knows which capabilities the council needs
// and how each of them answers. The host hands over the handle and nothing
// else — it does not decide what the council reads, and it cannot change the
// council's wiring without editing the application.
//
// The same shape already exists in app/catalog, app/providers and app/runs:
// the application takes the kernel and builds its own ports.
//
// The members the council seats are CONTRACT rows (reasoningapi.CouncilMember
// is the reasoning module's row, which is the contract's), so the port hands
// them over without a conversion.

// PortsFromKernel fills the council's ports from the daemon's own kernel.
//
// The council's own wiring is the council family's business: the seats, the
// settings store and the model catalog are named HERE, and the adapter never
// names them.
//
// A daemon that does not exist yields ports that REFUSE — there are no seats
// to sit, and no store to read.
func PortsFromKernel(k *runtime.Kernel, baseDir string) Ports {
	if k == nil {
		return Ports{
			Members:    func() []Member { return nil },
			SetMembers: func([]Member) {},
			Store:      func() Store { return nil },
			Models:     func() func(string) bool { return func(string) bool { return false } },
		}
	}
	return Ports{
		Members: func() []Member { return k.CouncilDefaultMembers() },
		SetMembers: func(members []Member) {
			k.SetCouncilMembers(func() []reasoningapi.CouncilMember { return members })
		},
		Store: func() Store { return settings.NewStore(baseDir) },
		Models: func() func(string) bool {
			cat := k.Catalog()
			return func(model string) bool {
				_, m := cat.FindModel(model)
				return m != nil
			}
		},
	}
}

// EnginesFromKernel fills the council's engines from the kernel handle: the
// council itself, the conductor that runs a thinking task, and the
// correlation mint for a run that arrives without one.
//
// The engines are the REASONING MODULE's (W3.76v): the module keeps the
// Council, the Conductor and the deep-research harness, and the daemon holds
// only the module's face — so the application asks the module directly.
//
// A daemon that holds no module yields engines that REFUSE: there is no panel
// to sit and no conductor to run, and the refusal says so rather than calling
// into nothing.
//
// The refusal is the CONTRACT's row (reasoningapi.ErrNoService): the missing
// capability is one fact, and every layer that feels it says it the same way.
func EnginesFromKernel(k *runtime.Kernel) Engines {
	engines := Engines{}
	if k == nil {
		engines.Convene = func(context.Context, string, string, int) (api.CouncilResult, error) {
			return api.CouncilResult{}, reasoningapi.ErrNoService
		}
		engines.RunConductor = func(context.Context, string, api.ConductorConfig) (api.ConductorResult, error) {
			return api.ConductorResult{}, reasoningapi.ErrNoService
		}
		engines.NewCorrelation = func() string { return "" }
		return engines
	}
	engines.NewCorrelation = k.NewCorrelation
	svc := k.ReasoningService()
	if svc == nil {
		engines.Convene = func(context.Context, string, string, int) (api.CouncilResult, error) {
			return api.CouncilResult{}, reasoningapi.ErrNoService
		}
		engines.RunConductor = func(context.Context, string, api.ConductorConfig) (api.ConductorResult, error) {
			return api.ConductorResult{}, reasoningapi.ErrNoService
		}
		return engines
	}
	engines.Convene = func(ctx context.Context, corr, question string, rounds int) (api.CouncilResult, error) {
		return svc.Council(ctx, corr, question, nil, rounds)
	}
	engines.RunConductor = svc.Conduct
	return engines
}
