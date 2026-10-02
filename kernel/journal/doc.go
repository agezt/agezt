// SPDX-License-Identifier: MIT

// Package journal is the append-only, BLAKE3-hash-chained event log
// that backs the entire system. Segments are 64 MiB JSONL with
// per-event ULIDs and a chained hash. There is no index: every read is
// a sequential scan (measured and deferred, architecture/21 W1.6b). The
// journal is the source of truth for replay, revert, and the projection
// of state; the mutable state store (kernel/state) is a read cache, not
// an alternative record (DECISIONS B0c).
//
// Open recovers the head hash by scanning and verifying every segment.
// A torn final line (a crash mid-write) is truncated away. A corrupt
// record in the middle of the chain is quarantined, not fatal (owner
// decision 5.6): the suffix from that record on is moved to
// *.quarantined-<UTC> files, the chain resumes from the last verified
// event, and a journal.recovered event records the gap (see Recovery).
// Options.FailOnCorruption keeps the old fail-closed behaviour for
// offline tools.
package journal
