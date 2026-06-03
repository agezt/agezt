# M266 — `agt backup inspect`: read a backup bundle offline

## Why
`agt backup` (M113) writes a portable, secret-free home snapshot and `agt
restore` unpacks it, verifying the journal chain afterward. But there was no way
to look *inside* a bundle before committing to a restore: an operator holding
`agezt-backup.tar.gz` on a fresh host could not confirm which home / journal
head it captured, when, or whether it had been tampered with — short of
restoring it (which refuses a non-empty home and is destructive to set up).
`agt journal verify --bundle` already gives this for a journal export; this
milestone gives the equivalent for the whole-home backup format.

## What
- **`cmd/agt/backup.go`** — new `agt backup inspect <file> [--json]` subcommand:
  - `inspectBackup(r)` reads the gzip+tar archive WITHOUT writing anything,
    returning the manifest and the list of regular-file entries (name + size
    from the tar headers — no file body buffered). The read-only counterpart to
    `restoreBackup`.
  - `cmdBackupInspect` prints the manifest (tool, format version, creation time,
    journal head seq/hash, included subtrees) and the contents (file count, total
    size, a capped per-file listing). Each entry is checked against
    `isAllowedBackupPath` (the same allowlist `restore` enforces); an entry
    outside the known subtrees is flagged `(!) unexpected path` and the command
    exits non-zero with a tamper notice — so a foreign/tampered bundle is caught
    here, before a restore. `--json` emits the manifest + entries for scripting.
  - Routed via a one-line `inspect` check at the top of `cmdBackup`; `backup -h`
    documents it.

## Files
- `cmd/agt/backup.go` — `cmdBackupInspect`, `inspectBackup`, `backupEntry`,
  dispatch + help (edited).
- `cmd/agt/backup_inspect_test.go` — 2 tests (new): a real `createBackup`
  archive round-trips through `inspectBackup` (manifest head + the journal
  segment as a within-subtree entry); a hand-crafted archive with a `secret/…`
  entry is flagged not-OK and `cmdBackupInspect` exits non-zero with a tamper
  notice.

## Verification
- `go test ./cmd/agt/ -run TestBackupInspect` — both green; full suite
  **1855 → 1857** (+2), 68 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on both touched files; `go vet ./cmd/agt/`
  clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.
- **Live-proven**: ran a mock daemon, made a run (journal head seq=15), `agt
  backup` → bundle, then `agt backup inspect` showed `tool: agt (format v1)`,
  the creation time, `journal head: seq=15 hash=124ba9c7a282…` (matching the
  backup output), `includes: journal`, and the `journal/00000001.jsonl` entry
  with its size; `--json` emitted the structured manifest + entries.

## Scope notes
- Pure read-side, additive: no daemon, no new event, no change to the backup
  format. Reuses the existing manifest type, path allowlist, and `humanBytes` /
  `shortHash` helpers.
- Rounds out the backup trio: **make** (`agt backup`, M113) → **inspect**
  (`agt backup inspect`, M266) → **restore** (`agt restore`, M113), mirroring the
  journal export → verify → import trio.
