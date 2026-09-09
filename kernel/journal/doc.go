// SPDX-License-Identifier: MIT

// Package journal is the append-only, BLAKE3-hash-chained event log
// that backs the entire system. Segments are 64 MiB JSONL with
// per-event ULIDs and a chained hash; a sidecar index records the
// (offset, seq, hash) of every entry so a tail/grep/head reads in
// O(log N). The journal is the source of truth for replay,
// revert, and the projection of state; the mutable state store
// (kernel/state) is a read cache, not an alternative record
// (DECISIONS B0c). Open() recovers the head hash by scanning
// existing segments so a crash mid-write never corrupts the chain.
package journal
