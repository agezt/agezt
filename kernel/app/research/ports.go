// SPDX-License-Identifier: MIT

package research

import (
	"context"
	"errors"

	"github.com/agezt/agezt/kernel/modules/reasoning/api"
	"github.com/agezt/agezt/kernel/runtime"
)

// EnginesFromHandle fills the research family's engines from the daemon's
// handle.
//
// The handle is not named by the caller: the research family fills its own
// engines from it. A handle that is not the daemon's own yields engines that
// REFUSE — the research question is answered by nobody rather than by a
// capability that does not exist.
func EnginesFromHandle(h any, newCorrelation func() string) Engines {
	research := ResearchFromHandle(h)
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

// ResearchFromHandle returns the daemon's research capability from the
// handle.
//
// It is nil when the handle is not the daemon's own.
func ResearchFromHandle(h any) func(ctx context.Context, corr, question string, opts api.ResearchOptions) (api.ResearchReport, error) {
	k, ok := h.(*runtime.Kernel)
	if !ok {
		return nil
	}
	return k.Research
}
