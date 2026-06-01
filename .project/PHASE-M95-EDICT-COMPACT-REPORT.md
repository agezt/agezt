# Phase Report — Milestone M95 (durable-policy compaction)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 / Edict durability.

## Why

The durable policy overlay (M20, `AGEZT_EDICT_DURABLE=on`) is rebuilt at boot by
folding *every* `policy.changed` event in the journal. Over a long-lived system
with much runtime tuning, that history grows unbounded and every superseded
change is replayed on each boot. M95 adds compaction: a snapshot collapses the
net overlay into a minimal change list + the journal seq it covers, so boot
replays `{snapshot + only the changes after it}` — the same result, bounded
replay. The journal stays the immutable source of truth; the snapshot is a
regenerable projection.

## What shipped

- **`edict.PolicyOverlay.ToChanges()`** — renders the overlay as the minimal
  ordered `PolicyChange` list that reproduces it (mode.set + one level.set per
  capability + one deny.add per surviving rule). Round-trip invariant:
  `ProjectPolicyChanges(o.ToChanges()) == o`.
- **`edict.OverlaySnapshot` + Load/Save** — `{through_seq, changes}` persisted
  atomically (write-temp-rename) at `<baseDir>/runtime/edict_overlay_snapshot.json`.
- **Snapshot-aware boot replay** (`replayPolicyOverlay`) — seeds the fold with the
  snapshot's changes and replays only journal events with `Seq > through_seq`. A
  missing or corrupt snapshot falls back to the full fold (never wedges boot).
- **`agt edict compact`** (`CmdEdictCompact`) — folds the journal, writes the
  snapshot at the current head seq, reports `folded → compacted`.

## Design decisions

- **Snapshot is a minimal change list, not a binary overlay.** Storing
  `ToChanges()` means boot uses the *same* `ProjectPolicyChanges` for snapshot and
  post-snapshot events — the fold is resumable and provably equivalent, with no
  second code path that could drift.
- **Fallback-safe.** The snapshot only ever optimizes; absent/corrupt → full
  fold. The journal remains authoritative, so compaction can never lose or
  corrupt policy (worst case: a slower boot).
- **Append-only respected.** The journal is never rewritten (it's hash-chained);
  compaction is a side snapshot, regenerable at any time by re-running compact.

## Tests

- `TestOverlayToChanges_RoundTrip` — `ProjectPolicyChanges(o.ToChanges()) == o`
  (levels, mode, deny rules).
- `TestOverlaySnapshot_SaveLoad` — disk round-trip; absent file is `(nil, nil)`.
- `TestEdictCompact_EquivalentToFullReplay` — the core guarantee: snapshot +
  post-snapshot fold == full-history fold, including a post-snapshot change.

Test count: **1339 → 1342**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt edict level shell L1/L2/L3 ; agt edict mode deny   # 4 policy.changed
$ agt edict compact
  compacted 4 policy.changed event(s) → 2 change(s) (through seq 3).
# restart the daemon (AGEZT_EDICT_DURABLE=on):
  policy engine : … durable=on (restored 1 level(s), 0 deny rule(s); mode=deny)
$ agt edict overlay
  mode : deny ; levels: shell L3      # intact after restart, via the snapshot
```
