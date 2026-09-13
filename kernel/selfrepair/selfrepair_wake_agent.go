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

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

func autoRepairWakeAgent(ctx context.Context, k *kernelruntime.Kernel, cand autoRepairCandidate, msg *board.Message) (autoRepairWakeResult, error) {
	if k == nil {
		return autoRepairWakeResult{}, fmt.Errorf("auto-repair wake requires kernel")
	}
	target := strings.TrimSpace(cand.EscalateTo)
	if target == "" {
		return autoRepairWakeResult{}, nil
	}
	p, ok := k.Roster().Get(target)
	if !ok {
		return autoRepairWakeResult{Target: target, Skipped: "unknown target agent " + target}, nil
	}
	if reason := autoRepairWakeSkipReason(p); reason != "" {
		return autoRepairWakeResult{Target: p.Slug, Skipped: reason}, nil
	}
	corr := k.NewCorrelation()
	rctx := kernelruntime.WithAgentProfile(ctx, p)
	if p.MaxCostMc > 0 {
		rctx = kernelruntime.WithMaxCost(rctx, p.MaxCostMc)
	}
	intent := autoRepairWakeIntent(cand, msg)
	var (
		err    error
		answer string
	)
	if p.RetryPolicy != nil && p.RetryPolicy.MaxAttempts > 1 {
		answer, err = k.RunWithRetry(rctx, corr, intent, *p.RetryPolicy)
	} else {
		answer, err = k.RunWith(rctx, corr, intent)
	}
	return autoRepairWakeResult{
		Target:      p.Slug,
		Correlation: corr,
		Answer:      answer,
		Resolution:  parseAutoRepairResolution(answer),
		Runbook:     roster.AutonomyRunbook(p),
	}, err
}

