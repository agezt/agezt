// SPDX-License-Identifier: MIT

// Package memory implements the memory store (ROADMAP §2.3): a journaled,
// content-addressed knowledge store that the agent loop reads as injected
// context and that the operator, the agent, and an auto-distiller can write
// to. Retrieval is hybrid (M803): exact keyword overlap blended with local
// hashed-n-gram embeddings (vector.go) for typo/morphology recall —
// DECISIONS C5's "local embeddings by default"; provider embeddings remain
// the documented opt-in.
//
// Two layers, mirroring how kernel/state and kernel/runtime split:
//
//   - Store (this file) is a pure, file-backed record store — no bus, no
//     journaling — so it is trivially testable and a CobaltDB-class engine
//     (DECISIONS D2) can replace it behind the interface. It also owns the
//     content-addressing and the keyword retrieval ranking, both pure
//     functions.
//   - Manager (manager.go) wraps a Store with the kernel bus so every
//     mutation is a durable-before-publish event carrying the run's
//     correlation_id — which is what makes `agt why` able to explain every
//     belief (SPEC-05 §2).
//
// Records are content-addressed (BLAKE3 of type\0subject\0content) so
// identical knowledge dedupes; updates are soft (SupersededBy) and forgets
// are soft (Tombstoned) — history is never destructively edited.
//
// Concurrency: a single Store instance is safe for concurrent use.
package memory
