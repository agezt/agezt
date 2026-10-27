# W5.4i — the CLI's exports and scaffolds go through the platform's atomic writer

## What the slice is

The last seven "output" write sites in `cmd/agt`. W5.4h paid down the six
that land on a file **the daemon reads back**; these seven write files other
components read: a skill bundle an importer verifies, a registry index a
static host serves, an artifact the operator trusts, a journal bundle another
process verifies, a plugin scaffold the Go toolchain builds.

## The measurement said something that changed the plan

The exemption in `tools/archcheck/layers.json` is a **package** allow list:

```json
{"rule": "raw-write", "allow": ["kernel/platform/filestore/...", "tools/...", "kernel/internal/testfixtures"]}
```

`cmd/agt` is one package (`package main`). So "exempt the generator" — the
one site whose reasoning matches the `tools/...` exemption verbatim
(`plugin new` writes the scaffold, and no component in the platform depends
on those bytes) — would have meant **"exempt all of `cmd/agt`"**, and the
gate would have gone back to the blind zero W5.4g exposed.

**There is no per-site exemption.** The honest routes are: move the write,
or change the gate. This slice moves the writes. A per-site marker in the
gate (a `//archcheck:allow raw-write <reason>` comment the counter honours)
is a separate, small archcheck slice — and it is the right instrument if a
real generator ever needs one.

So the plugin scaffold moved too, and the reason is worth recording: it is
not that the scaffold needed it, it is that **the exemption unit is the
package**, and a package-level exemption would have hidden the six others
with it.

## The seven sites

| # | Site | Who reads it | A half-written file means |
|---|---|---|---|
| 1 | `skill_export_cli.go:87` — the exported bundle | `agt skill import`, `agt skill registry` (a **different process**), a **remote** `agt` over HTTP | the import is **refused** (content address mismatch) — loud |
| 2 | `skill_export_cli.go:106` — the registry index | a **remote** consumer over HTTP | the manifest does not parse — loud |
| 3 | `skill_export_cli.go:243` — the single-skill bundle | `agt skill import` (another process) | the import is refused — loud |
| 4 | `artifact.go:98` — the exported artifact | **nobody verifies it** | a **silently truncated** artifact — the bytes no longer match the daemon's content-addressed object, and nothing says so |
| 5 | `journal_export.go:213` — the journal bundle | `agt journal verify --bundle`, `agt journal import` (other processes) | the chain verification fails — loud |
| 6 | `peers_fetch_cmd.go:83` — the fetched peer artifact | **nobody verifies it** | a **silently truncated** copy of remote bytes — the worst case: untrusted input, no check |
| 7 | `plugin_new.go:96` — the plugin scaffold | the **Go toolchain** (`go build`, `go mod tidy`) | the project does not build — loud |

Two of the seven are **silent**: the artifact and the peer artifact. Nothing
in the tree verifies what was written, so a truncated export is a file that
looks fine and is wrong.

## The ratchet

```
raw-write cmd/agt 9   →   raw-write cmd/agt 2
```

The two that remain are named below, and they are the two that need a
**platform addition** before they can move.

## The behaviour changes, named

Moving to `internal/atomicfile` is not free, and the differences are real:

- **The mode is re-asserted on every write.** `os.WriteFile` applies its
  `perm` argument only when it **creates** the file — an existing file keeps
  whatever mode it had. So an export written world-readable once stayed
  world-readable forever, and a re-export could not tighten it. The atomic
  writer chmods the temp **before** the rename, so the target's mode is the
  one the caller asked for on every write. For the skill bundles this is the
  point: `agt skill export --agent` can lift an agent's **private** skills,
  and `0600` is now a fact rather than a hope.
- **The target is replaced, not rewritten.** The inode changes; a reader
  that already holds the file open keeps the whole old file instead of
  seeing a truncated one.
- **A transient scratch file** appears in the target's directory during the
  write (`.name.<rand>.tmp`) and is removed on every path. A file-sync tool
  or an editor that watches the directory may see it.
- **A symlink at the target path is replaced** rather than written through.
  `os.WriteFile` follows a symlink and writes its target; the atomic writer
  renames over the link itself.
- **The parent directory must already exist** — exactly as before
  (`os.WriteFile` did not create it either).

## The proof

### Permanent tests — 3 new

| Test | What it holds |
|---|---|
| `TestTheCLIRawWritesAreOnlyTheTwoKnownSites` (`cmd/agt/atomic_write_test.go`) | the package's remaining raw writes, **by file**, with the reason each is still there. The GATE counts a *package*; this counts *files* — so a package that stays under the count cannot hide a new raw write in a file the count no longer names |
| `TestThePluginScaffoldIsWholeAndLeavesNoTempFile` | the scaffold holds exactly its four files, no scratch file among them |
| `TestWriteFile_ReassertsTheModeOfAnExistingFile` (`internal/atomicfile/atomicfile_test.go`) | the mode is re-asserted on **every** write, not only at creation — **POSIX only**, and the test says so instead of pretending on Windows |

The file-grained test is the interesting one: it is **strictly stronger**
than the gate for this package, and it makes the ratchet legible — the
table says which two sites are left and why.

### The mutation run — 9 mutations, 9 caught

Two independent passes with **byte-identical** logs.

| Mutation | What it breaks | Caught by |
|---|---|---|
| M01 | the exported skill bundle goes back to a plain `os.WriteFile` | the file-grained test **and** the gate |
| M02 | the registry index goes back to a plain `os.WriteFile` | the file-grained test and the gate |
| M03 | the single-skill bundle goes back to a plain `os.WriteFile` | the file-grained test and the gate |
| M04 | the exported artifact goes back to a plain `os.WriteFile` | the file-grained test and the gate |
| M05 | the exported journal bundle goes back to a plain `os.WriteFile` | the file-grained test and the gate |
| M06 | the fetched peer artifact goes back to a plain `os.WriteFile` | the file-grained test and the gate |
| M07 | the plugin scaffold goes back to a plain `os.WriteFile` | the file-grained test and the gate |
| M08 | the scaffold's bytes are not the template's bytes — a scaffold that is almost right is a scaffold that does not build | `TestPluginNew_ScaffoldsBuildableProject` (the existing test) |
| M09 | the atomic writer uses a fixed temp name again — the M478 race | `TestTheCLIRollbackCatalogSurvivesAStaleTempFile` |

**Three findings from the calibration, all of them the mutation's fault:**

- **A reverted write breaks the build.** Dropping the `atomicfile` call
  leaves the import unused; adding an `os.WriteFile` back leaves the `os`
  import missing. Both are compile errors, and **a compile error is not a
  caught mutation — it is a mutation that never ran**. The harness now
  carries the import fix-ups a reverted write needs, and it checks that a
  fix-up does not remove an import the file still uses.
- **A backtick escape inside a nested quote is a silent trap.** The first
  version of the import anchor was built with ``"`t`"io`"`n…"`` and matched
  **zero** times — and a missed anchor reads as "the mutation does not
  compile", which sends you looking in the wrong place. The anchors are now
  built from `[char]9` and `[char]10`.
- **Cancelling a harness mid-run leaves the tree mutated.** The `finally`
  block that restores the files does not run when the process is killed, and
  the next run then measures a tree that is already broken. Two runs were
  lost this way before it was noticed: the baseline gate caught one, and
  the second showed up as an anchor that matched zero times.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | OK — **2 allowlisted import violations and 2 allowlisted call sites remain** |
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

Two sites, and neither can move without a platform decision first.

| Site | What it needs |
|---|---|
| `backup.go:100` | the backup archive is a **tar+gzip stream**. `internal/atomicfile` has no streaming API: `WriteFile` takes `[]byte`, and a home backup can be large. The platform needs `WriteStream(path, mode, func(io.Writer) error)` — unique temp, fsync, chmod, rename — with the Windows last-resort **copying the temp onto the target** instead of re-writing from memory (a stream's bytes are not in memory). And `backup.go`'s `os.Remove(outPath)` cleanup must **go**: under an atomic API a failed write leaves the previous, good archive intact, and the removal would destroy it |
| `doctor_ops_env.go:34` | the writability **probe** — a transient file nobody reads. `os.CreateTemp` is its right shape. But the honest note: the rule counts only `os.WriteFile` and `os.Create`, so `os.CreateTemp` is **invisible to the gate**. The count drops without a new guarantee being enforced, and the fix is a follow-up slice that **extends the rule** to `os.CreateTemp` and `os.OpenFile(O_CREATE)` — otherwise a zero means "no raw writes the counter can see" rather than "no raw writes" |

And the two import edges are untouched: `controlplane → runtime` (37 verbs, a
redesign) and `selfrepair → runtime` (needs a runs port that does not exist
yet).
