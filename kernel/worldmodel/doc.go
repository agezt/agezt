// SPDX-License-Identifier: MIT

// Package worldmodel implements "World Model v1" (SPEC-05 §3): a journaled,
// content-addressed graph of the operator's world — the projects, repos,
// people, accounts, channels and topics they care about, and the weighted
// relations between them. It is the substrate the retrieval pipeline resolves
// against *before* anything else (SPEC-05 §7): "what does 'the portfolio'
// mean?" → a set of repo entities. It is also what lets Pulse's Salience judge
// relevance *to this operator specifically* — the hole salience.go left open
// for "the full world-model relevance signals land with Memory".
//
// It deliberately mirrors kernel/memory's two-layer split, because the same
// properties are wanted (auditability, reversibility, dedupe):
//
//   - Store (this file) is a pure, file-backed graph store — no bus, no
//     journaling — owning content-addressing and the (pure) resolve ranking
//     (resolve.go). A CobaltDB-class adjacency engine (DECISIONS D2) can
//     replace it behind the Store interface later.
//   - Graph (manager.go) wraps a Store with the kernel bus so every node/edge
//     mutation is a durable-before-publish event carrying the run's
//     correlation_id — which is what makes `agt why` able to explain why the
//     system believes "the portfolio" is those repos.
//
// Nodes and edges are content-addressed (BLAKE3) so identical entities and
// relations dedupe and reinforce instead of duplicating; updates are soft
// (SupersededBy) and forgets are soft (Tombstoned) — history is never
// destructively edited, and the graph is diffable across time.
//
// Concurrency: a single Store instance is safe for concurrent use.
package worldmodel
