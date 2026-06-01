# Phase Report — Milestone M101 (verifiable journal export)

> Status: **shipped** · Date: 2026-06-01 · SPEC-09 §8 (export/backup).

## Why

The durable, BLAKE3-hash-chained journal is the system's source of truth. An
operator running SLA/compliance workloads needs to archive it (or a recent
window) to disk for audit, disaster-recovery, or off-system analysis — **and to
trust that archive later**. `agt journal tail --json > file` can dump events,
but it is count-capped (10k) and produces nothing an auditor can re-verify: a
single edited byte goes undetected. SPEC-09 §8 specifies an export/backup
capability; this ships the foundational, self-contained piece — a *verifiable*
export.

## What shipped

- **`agt journal export [--since <dur>] [--out <file>]`** — writes a bundle:
  a manifest binding the export to the chain head at export time
  (`head_seq` + `head_hash`) plus every event (optionally only those in the last
  `<dur>`) with its `hash` and `prev_hash` intact. Default target is stdout.
- **`agt journal verify --bundle <file>`** — re-verifies a bundle **OFFLINE**
  (no daemon): recomputes each event's BLAKE3 hash (catching any payload/field
  tampering) and checks prev-hash continuity across the slice (catching gaps /
  reordering). `agt journal verify` with no flag keeps verifying the live chain.
- **`CmdJournalExport`** server handler — streams every event since an optional
  `since_ms` cutoff, with a 200k-event memory backstop surfaced as
  `truncated=true` (never a silent cut).
- **`Client.CallRaw`** — a byte-preserving sibling of `Call`. The standard path
  decodes results into `map[string]any`, which reorders payload object keys and
  renumbers integers — fatal to a hash computed over canonical bytes. `CallRaw`
  returns the raw `result` JSON so exported events survive the wire verifiably.

## Design decisions

- **Byte fidelity is the whole game.** The export is worthless if it can't
  re-verify; the round-trip (disk → server → wire → bundle file → decode →
  recompute) is normalized by `event.Canonical()`'s compaction, so re-indenting
  the bundle for readability is safe. `CallRaw` exists solely to stop
  `map[string]any` from corrupting payload bytes en route.
- **Windowed slices verify too.** A `--since` export's first event is not
  genesis-linked, so verification checks per-event integrity + *intra-slice*
  continuity rather than a genesis chain — proving the slice is untampered and
  gap-free without requiring the whole journal.
- **Self-contained, not the whole SPEC-09 suite.** Full `export/import/backup/
  restore/migrate` is large; this ships the verifiable-export primitive the rest
  builds on, end-to-end and demo-proven, in one milestone.

## Tests

- `TestVerifyBundleEvents` — intact chain OK; windowed slice OK; tampered
  payload → error; dropped-middle gap → chain break at the gap; empty → OK.
- `TestShortHash` — display trimming.
- `TestJournalExport` — runs a task, exports via `CallRaw`, decodes every event
  and confirms each re-verifies + chains after the wire round-trip; head
  attestation present and matching the live journal head.
- `TestJournalExportSinceWindow` — `since_ms` narrows without breaking verify.

Test count: **1348 → 1352**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt journal export --out audit.json
exported 15 event(s) (seq 0..14) to audit.json
  chain head at export: seq=14 hash=9b6a604fc3c8…
$ agt journal verify --bundle audit.json
bundle OK: 15 event(s) verified (seq 0..14); chain head at export seq=14 hash=9b6a604fc3c8…
$ # flip one byte inside an event, then:
$ agt journal verify --bundle audit.json
agt journal verify: bundle INVALID (verified 0/15): event 0 (seq 0): event: hash mismatch …
  (exit 1)
```
