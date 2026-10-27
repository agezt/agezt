// SPDX-License-Identifier: MIT

package council

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/reasoningapi"
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

// PortsFromKernel fills the council's ports from the kernel handle.
func PortsFromKernel(k *runtime.Kernel, baseDir string) Ports {
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
func EnginesFromKernel(k *runtime.Kernel) Engines {
	return Engines{
		Convene: func(ctx context.Context, corr, question string, rounds int) (reasoningapi.CouncilResult, error) {
			return k.Council(ctx, corr, question, nil, rounds)
		},
		RunConductor:   k.Conduct,
		NewCorrelation: k.NewCorrelation,
	}
}
