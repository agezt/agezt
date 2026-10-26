// SPDX-License-Identifier: MIT

package runtime

// Provenance: the kernel's nil-safe face of the memory (W3.57). The memory's
//             FILE is the daemon's, and the kernel holds the contract's
//             port — so a host that was given no memory holds a NIL port.
//
// The answers differ by direction, and each one is stated ONCE, here:
//
//   - the READS answer "no memory": a count of zero, an empty list, an empty
//     profile block. "How much do you know?" has an honest answer even when
//     the daemon has no memory;
//   - the WRITES refuse: they report that the memory is not available on this
//     daemon rather than pretending the row was written or forgotten.
//
// Every surface above the kernel goes through these helpers — a method value
// taken from a nil port panics before the first call, so the decision has to
// live in one place.

import (
	"context"
	"errors"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
)

// ErrNoMemory is the refusal a write answers when the daemon gave no memory.
var ErrNoMemory = errors.New("the memory is not available on this daemon")

// MemoryCount returns how many records the memory holds — ZERO when the
// daemon gave no memory. A count of zero is an honest answer; calling into a
// nil port is not.
func (k *Kernel) MemoryCount() int {
	if k.memory == nil {
		return 0
	}
	return k.memory.Count()
}

// MemoryActive returns the records that take part in retrieval — an empty
// list when the daemon gave no memory.
func (k *Kernel) MemoryActive() ([]memoryapi.Record, error) {
	if k.memory == nil {
		return nil, nil
	}
	return k.memory.Active()
}

// MemoryProfileText renders the operator profile block — empty when the
// daemon gave no memory.
func (k *Kernel) MemoryProfileText() string {
	if k.memory == nil {
		return ""
	}
	return k.memory.ProfileText()
}

// MemorySearchScoped ranks the records of one scope against a query — an
// empty list when the daemon gave no memory.
func (k *Kernel) MemorySearchScoped(query string, limit int, scope string) ([]memoryapi.Scored, error) {
	if k.memory == nil {
		return nil, nil
	}
	return k.memory.SearchScoped(query, limit, scope)
}

// MemoryRecallScoped recalls the records of one scope — an empty list when
// the daemon gave no memory.
func (k *Kernel) MemoryRecallScoped(corr, query string, limit int, scope string) ([]memoryapi.Scored, error) {
	if k.memory == nil {
		return nil, nil
	}
	return k.memory.RecallScoped(corr, query, limit, scope)
}

// MemoryForget soft-deletes a record. It REFUSES when the daemon gave no
// memory: a write that cannot happen must say so, not answer "nothing was
// forgotten" — that would read as a successful no-op on a store that does
// not exist.
func (k *Kernel) MemoryForget(corr, id string) (bool, error) {
	if k.memory == nil {
		return false, ErrNoMemory
	}
	return k.memory.Forget(corr, id)
}

// MemoryDistillFacts extracts durable facts from a finished run's transcript.
// It REFUSES when the daemon gave no memory.
func (k *Kernel) MemoryDistillFacts(ctx context.Context, corr string, provider llm.Provider, model, intent, transcript string) ([]string, error) {
	if k.memory == nil {
		return nil, ErrNoMemory
	}
	return k.memory.Distill(ctx, corr, provider, model, intent, transcript)
}

// MemoryDistillBrain consolidates related records into one. It REFUSES when
// the daemon gave no memory.
func (k *Kernel) MemoryDistillBrain(ctx context.Context, corr string) (memoryapi.BrainDistillReport, error) {
	if k.memory == nil {
		return memoryapi.BrainDistillReport{}, ErrNoMemory
	}
	return k.memory.DistillBrain(ctx, corr, k.cfg.Provider, k.cfg.Model)
}

// MemoryDistillProfile synthesises the operator profile. It REFUSES when the
// daemon gave no memory.
func (k *Kernel) MemoryDistillProfile(ctx context.Context, corr string) (memoryapi.ProfileReport, error) {
	if k.memory == nil {
		return memoryapi.ProfileReport{}, ErrNoMemory
	}
	return k.memory.DistillProfile(ctx, corr, k.cfg.Provider, k.cfg.Model)
}
