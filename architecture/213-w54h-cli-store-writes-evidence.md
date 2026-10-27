# W5.4h — the CLI writes the daemon's stores through the daemon's writers

## What the slice is

W5.4g made the CLI's file writes **countable** (the forbidden-call ratchet
went from a blind zero to `raw-write cmd/agt 15`). This slice **pays down the
six of the fifteen that are not "a file the operator asked for"** — the ones
that write a file **the daemon reads back**.

The rule that made them visible says it plainly (`tools/archcheck/layers.json`):

> *"The CLI is NOT exempt: a file it writes is a file the daemon may read
> back."*

So the measurement is not "which writes are ugly" but **"which writes land on
a file the daemon owns"**.

## The measurement

All fifteen sites were classified before a line was changed.

| # | Site | What it writes | Who reads it back | Verdict |
|---|---|---|---|---|
| 1 | `doctor_mesh.go:34` | `<home>/memory/memory.json` | **the daemon** (`memory.Open` → `filestore.LoadFrom`) | **real violation** |
| 2 | `doctor_mesh.go:47` | the same store (empty-file repair) | the daemon | **real violation** |
| 3 | `doctor_mesh.go:62` | the same store (corrupt-file repair) | the daemon | **real violation** |
| 4 | `doctor_mesh.go:71` | the same store (BOM normalisation) | the daemon | **real violation** |
| 5 | `doctor_ops_env.go:34` | `<base>/.doctor-probe` | nobody (removed at once) | transient probe |
| 6 | `rollback_catalog.go:90` | `<home>/rollback/checkpoints.json` | **the daemon** (`rollbackstore.LoadAt`) | **real violation** + the fixed-name temp |
| 7 | `plugin_registry_index.go:139` | `<home>/plugins/<binary>` | **the daemon executes it** | **real violation** |
| 8 | `skill_export_cli.go:87` | `<dir>/<skill>.json` | another CLI / a static host | operator output |
| 9 | `skill_export_cli.go:106` | `<dir>/index.json` | another CLI / a static host | operator output |
| 10 | `skill_export_cli.go:243` | `--out` bundle | the operator | operator output |
| 11 | `artifact.go:98` | `--out` artifact | the operator | operator output |
| 12 | `journal_export.go:213` | `--out` journal bundle | `agt journal verify --bundle` | operator output |
| 13 | `peers_fetch_cmd.go:83` | `--out` peer artifact | the operator | operator output |
| 14 | `plugin_new.go:96` | the scaffold's own files | the scaffold itself | **a generator writing its own output** |
| 15 | `backup.go:100` | `--out agezt-backup.tar.gz` | `agt restore` | operator output, **streamed** |

**Six are real violations, one is a stream the platform cannot write yet, and
eight are outputs.** The eight outputs are not in this slice: they are the
subject of the `tools/...` exemption's own reasoning ("a generator IS the
implementation of its own output"), and only site 14 matches that reasoning
exactly. Deciding that for them is the next slice's measurement, not this
one's guess.

## The three fixes

### 1. The memory store → the store framework

The doctor used to write `"{}\n"` with `os.WriteFile` — its own idea of the
store's format, its own mode, and a **truncate-in-place** write. The daemon
writes the same file with `filestore.Save`, which is `internal/atomicfile`
(unique temp + fsync + chmod + rename) at the store framework's `0600`.

Now the doctor writes through the same door:

```go
if err := filestore.Save(path, map[string]json.RawMessage{}); err != nil {
```

and the directory through the framework's own `filestore.EnsureDir` (`0700`,
and it *tightens* an existing one — the doctor's old `MkdirAll(…, 0o755)`
left a store directory world-readable).

Two consequences worth naming:

- **The bytes are now the daemon's bytes.** The CLI writes `{}`; the daemon
  writes `{}`. Before, the CLI wrote `"{}\n"`. Two writers, one file, two
  formats — and the daemon rewrites the file on its next mutation, so the
  CLI's repair was a no-op with a diff attached.
- **The BOM repair now re-serializes.** It reads the records, then writes
  them back through the framework. That is a behaviour change with a real
  risk (a repair that empties the store is a data loss wearing a repair's
  clothes), so a permanent test holds the round trip.

### 2. The rollback catalog → the platform's catalog writer

`cmd/agt` carried its **own copy** of the catalog types *and* its own writer
— with the **fixed-name temp** (`path + ".tmp"` then rename) that
`internal/atomicfile` replaced in M478. Two concurrent writers race on that
one name. The daemon has had the right answer all along in
`kernel/platform/rollbackstore`, and `kernel/webui` was already using it.

The CLI now aliases the platform's types — the same move `kernel/webui` made:

```go
type rollbackCatalog = rollbackstore.Catalog
type rollbackCheckpoint = rollbackstore.Checkpoint
```

and calls the platform's writer:

```go
func writeRollbackCatalogAt(path string, cat rollbackCatalog) error {
	return rollbackstore.WriteAt(path, cat)
}
```

The CLI's **second** atomic writer — its own `writeRollbackFile` — was a
duplicate of the platform's private one, with one difference: it had no
Windows fallback. The platform's is now exported as
`rollbackstore.WriteSnapshotFile`, and both sides use it.

### 3. The plugin binary → the atomic writer

`agt plugin registry --install` writes the binary the **daemon executes**.
`os.WriteFile(dest, data, 0o755)` has two problems: it truncates the target
in place (the daemon can pick up a half-written binary), and **it does not
change the mode of an existing file** — its `perm` argument only applies at
creation. So a re-install over a file whose mode had drifted left the binary
non-executable.

`atomicfile.WriteFile` chmods the temp **before** the rename, so the binary
appears complete and executable, or not at all.

## The ratchet

```
raw-write cmd/agt 15   →   raw-write cmd/agt 9
```

Six sites, each one a file the daemon reads back. The nine that remain are
named in "What remains" below, and they may only go down.

## The proof

### Permanent tests — 10 new

| Test | What it holds |
|---|---|
| `TestTheDoctorRepairWritesTheStoreTheDaemonOpens` | the daemon's own `memory.Open` opens the file the CLI's repair wrote, and sees no records — the CLI's repair is not "some JSON the CLI likes" |
| `TestTheDoctorRepairWritesTheBytesTheDaemonWrites` | the CLI's repaired file is **byte-identical** to the one the daemon's store writer produces (open a store, mutate it, let it snapshot itself) — one file, one format |
| `TestTheDoctorBOMRepairKeepsTheRecords` | normalising the file does not lose what was in it: the record survives, and the BOM is gone |
| `TestTheDoctorRepairLeavesNoTempFileBehind` | the atomic writer's promise: no scratch file next to the store |
| `TestTheCLIAndTheDaemonShareTheRollbackCatalog` | **both directions**: the daemon reads what the CLI wrote, and the CLI reads what the daemon wrote — including the checkpoint *kind*, so the two sides share one vocabulary |
| `TestTheCLIRollbackCatalogSurvivesAStaleTempFile` | a stale scratch name cannot block the write — the fixed-name-temp race (M478) is closed |
| `TestTheCLIRollbackCatalogLeavesNoTempFileBehind` | the catalog directory holds the catalog and nothing else |
| `TestTheInstalledPluginBinaryIsExecutable` | the installed binary carries the executable bit even after the mode drifted — **POSIX only**, and the test says so instead of pretending on Windows |
| `TestTheInstalledPluginBinaryLeavesNoTempFileBehind` | the install directory holds the binary and nothing else |
| `TestWriteSnapshotFileRestoresContentAndMode` | the exported snapshot writer restores the content, creates the parent directory, and leaves no scratch file |

### The mutation run — 11 mutations, 11 caught

Two independent passes with **byte-identical** logs.

The proofs are **two different instruments**, because the slice changed two
different kinds of thing: the *writer* (which the gate sees) and the *bytes*
(which only a test can see).

| Mutation | What it breaks | Caught by |
|---|---|---|
| M01 | the missing-store repair goes back to a plain `os.WriteFile` | the gate reports `raw-write cmd/agt` above its allowance |
| M02 | the empty-store repair goes back to a plain `os.WriteFile` | the gate |
| M03 | the corrupt-store repair goes back to a plain `os.WriteFile` | the gate |
| M04 | the BOM repair writes an empty store instead of the records it read | `TestTheDoctorBOMRepairKeepsTheRecords` |
| M05 | the repaired store is not the bytes the daemon writes — one file, two formats | `TestTheDoctorRepairWritesTheBytesTheDaemonWrites` |
| M06 | the corrupt store is overwritten without a backup — the repair destroys the evidence | `TestCheckMemoryStoreFileRepair` (the existing test) |
| M07 | the BOM repair skips the write — the BOM stays in the store | `TestTheDoctorBOMRepairKeepsTheRecords` |
| M08 | the catalog writer uses a fixed temp name again — the M478 race | the test **and** the gate (it reports `raw-write kernel/platform/rollbackstore`) |
| M09 | the CLI writes an empty catalog — the checkpoint it was asked to record is lost | `TestTheCLIAndTheDaemonShareTheRollbackCatalog` |
| M10 | the CLI and the daemon disagree on a checkpoint kind | `TestTheCLIAndTheDaemonShareTheRollbackCatalog` |
| M11 | the installed plugin binary goes back to a plain `os.WriteFile` | the gate |

**Two findings from the calibration, both named:**

- **M08 was first caught by the gate alone.** The stale-temp test used a
  **read-only file** as the stale scratch name, and on Windows that does not
  block a writer: `CREATE_ALWAYS` *replaces* the file and drops its
  read-only attribute. The test was passing against the broken writer — a
  test that cannot fail is not a test. The stale name is now a **directory**,
  which blocks a fixed-name writer on both platforms (POSIX: `EISDIR`;
  Windows: `ACCESS_DENIED`), and M08 is now caught by the test too.
- The **read-only file** is a tempting fixture and a trap: on POSIX it works,
  on Windows it silently does not. A cross-platform fixture has to be
  cross-platform *in behaviour*, not just in syntax.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | OK — **2 allowlisted import violations and 9 allowlisted call sites remain** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | OK |
| `go run ./tools/docclaimscheck -base origin/main` | OK |
| `go run ./tools/docsindex` | OK |
| `staticcheck ./...` | clean |
| `go test ./... -count=1 -timeout=40m` | green |

## What remains

Nine sites, and they are **not** the same kind of thing as the six that left:

| Sites | What they are |
|---|---|
| 1 — `backup.go:100` | an `os.Create` **stream** (tar + gzip). The atomic writer has no streaming API, so this one needs a **platform addition** first: `atomicfile.WriteStream(path, mode, func(io.Writer) error)` |
| 1 — `doctor_ops_env.go:34` | the doctor's writability **probe** — a transient file nobody reads; `os.CreateTemp` is the right shape for it |
| 7 — the exports | `skill export` (3), `artifact get`, `journal export`, `peers fetch`, `plugin new` |

The exports are the interesting half: they are "a file the operator asked
for", but `skill export`'s `index.json` is read by **another process**, and
`plugin new` is a **generator writing its own output** — the one case where
the `tools/...` exemption's reasoning applies verbatim. The next slice's
measurement should decide between *moving them to the atomic writer* and
*exempting them by name*, and it should say which and why.

And the two import edges are untouched: `controlplane → runtime` (37 verbs, a
redesign) and `selfrepair → runtime` (needs a runs port that does not exist
yet).
