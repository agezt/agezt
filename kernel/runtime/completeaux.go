// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"errors"

	"github.com/agezt/agezt/kernel/agent"
)

// completeAux is the one funnel for the kernel's AUXILIARY one-shot
// completions — council seats, research plan/synth/verify, conductor roles,
// workflow llm nodes and drafts, assure verification, the vision sidecar
// (LD-5). It exists so every aux call carries the two fields the governor's
// task routing, task budgets, and per-run spend attribution key on:
// CorrelationID and TaskType. Before this funnel, individual call sites had
// begun to drop them (workflow nodes and drafts made completions with no
// correlation, so their spend was unattributable in `agt why`). A site's own
// non-empty values win; corr/taskType only fill gaps.
func (k *Kernel) completeAux(ctx context.Context, corr, taskType string, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	if k.cfg.Provider == nil {
		return nil, errors.New("runtime: no provider configured")
	}
	if req.CorrelationID == "" {
		req.CorrelationID = corr
	}
	if req.TaskType == "" {
		req.TaskType = taskType
	}
	return k.cfg.Provider.Complete(ctx, req)
}
