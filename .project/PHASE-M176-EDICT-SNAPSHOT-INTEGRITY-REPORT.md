# M176 — Durable policy snapshot bound to the tamper-evident journal

## Why
Follow-up to the M173 Edict review (HIGH-1): the durable-policy compaction snapshot
(`edict_overlay_snapshot.json`, M95) is read as authoritative at boot. The daemon
seeds the policy fold with `snapshot.Changes` and replays only journal events after
`snapshot.ThroughSeq`, with NO integrity check on the file. The journal is
hash-chained and tamper-evident; the snapshot was not. So an attacker (or a stray
process) that could write that one file in `<baseDir>/runtime/` could downgrade a
trust level (e.g. `shell` L0→L4) or drop a deny rule, and the *next restart* would
adopt the loosened policy — the snapshot won over the journal it was supposed to
merely summarize. This is a boot-time privilege-escalation / policy-downgrade vector.

## What
Bind the on-disk snapshot to the journal so it can only ever be trusted when it
faithfully reflects journaled history.

- **`OverlaySnapshot.ContentHash()`** (`kernel/edict/snapshot.go`) — deterministic
  SHA-256 (hex) over the snapshot's meaning (`through_seq` + ordered `Changes`).
  Deterministic because `OverlaySnapshot` marshals with no maps and `ToChanges`
  already sorts its output.
- **`event.KindPolicyCompacted` (`policy.compacted`)** (`kernel/event/kinds.go`) — a
  new journaled event kind, registered in `knownKinds`.
- **Compact emits the binding** (`kernel/controlplane/edict_overlay.go`) — after
  saving the snapshot, `handleEdictCompact` publishes a `policy.compacted` event with
  `{through_seq, content_hash}`. Because the journal is the hash-chained source of
  truth, this records the legitimate snapshot's fingerprint immutably.
- **Boot trusts the snapshot only on a hash match** (`cmd/agezt/main.go`,
  `replayPolicyOverlay`) — a single `Journal().Range` collects both the
  `policy.changed` history AND the latest journaled `policy.compacted` hash. The
  snapshot seeds the fold ONLY when `snap.ContentHash() == journaledHash`; otherwise
  the snapshot is ignored and the full journal is folded.

## Failure modes, all safe
- **Tampered snapshot** (edited to loosen policy) → hash differs from journaled →
  ignored → full journal fold (the un-loosened truth).
- **Corrupt/unreadable snapshot** → `LoadOverlaySnapshot` error → `snap=nil` → full fold.
- **Pre-binding snapshot** (written before M176, no `policy.compacted` event) →
  `journaledHash == ""` → snapshot ignored → full fold.
- **Legitimate snapshot** → hash matches → bounded resumable replay, equivalent to the
  full fold (the M95 round-trip invariant). No behavior change for honest boots beyond
  the integrity gate.

The snapshot remains a regenerable projection; the journal remains the source of
truth. The fix only ever *removes* trust from a snapshot — it can never grant a
snapshot more authority than the journal already attests, so it cannot itself loosen
policy.

## Tests
- `kernel/edict` — `TestOverlaySnapshot_ContentHash`: hash is non-empty,
  deterministic across independent equal snapshots, and changes under every
  policy-affecting mutation (`through_seq`, a change's level/capability, a deny
  substring, dropping a change).
- `kernel/controlplane` — `TestEdictCompact_JournalsBindingHash`: after
  `CmdEdictCompact` a `policy.compacted` event is journaled whose `content_hash`
  equals the on-disk snapshot's `ContentHash()`; a snapshot edited to loosen a level
  no longer matches the journaled hash.
- `TestEdictCompact_EquivalentToFullReplay` (M95) still green — resumable replay
  unchanged.

## Verification
- `go test ./...` — 1558 passing, 0 failing.
- `go vet ./kernel/edict/ ./kernel/controlplane/ ./cmd/agezt/` clean.
- `gofmt -l` clean on all touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `kernel/event/kinds.go` — `KindPolicyCompacted` const + `knownKinds` registration.
- `kernel/edict/snapshot.go` — `ContentHash()` (+ `crypto/sha256`, `encoding/hex`).
- `kernel/controlplane/edict_overlay.go` — `handleEdictCompact` emits `policy.compacted`.
- `cmd/agezt/main.go` — `replayPolicyOverlay` hash-gated snapshot trust.
- `kernel/edict/snapshot_test.go`, `kernel/controlplane/edict_overlay_test.go` — tests.
