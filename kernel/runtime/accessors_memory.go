// SPDX-License-Identifier: MIT

package runtime

// Provenance: the kernel's nil-safe face of the memory (W3.57). The memory's
//             FILE is the daemon's, and the kernel holds the contract's
//             port — so a host that was given no memory holds a NIL port.
//
// The answers differ by direction, and each one is stated ONCE, here:
//
//   - the READ answer "no memory": a count of zero. "How much do you know?"
//     has an honest answer even when the daemon has no memory;
//   - the WRITES refuse: they report that the memory is not available on this
//     daemon rather than pretending the row was written or forgotten.
//
// Every surface above the kernel goes through these helpers — a method value
// taken from a nil port panics before the first call, so the decision has to
// live in one place.
//
// The surfaces NOBODY asked for went with W3.76k: the active list, the
// profile block, the scoped search, the scoped recall, the forget and the
// fact distillation had no caller outside the daemon's own tests. The memory
// family is the knowledge module's (kernel/modules/knowledge/memory) — the
// daemon keeps the PORT and the count, and it stops speaking for the rest
// of the family.
//
// The two distill verbs went the same way, one step further: the daemon kept
// TWO names for the one operation — DistillBrain / DistillProfile (the doors
// the app layer and the pulse timer call) and MemoryDistillBrain /
// MemoryDistillProfile beside them. The doors hold the operation now, and
// there is one spelling of it.

import (
	"errors"
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
