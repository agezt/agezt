// SPDX-License-Identifier: MIT

// Package skill implements "Forge v1" (SPEC-05 §4–5): the agent learns
// reusable, named procedures ("skills") from what it does, and those skills
// are governed through a journaled, reversible state machine instead of
// straight-to-active markdown. This is the "Curator-killer" — Agezt matches
// Hermes's learning loop and beats it on auditability and reversibility: every
// skill mutation is a content-addressed, hash-chained event, so you can ask
// why a skill exists, when it was promoted, and undo it (`agt skill revert`).
//
// Two layers, mirroring kernel/memory and kernel/worldmodel:
//
//   - Store (this file) is a pure, file-backed record store — no bus, no
//     journaling — owning content-addressing and the legal-transition table.
//     A CobaltDB-class engine can replace it behind the Store interface later.
//   - Forge (forge.go) wraps a Store with the kernel bus so every transition
//     (create/promote/quarantine/revert/activate) is a durable-before-publish
//     event carrying the run's correlation_id (SPEC-05 §5.3).
//
// Content-addressing is versioning: a skill's id is BLAKE3(name\0body), so
// editing the body yields a NEW record (a new version) with Lineage pointing
// at its parent — never a destructive edit (§4.3). Status and metrics are
// mutable metadata on the record; lifecycle transitions don't change the id.
//
// Concurrency: a single Store instance is safe for concurrent use.
package skill
