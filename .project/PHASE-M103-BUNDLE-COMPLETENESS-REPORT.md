# Phase Report — Milestone M103 (bundle completeness / anti-truncation)

> Status: **shipped** · Date: 2026-06-01 · SPEC-09 §8 hardening.

## Why

M101's `agt journal verify --bundle` recomputes every event's BLAKE3 hash and
checks prev-hash continuity — proving the events present are untampered and
gap-free. But it never confirmed the bundle **reaches** the chain head its own
manifest attests. So an adversary could **drop the tail** of a full export: the
surviving prefix still chain-verifies, and the bundle reports "OK" while silently
omitting the most recent (often most interesting) events. For an audit/archival
format that is a real omission attack, and for `import` it means a truncated
backup would restore an incomplete history undetected.

## What shipped

- **`checkBundleCompleteness(events, manifest)`** — a pure check wired into both
  `agt journal verify --bundle` and `agt journal import`. Because an export
  streams every event up to the head read at the same instant, the LAST bundle
  event must BE that head; its hash is cryptographically bound, so
  `last.Hash == manifest.head_hash` proves nothing was truncated. Seq
  cross-checks (`first==first_seq`, `last==last_seq==head_seq`) give a clearer
  message; an empty `head_hash` (legacy/pre-genesis) skips the crypto check.
- Both surfaces now reject a tail-truncated or forged-head bundle with
  `bundle INCOMPLETE: …` — `import` does so **before touching disk**.

## Design notes

- The cryptographic anchor is `last.Hash == head_hash`. The seq checks are for
  legibility; the hash check is the guarantee (a forged `last_seq` without the
  matching head hash is still caught).
- This composes cleanly with the existing per-event + continuity verification:
  prefix integrity (M101) + completeness-to-head (M103) together mean a bundle
  that verifies is provably the WHOLE attested chain slice, untampered.

## Tests

- `TestCheckBundleCompleteness` — complete bundle OK; tail-truncated → error
  (the omission attack); forged manifest `head_hash` → error; empty-vs-nonzero-
  count → error; genuinely empty → OK.

Test count: **1356 → 1357**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt journal verify --bundle full.json        → bundle OK: 15 event(s) verified (seq 0..14)
$ # drop the last event, leave the manifest attesting the original head:
$ agt journal verify --bundle full.json
agt journal verify: bundle INCOMPLETE: last event seq 13 != manifest last_seq 14 (bundle truncated?)  (exit 1)
$ agt journal import full.json --home /restored
agt journal import: bundle INCOMPLETE: last event seq 13 != manifest last_seq 14 (bundle truncated?)  (exit 1)
```
