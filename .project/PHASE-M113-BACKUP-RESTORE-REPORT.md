# Phase Report — Milestone M113 (`agt backup` / `agt restore`)

> Status: **shipped** · Date: 2026-06-02 · SPEC-09 §8 (backup/restore).

## Why

M101–M103 made the journal exportable/importable/verifiable. The headline
SPEC-09 capability is one-command, secret-free node migration: snapshot a home
to a portable archive and restore it on a fresh host. Over journal export/import
it adds the **catalog** (network-synced pricing, not in the journal, so a
migrated node works without re-syncing) and a single `.tar.gz` artifact.

## What shipped

- **`agt backup [--home <dir>] [--out <file>]`** — a gzip+tar of the home,
  capturing `journal/` + `catalog/` with a `backup-manifest.json` (journal head
  at backup time). Verifies the journal chain first (won't archive a corrupt
  one). Offline; default output `agezt-backup.tar.gz`.
- **`agt restore <file> [--home <dir>]`** — unpacks into an EMPTY home (refuses
  an existing journal), then confirms the restored journal boots + verifies.
- **Pure core** — `createBackup` / `restoreBackup` / `isAllowedBackupPath`,
  unit-tested without a daemon.

## Security

- **Secrets excluded by construction.** Only `journal/` and `catalog/` are
  captured; `creds.json` (home root) and `runtime/control.token` live OUTSIDE
  those subtrees, so a backup *cannot* leak them — asserted by a test that fails
  if `creds.json` ever appears in the archive. Restore reminds the operator to
  re-provision credentials.
- **Path-traversal safe.** Every archive entry must sit within a known include
  subtree and resolve under the destination home; a `..` / absolute / unknown
  entry is refused (zip-slip protection), with a dedicated test.

## Tests

- `TestBackupRestore_RoundTrip` — backup a home (journal+catalog+secret) → the
  archive omits the secret, includes the segment → restore into a fresh home →
  the journal verifies, head matches, catalog restored, no creds.json.
- `TestIsAllowedBackupPath` — allows the two subtrees; rejects traversal /
  absolute / secret / runtime paths.
- `TestRestore_RejectsTraversalArchive` — a hand-crafted `../escape` archive is
  refused.

Test count: **1375 → 1378**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt backup --home <src> --out node.tar.gz
backed up journal + catalog → node.tar.gz   (journal head seq=14)
$ tar tzf node.tar.gz
backup-manifest.json
catalog/custom.json
journal/00000001.jsonl                      # creds.json ABSENT
$ agt restore node.tar.gz --home <fresh>
restored journal + catalog ; journal head seq=14
$ agt restore node.tar.gz --home <fresh>    # again
… already has a journal — restore only into an empty home   (exit 1)
$ AGEZT_HOME=<fresh> agezt & ; agt runs list → the migrated run survives
```
