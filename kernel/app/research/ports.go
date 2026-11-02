// SPDX-License-Identifier: MIT

package research

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/reasoningapi"
	"github.com/agezt/agezt/kernel/modules/reasoning/api"
	"github.com/agezt/agezt/kernel/runtime"
)

// EnginesFromKernel fills the research family's engines from the daemon's own
// kernel.
//
// The research capability IS the REASONING MODULE's (W3.76v): the harness
// lives in kernel/modules/reasoning, and the daemon holds the module's face. A
// daemon that does not exist — or that was given no module — yields engines
// that REFUSE: the research question is answered by nobody rather than by a
// capability that does not exist.
//
// The refusal is the CONTRACT's row (reasoningapi.ErrNoService): the missing
// capability is one fact, and the council and the research family say it the
// same way.
func EnginesFromKernel(k *runtime.Kernel, newCorrelation func() string) Engines {
	research := ResearchFromKernel(k)
	if research == nil {
		research = func(context.Context, string, string, api.ResearchOptions) (api.ResearchReport, error) {
			return api.ResearchReport{}, reasoningapi.ErrNoService
		}
	}
	return Engines{
		Research:       research,
		NewCorrelation: newCorrelation,
	}
}

// ResearchFromKernel returns the reasoning module's research capability.
//
// The capability is the REASONING MODULE's (W3.76v): the daemon holds the
// module's face, and the application asks the module directly.
//
// It is nil when there is no daemon — or when the daemon holds no module —
// and the engines refuse.
func ResearchFromKernel(k *runtime.Kernel) func(ctx context.Context, corr, question string, opts api.ResearchOptions) (api.ResearchReport, error) {
	if k == nil {
		return nil
	}
	svc := k.ReasoningService()
	if svc == nil {
		return nil
	}
	return svc.Research
}
