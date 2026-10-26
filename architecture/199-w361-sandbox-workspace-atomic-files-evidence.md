# W3.61 — the sandbox's workspace files are written atomically

## What the slice is

W3.60 took the control plane's three raw writes and left the ratchet at
**13/10**: thirteen allowlisted **import** edges and **ten** allowlisted
**call sites**. This slice is the **second call-site slice**, and it takes
**four** of those ten sites, in **two** packages:

| Site | What it writes |
|---|---|
| `plugins/tools/codeexec/codeexec_invoke.go:64` | the **entry script** the sandbox runs (`main.py` / `main.js` / `main.ts`) |
| `plugins/tools/codeexec/codeexec_invoke.go:76` | the **extra files** the operator's code reads |
| `plugins/tools/codeexec/codeexec_invoke.go:81` | the **stdin file** the program reads |
| `kernel/executionprofile/secretfiles.go:67` | the **vault secret** the child process reads |

The ratchet went **10 → 6** call sites. The import ratchet stayed at **13**,
because `internal/atomicfile` is a **foundation** package (layer `L0`) and a
downward import costs no edge — the sandbox tool names the platform's writer
**directly**, and so does the execution profile.

The ten sites that were left after W3.60 were measured first, and the
measurement decided the slice. Six of them are **already atomic by another
means** — `kernel/market/sources.go:151,159` and
`kernel/modules/skills/skill/bundle_io.go:55` write into a **staging
directory** that is then swapped in as a whole (`os.RemoveAll(final)` followed
by `os.Rename(staging, final)`), so a torn write there is already prevented.
Moving them would have been ceremony with no crash-atomicity benefit.

That left the four sites of this slice, and they are the four with the
**widest reader distance**: the files they write are read by a **child
process** — or, for the SSH/K8s/Modal/Daytona backends, by **another machine**
that receives them.

## Why these four

**The entry script is executed.** A write that truncates in place leaves a
window in which the file is short, and a short script is not an error the
interpreter can report: it may be a **valid prefix that runs and stops early**.
That is the silent kind of failure — the tool reports success, and the program
did half of what it was asked to do.

**The extra files and the stdin file are read by the program.** A half-written
`stdin.txt` is a program that reads invalid input; and the write error used
to be **swallowed** (`_ = os.WriteFile(...)`), so a program that silently
received no stdin read EOF and behaved as if the operator had typed nothing.

**The secret file is a credential.** `PrepareSecretFileMounts` writes the
**vault's value** into a file whose path is handed to the child process
through the environment (`SECRET_FILE_<KEY>=<path>`; for the container profile
the file is mounted into the container). A torn write there is a **partial
API key** in the child — which reads as an authentication failure, not as a
broken file.

And the **mode** was never the file's: `os.WriteFile` applies its mode only
when it **creates** the file, so the mode on disk was a property of *whoever
wrote last*. A project directory **keeps** its files across calls
(`<home>/sandbox/projects/<slug>`), so a leftover file from an earlier run
kept whatever mode it had — and a leftover **read-only** file made the write
**fail outright**: opening a read-only file for writing fails with `EACCES` on
POSIX and "Access is denied" on Windows.

## What it did before

```go
// plugins/tools/codeexec/codeexec_invoke.go
if err := os.WriteFile(filepath.Join(dir, entry), []byte(in.Code), 0o600); err != nil {
    return errResult("write entrypoint: " + err.Error()), nil
}
...
if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
    return errResult("write " + rel + ": " + err.Error()), nil
}
...
if in.Stdin != "" {
    _ = os.WriteFile(filepath.Join(dir, "stdin.txt"), []byte(in.Stdin), 0o600)
}
```

```go
// kernel/executionprofile/secretfiles.go
if err := os.WriteFile(hostPath, []byte(value), 0o600); err != nil {
    cleanup()
    return nil, func() {}, nil, fmt.Errorf("write secret file mount %q: %w", mount.Key, err)
}
```

## What it does now

All four go through the platform's canonical writer, `internal/atomicfile`: a
unique temp file in the target's directory, `fsync`, `chmod`, then a rename
over the target.

```go
// plugins/tools/codeexec/codeexec_invoke.go
if err := atomicfile.WriteFile(filepath.Join(dir, entry), []byte(in.Code), 0o600); err != nil {
    return errResult("write entrypoint: " + err.Error()), nil
}
...
if err := atomicfile.WriteFile(full, []byte(content), 0o600); err != nil {
    return errResult("write " + rel + ": " + err.Error()), nil
}
...
if in.Stdin != "" {
    if err := atomicfile.WriteFile(filepath.Join(dir, "stdin.txt"), []byte(in.Stdin), 0o600); err != nil {
        return errResult("write stdin: " + err.Error()), nil
    }
}
```

```go
// kernel/executionprofile/secretfiles.go
if err := atomicfile.WriteFile(hostPath, []byte(value), 0o600); err != nil {
    cleanup()
    return nil, func() {}, nil, fmt.Errorf("write secret file mount %q: %w", mount.Key, err)
}
```

Three properties come with the writer, and each is a **guarantee** rather
than an implementation detail:

- **A reader never sees a half-written file.** The rename is atomic, so the
  path carries either the old file or the new one.
- **The file is REPLACED, not rewritten.** A reader that already holds the
  file open keeps reading a whole file.
- **The mode is re-asserted on every write.** The writer chmods the temp file
  before the rename, so a leftover file's mode cannot survive the write — and
  a leftover read-only file cannot stop it.

### The stdin write error is now REPORTED — and that is a behaviour change

The stdin write's error used to be **swallowed**. It is now reported, like
every other write in the same function.

The reason is the same one that makes the other three worth fixing: the
program **reads** that file. A write that fails silently produces a program
that behaves as if the operator had typed nothing — and the tool reports
success. The change is one line wide (`_ = os.WriteFile(...)` →
`if err := atomicfile.WriteFile(...); err != nil { return errResult(...) }`),
and it is a **deliberate** change: a failed stdin write is now a visible
error instead of a silent difference in behaviour.

### The platform's behaviour was measured, not assumed

The writer's behaviour differs by platform, and the tests had to hold on
both. Measured on Windows (the host this slice was developed on), the same
facts W3.60 measured:

| Fact | Measurement |
|---|---|
| A rename over a file **another handle holds open** | **refused** ("Access is denied"); the writer's remove-then-rename fallback is refused too, and the write fails |
| `os.SameFile` on **path** stats across a rename | **true** on Windows — the path stat does not carry the file index, so identity has to come from an **open handle** |
| `os.SameFile` on **open-handle** stats across a rename | **false** — and **true** after a truncate-in-place write |
| `os.WriteFile` over a 0400 file | **fails** ("Access is denied") — so a leftover read-only file really does stop the write |
| `atomicfile.WriteFile` over a 0400 file | **succeeds**, and the result is writable by its owner |
| The permission bits | Windows reports `-rw-rw-rw-` for anything writable and `-r--r--r--` for a read-only file; there is no group/other distinction |

One more measured fact shaped a test: the writer's Windows fallback
**removes** the target before renaming, so a write over an **empty
directory** at the target path *succeeds* (the directory is removed and the
file takes its place). A test that wants to force a write failure has to use
a **non-empty** directory — which is what
`TestAFailedStdinWriteIsReported` does.

The tests therefore observe the writer's **guarantee**, not its mechanism: the
file's **identity** changes when it is rewritten, the file ends up **writable
by its owner**, and the content is **whole**.

## The proof

### Permanent tests

`plugins/tools/codeexec/workspace_files_test.go` (seven tests):

| Test | What it holds |
|---|---|
| `TestTheWorkspaceScriptIsReplacedNotRewritten` | the script is **replaced** (the open-handle identity changes) and the new content is the one the child runs |
| `TestTheWorkspaceScriptIsWholeWhileAReaderHoldsItOpen` | a reader that already holds the script open reads a **whole** file, and the path holds a whole file — on both platform outcomes |
| `TestTheWorkspaceFilesAreWritableByTheirOwner` | a **leftover read-only** script from an earlier run cannot stop the run, and the result is writable by its owner |
| `TestTheWorkspaceFilesArePrivateToTheirOwner` | the script is not readable by others (POSIX permission bits) |
| `TestTheWorkspaceExtraFilesAndStdinAreReplacedNotRewritten` | the extra files and the stdin file are **replaced** too, and hold the new content |
| `TestTheWorkspaceFilesTheChildReadsAreWhole` | the script, the extra file and the stdin file are **whole** at the moment the child reads them (the fake warden stands in for the interpreter) |
| `TestAFailedStdinWriteIsReported` | a failed stdin write is **reported**, not swallowed |

`kernel/executionprofile/secretfiles_atomic_test.go` (three tests):

| Test | What it holds |
|---|---|
| `TestTheSecretFileIsReplacedNotRewritten` | the secret file is **replaced**, and it holds the **new** vault value |
| `TestTheSecretFileIsWritableByTheirOwner` | a **leftover read-only** secret file cannot stop the write, and the result is writable by its owner |
| `TestTheSecretFileIsPrivateToItsOwner` | the secret file is not readable by others (POSIX permission bits) |

The permission-bit tests are POSIX-only (`os.PathSeparator != '/'` skips
them); the **owner-writable** guarantee is checked on **both** platforms,
because Windows carries the read-only attribute — so a mutation that drops the
write bit is caught everywhere.

### The mutation run

**12 mutations: 12 caught, 0 survived, 0 build errors, 0 skipped** — in **two**
independent passes, with identical logs.

| # | Mutation | Caught by |
|---|---|---|
| 1 | the script goes back to a raw in-place write | `TestTheWorkspaceScriptIsReplacedNotRewritten` |
| 2 | the extra files go back to a raw in-place write | `TestTheWorkspaceExtraFilesAndStdinAreReplacedNotRewritten` |
| 3 | the stdin file goes back to a raw in-place write | `TestTheWorkspaceExtraFilesAndStdinAreReplacedNotRewritten` |
| 4 | **the stdin write error is swallowed again** | `TestAFailedStdinWriteIsReported` |
| 5 | the script is written read-only | `TestTheWorkspaceFilesAreWritableByTheirOwner` |
| 6 | the script is written empty | `TestTheWorkspaceScriptIsReplacedNotRewritten` |
| 7 | the script is written to another path | `TestTheWorkspaceScriptIsReplacedNotRewritten` |
| 8 | the secret file goes back to a raw in-place write | `TestTheSecretFileIsReplacedNotRewritten` |
| 9 | the secret file is written read-only | `TestTheSecretFileIsWritableByTheirOwner` |
| 10 | the secret file is written empty | `TestTheSecretFileIsReplacedNotRewritten` |
| 11 | **a raw write reappears in the sandbox tool** | **archcheck** — `raw-write plugins/tools/codeexec: 1 call site(s), 0 allowlisted` |
| 12 | **a raw write reappears in the secret mounts** | **archcheck** — `raw-write kernel/executionprofile: 1 call site(s), 0 allowlisted` |

**Two of the twelve are gate-measured.** "The sandbox writes less" is not
something a test can see: the files would hold the same bytes. The archcheck
call-site rule is what measures it.

### The first run found a real test gap — and that is why it is not reported

The **first** pass of this slice reported **10 caught, 2 survived**. Both
survivals were the harness being right and the **test** being wrong:

- The two mutations that reverted the **extra files** and the **stdin file**
  to in-place writes were not caught by
  `TestTheWorkspaceFilesTheChildReadsAreWhole`, because that test asserts
  the **content** — and the content is whole either way. A reader that
  arrives *after* the write cannot tell a rename from a truncate.

The guarantee lives in the file's **identity**, not in its bytes, so the fix
was a test that measures the identity:
`TestTheWorkspaceExtraFilesAndStdinAreReplacedNotRewritten`. The two recorded
passes are the ones **after** that test was added; the first pass is not
counted.

## The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 315 packages placed; 13 allowlisted import violations and 6 allowlisted call sites remain (ratchet)** — **call sites down from 10** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `staticcheck ./...` | clean |
| `go test ./... -count=1` | **all green** |

## What remains

The ratchet stands at **13/6**, and the **six** remaining call sites are the
whole of the remaining raw-write debt:

| Package | Sites | Why it is still there |
|---|---|---|
| `kernel/market` | 4 | two of them write into a **staging** directory that is swapped in (`sources.go:151,159`); the other two are the **published** pack and the marketplace index (`publish.go:160,193`) |
| `kernel/modules/skills/skill` | 1 | a skill bundle, written into a **staging** directory that is swapped in (`bundle_io.go:55`) |
| `plugins/tools/browser` | 1 | a browser action's tab state (`action_tabs.go:152`) |

The `kernel/market` **publish** path is the interesting one for the next
slice, and the measurement says why: the **sync** path (the download) already
stages and swaps, but the **publish** path (the upload) writes the pack and
the index **in place** — into a directory that is meant to be **served
statically** to remote clients (`kernel/market/sync.go` fetches a remote
`marketplace.json` over HTTP). And the writer reads the index back at
`publish.go:167` while **ignoring the parse error**:

```go
if existing, rerr := os.ReadFile(idxPath); rerr == nil {
    _ = json.Unmarshal(existing, &mp) // a corrupt index is overwritten fresh
}
```

A torn index therefore does not fail the publish — it **resets** the
catalogue, and every previously published pack is silently dropped. That is
a "silent lie" in the same family as the ones this series has been closing,
and it is the next slice to measure.

### The blind spot the ratchet cannot see

`cmd/agt/market_publish.go:103` writes an **Ed25519 private key** with
`os.WriteFile`. `cmd/...` is layer `L7` (root), and the root layer is
**exempt** from the forbidden-call rules — so the ratchet cannot see it, and
this slice did not change it.

It is worth naming because it is the most valuable raw write in the tree: a
**private key** written in place, where a torn write loses the credential, a
wider mode exposes it, and a crash between the `.key` write and the `.pub`
write leaves a key whose **public half no longer matches** — so every
consumer's signature check fails. It is a separate slice, because the fix
belongs to the `market keygen` path.

## The rules the slice is really about

**A file that another PROCESS reads must be REPLACED, not rewritten.** The
script is executed by an interpreter, the secret is read by a child, and for
the remote backends the workspace is read by another machine. A rename is the
only write that gives them one of two whole files.

**A mode that is only applied on CREATE is not a mode.** The mode on disk was
a property of *whoever wrote last*, and a project workspace keeps its files —
so a leftover read-only script stopped the sandbox from running the
operator's code at all. The platform's writer re-asserts the mode on every
write, which is what makes "the workspace is 0600" a property of the **code**
instead of a property of the filesystem's history.

**A swallowed write error is a silent behaviour change.** The stdin write's
error was discarded, so a program that received no stdin looked exactly like
a program whose operator had typed nothing. Every write in that function now
reports its failure, because the file is **read** by the program.

**The content check cannot tell a rename from a truncate.** The mutation run
proved it: a test that asserts the bytes a reader sees passes for both. The
guarantee lives in the file's **identity**, so that is what the test has to
measure.
