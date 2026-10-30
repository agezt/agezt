// SPDX-License-Identifier: MIT

package research

import (
	"context"
	"errors"

	"github.com/agezt/agezt/kernel/modules/reasoning/api"
	"github.com/agezt/agezt/kernel/runtime"
)

// EnginesFromKernel fills the research family's engines from the daemon's own
// kernel.
//
// The research capability IS the daemon's own: the research family fills its
// own engines from it. A daemon that does not exist yields engines that
// REFUSE — the research question is answered by nobody rather than by a
// capability that does not exist.
func EnginesFromKernel(k *runtime.Kernel, newCorrelation func() string) Engines {
	research := ResearchFromKernel(k)
	if research == nil {
		research = func(context.Context, string, string, api.ResearchOptions) (api.ResearchReport, error) {
			return api.ResearchReport{}, errors.New("the research capability is not available")
		}
	}
	return Engines{
		Research:       research,
		NewCorrelation: newCorrelation,
	}
}

// ResearchFromKernel returns the daemon's research capability from the
// daemon's own kernel.
//
// It is nil when there is no daemon.
func ResearchFromKernel(k *runtime.Kernel) func(ctx context.Context, corr, question string, opts api.ResearchOptions) (api.ResearchReport, error) {
	if k == nil {
		return nil
	}
	return k.Research
}
