// SPDX-License-Identifier: MIT

package runtime

// Brain distillation surface (M804): the kernel-side wrapper for one memory
// consolidation pass (kernel/modules/knowledge/memory.DistillBrain) — clusters related
// records by the M803 local embeddings, merges each cluster through the
// provider into one consolidated record, and supersedes the originals.
// Runs on demand (`agt memory consolidate`, the console) and on the
// AGEZT_BRAIN_DISTILL_EVERY timer the daemon arms at boot — the standing
// "sleep cycle" that keeps a long-lived brain from drowning in its own
// accumulated near-duplicates.

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/memoryapi"
)

// DistillBrain runs one consolidation pass over the memory store under
// corr, using the kernel's configured provider/model (TaskType "distill" —
// the same budgeting class as per-run distillation). Halted kernels refuse.
func (k *Kernel) DistillBrain(ctx context.Context, corr string) (memoryapi.BrainDistillReport, error) {
	k.registry.Lock()
	halted := k.registry.Halted()
	k.registry.Unlock()
	if halted {
		return memoryapi.BrainDistillReport{}, ErrHalted
	}
	return k.MemoryDistillBrain(ctx, corr)
}

// DistillProfile runs one operator-profile distillation pass (M1000): synthesize
// the operator's profile facets from accumulated shared-scope memory and write
// each as a reinforced PREFERENCE record, so every subsequent run knows who it
// works for. Same provider/model + "distill" budgeting class as DistillBrain.
// Halted kernels refuse.
func (k *Kernel) DistillProfile(ctx context.Context, corr string) (memoryapi.ProfileReport, error) {
	k.registry.Lock()
	halted := k.registry.Halted()
	k.registry.Unlock()
	if halted {
		return memoryapi.ProfileReport{}, ErrHalted
	}
	return k.MemoryDistillProfile(ctx, corr)
}
