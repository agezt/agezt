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
//
// The nil-safe face of the memory lives HERE now (W3.76k): the daemon used to
// keep two names for the one operation — this door (the one the app layer
// and the pulse timer call) and a MemoryDistillBrain accessor beside it.
// There is one spelling now.
func (k *Kernel) DistillBrain(ctx context.Context, corr string) (memoryapi.BrainDistillReport, error) {
	k.registry.Lock()
	halted := k.registry.Halted()
	k.registry.Unlock()
	if halted {
		return memoryapi.BrainDistillReport{}, ErrHalted
	}
	if k.memory == nil {
		return memoryapi.BrainDistillReport{}, ErrNoMemory
	}
	return k.memory.DistillBrain(ctx, corr, k.cfg.Provider, k.cfg.Model)
}

// DistillProfile runs one operator-profile distillation pass (M1000): synthesize
// the operator's profile facets from accumulated shared-scope memory and write
// each as a reinforced PREFERENCE record, so every subsequent run knows who it
// works for. Same provider/model + "distill" budgeting class as DistillBrain.
// Halted kernels refuse.
//
// One spelling, like DistillBrain: the MemoryDistillProfile accessor beside
// it is gone (W3.76k).
func (k *Kernel) DistillProfile(ctx context.Context, corr string) (memoryapi.ProfileReport, error) {
	k.registry.Lock()
	halted := k.registry.Halted()
	k.registry.Unlock()
	if halted {
		return memoryapi.ProfileReport{}, ErrHalted
	}
	if k.memory == nil {
		return memoryapi.ProfileReport{}, ErrNoMemory
	}
	return k.memory.DistillProfile(ctx, corr, k.cfg.Provider, k.cfg.Model)
}
