// SPDX-License-Identifier: MIT

package selfrepair

// autoRepairWakeAgent: single-wake dispatcher (called by
// autoWakeManager above). Carved out of selfrepair_wake.go during
// the Day 191 god-file split so the main file can stay focused on
// the autoWakeManager loop and the helpers file can stay focused
// on the wake-intent / skip-reason / message-id helpers.
// Public API unchanged.

import (
	"context"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
)

func autoRepairWakeAgent(ctx context.Context, ports Ports, cand autoRepairCandidate, msg *boardapi.Message) (autoRepairWakeResult, error) {
	if ports.Runner == nil || ports.Correlation == nil || ports.BindProfile == nil {
		return autoRepairWakeResult{}, fmt.Errorf("auto-repair wake requires the run engine, a correlation and the profile binding")
	}
	target := strings.TrimSpace(cand.EscalateTo)
	if target == "" {
		return autoRepairWakeResult{}, nil
	}
	p, ok := ports.profile(target)
	if !ok {
		return autoRepairWakeResult{Target: target, Skipped: "unknown target agent " + target}, nil
	}
	if reason := autoRepairWakeSkipReason(p); reason != "" {
		return autoRepairWakeResult{Target: p.Slug, Skipped: reason}, nil
	}
	corr := ports.correlation()
	rctx := ports.bindProfile(ctx, p)
	if p.MaxCostMc > 0 {
		rctx = ports.capCost(rctx, p.MaxCostMc)
	}
	intent := autoRepairWakeIntent(cand, msg)
	var (
		err    error
		answer string
	)
	if p.RetryPolicy != nil && p.RetryPolicy.MaxAttempts > 1 {
		answer, err = ports.Runner.RunWithRetry(rctx, corr, intent, *p.RetryPolicy)
	} else {
		answer, err = ports.Runner.RunWith(rctx, corr, intent)
	}
	return autoRepairWakeResult{
		Target:      p.Slug,
		Correlation: corr,
		Answer:      answer,
		Resolution:  parseAutoRepairResolution(answer),
		Runbook:     roster.AutonomyRunbook(p),
	}, err
}
