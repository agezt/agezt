// SPDX-License-Identifier: MIT

package runtime

// Brain distillation surface (M804): the kernel-side wrapper for one memory
// consolidation pass (kernel/memory.DistillBrain) — clusters related
// records by the M803 local embeddings, merges each cluster through the
// provider into one consolidated record, and supersedes the originals.
// Runs on demand (`agt memory consolidate`, the console) and on the
// AGEZT_BRAIN_DISTILL_EVERY timer the daemon arms at boot — the standing
// "sleep cycle" that keeps a long-lived brain from drowning in its own
// accumulated near-duplicates.

import (
	"context"

	"github.com/agezt/agezt/kernel/memory"
)

// DistillBrain runs one consolidation pass over the memory store under
// corr, using the kernel's configured provider/model (TaskType "distill" —
// the same budgeting class as per-run distillation). Halted kernels refuse.
func (k *Kernel) DistillBrain(ctx context.Context, corr string) (memory.BrainDistillReport, error) {
	k.mu.Lock()
	halted := k.halted
	k.mu.Unlock()
	if halted {
		return memory.BrainDistillReport{}, ErrHalted
	}
	return k.memory.DistillBrain(ctx, corr, k.cfg.Provider, k.cfg.Model)
}
