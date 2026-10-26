# W3.60 — the control plane's files are written atomically

## What the slice is

W3.59 left the ratchet at **13/13**: thirteen allowlisted **import** edges and
thirteen allowlisted **call sites**. The import debt is now the whole of the
remaining reach, but the second counter was still there — and it is the one that
names **raw writes**.

`tools/archcheck`'s `raw-write` rule counts every `os.WriteFile` / `os.Create`
call outside the platform layer. At the start of this slice there were **13**
such call sites in **six** packages:

| Package | Sites | What it writes |
|---|---|---|
| `kernel/controlplane` | 3 | the daemon's address + token files, the chat prompt library |
| `kernel/market` | 4 | the marketplace's pack files and its index |
| `plugins/tools/codeexec` | 3 | the sandbox's entry file, the script and stdin |
| `kernel/executionprofile` | 1 | a host secret file |
| `kernel/modules/skills/skill` | 1 | a skill bundle |
| `plugins/tools/browser` | 1 | a browser action's tab state |

This slice takes the **three** control-plane sites. The ratchet went
**13 → 10** call sites; the import ratchet stayed at **13**, because
`internal/atomicfile` is a **foundation** package (layer `L0`) and a
downward import costs no edge.

This is the **first call-site slice**. Until now every slice moved an import
edge and left the call sites untouched — and the call sites are the ones that
write **without** the platform's guarantees.

## Why the control plane's three

The three sites are one family: they are the daemon's **identity on disk** and
the operator's **own text**.

- `runtime/control.addr` is the address a client dials; `runtime/control.token`
  is the daemon's **primary admin token** — it authorizes everything, on any
  tenant. A client reads both (`kernel/controlplane/client.go`), the SDK reads
  them (`sdk/sdk.go`), the CLI reads them (`cmd/agt/dial`, `cmd/agt/doctor`),
  and the Web UI proxy reads them.
- `chat_prompts.json` is the operator's saved prompt library. It is read back on
  **every** Chat view, and `kernel/app/persona/persona.go` drops an
  **unparseable** library rather than failing the read — so a torn file does not
  surface as an error, it surfaces as an **empty library**.

That second one is the sharp end: a truncate-in-place write leaves a window in
which the file is short, the short file does not parse, and the operator's
prompts are **gone with no reason given**. The control plane's own reader is
the reason this is a defect and not a theoretical one.

The other eight sites were left for later slices on purpose: `kernel/market`
and `kernel/modules/skills/skill` already write into a **staging directory** and
swap it in (`kernel/market/sources.go:151,159`, `kernel/modules/skills/skill/bundle_io.go:55`),
so a torn write there is already prevented by the directory swap — moving them
would be ceremony with no crash-atomicity benefit.

## What it did before

```go
// kernel/controlplane/server_lifecycle_internal.go
if err := os.WriteFile(filepath.Join(dir, addrFile), []byte(addr+"\n"), 0o600); err != nil {
    return fmt.Errorf("controlplane: write addr file: %w", err)
}
if err := os.WriteFile(filepath.Join(dir, tokenFile), []byte(s.token+"\n"), 0o600); err != nil {
    return fmt.Errorf("controlplane: write token file: %w", err)
}
```

```go
// kernel/controlplane/app_operations.go
SavePrompts: func(data []byte) error { return os.WriteFile(prompts, data, 0o600) },
```

`os.WriteFile` opens the target with `O_TRUNC` and writes into it. Two things
follow, and both were measured on this codebase:

- **A reader can see a torn file.** The window between the truncate and the
  last write is real, and the reader in question is another process (the CLI,
  the SDK, the Web UI) that reads the file while the daemon starts.
- **The mode is applied only on CREATE.** `os.WriteFile` applies its mode when
  it creates the file; a **leftover** file from an earlier run keeps whatever
  mode it had. A leftover **read-only** file (0400) makes the daemon's own
  write **fail outright** — measured: `os.WriteFile` over a 0400 file on
  Windows answers "Access is denied".

## What it does now

Both writes go through the platform's canonical writer, `internal/atomicfile`:
a unique temp file in the target's directory, `fsync`, `chmod`, then a rename
over the target.

```go
// kernel/controlplane/server_lifecycle_internal.go
if err := atomicfile.WriteFile(filepath.Join(dir, addrFile), []byte(addr+"\n"), 0o600); err != nil {
    return fmt.Errorf("controlplane: write addr file: %w", err)
}
if err := atomicfile.WriteFile(filepath.Join(dir, tokenFile), []byte(s.token+"\n"), 0o600); err != nil {
    return fmt.Errorf("controlplane: write token file: %w", err)
}
```

```go
// kernel/controlplane/app_operations.go
SavePrompts: func(data []byte) error { return atomicfile.WriteFile(prompts, data, 0o600) },
```

Three properties come with it, and each is a **guarantee** rather than an
implementation detail:

- **A reader never sees a half-written file.** The rename is atomic, so the
  path carries either the old file or the new one.
- **The file is REPLACED, not rewritten.** A reader that already holds the file
  open keeps reading a whole file.
- **The mode is re-asserted on every write.** The writer chmods the temp file
  before the rename, so a leftover file's mode cannot survive the write — and a
  leftover read-only file cannot stop the write.

### The writer is a foundation package, so this costs no import debt

`internal/...` is layer `L0` in `tools/archcheck/layers.json`: every layer may
import it, and a downward import is not an edge. The control plane therefore
names the platform's writer **directly** — not through `kernel/platform/filestore`
— and the import ratchet stays at **13**. The repo already had the precedent
(`kernel/market/store.go`, `kernel/settings/store.go`, `kernel/auth/tokenfile.go`,
`plugins/tools/file/checkpoint.go`).

### The platform's behaviour was measured, not assumed

The writer's behaviour differs by platform, and the tests had to hold on both.
Measured on Windows (the host this slice was developed on):

| Fact | Measurement |
|---|---|
| A rename over a file **another handle holds open** | **refused** ("Access is denied"); the writer's remove-then-rename fallback is refused too, and the write fails |
| `os.SameFile` on **path** stats across a rename | **true** on Windows — the path stat does not carry the file index, so identity has to come from an **open handle** |
| `os.SameFile` on **open-handle** stats across a rename | **false** — and **true** after a truncate-in-place write |
| `os.WriteFile` over a 0400 file | **fails** ("Access is denied") |
| `atomicfile.WriteFile` over a 0400 file | **succeeds**, and the result is writable by its owner |
| The permission bits | Windows reports `-rw-rw-rw-` for anything writable and `-r--r--r--` for a read-only file; there is no group/other distinction |

The tests therefore observe the writer's **guarantee**, not its mechanism: the
file's **identity** changes when it is rewritten, the file ends up **writable
by its owner**, and the content is **whole**. The Windows refusal to replace a
file a reader holds open is **not** a regression introduced by this slice — it
is the platform writer's existing behaviour, shared with `kernel/auth/tokenfile.go`
— and the invariant both platforms share is the one a reader depends on: the
held handle reads a **complete** file, and the path holds a **complete** file.

## The proof

### Permanent tests

`kernel/controlplane/runtime_files_test.go` (seven tests):

| Test | What it holds |
|---|---|
| `TestTheRuntimeFilesAreReplacedNotRewritten` | the address and token files are **replaced** (the open-handle identity changes), and the new content is the one the daemon serves |
| `TestTheRuntimeFilesAreReplacedWhileAReaderHoldsThemOpen` | a reader that already holds the file open reads a **whole** file, and the path holds a whole file — on both platform outcomes |
| `TestTheRuntimeFilesAreWritableByTheirOwner` | a **leftover read-only** file from an earlier run cannot stop the daemon's own write, and the result is writable by its owner |
| `TestTheRuntimeTokenIsPrivateToItsOwner` | the primary token is not readable by others (POSIX permission bits; skipped where the bits do not exist) |
| `TestThePromptLibraryIsReplacedNotRewritten` | the prompt library is **replaced**, and the new library is the whole, parseable one |
| `TestThePromptLibraryIsWritableByItsOwner` | a leftover read-only library cannot stop the save, and the result is writable by its owner |
| `TestThePromptLibraryIsPrivateToItsOwner` | the library is not readable by others (POSIX permission bits) |

The two permission-bit tests are POSIX-only (`os.PathSeparator != '/'` skips
them); the **owner-writable** guarantee is checked on **both** platforms, because
Windows carries the read-only attribute — so a mutation that drops the write
bit is caught everywhere.

### The mutation run

**12 mutations: 12 caught, 0 survived, 0 build errors, 0 skipped** — in **two**
independent passes, with identical logs.

| # | Mutation | Caught by |
|---|---|---|
| 1 | the address file goes back to a raw in-place write | `TestTheRuntimeFilesAreReplacedNotRewritten` |
| 2 | the token file goes back to a raw in-place write | `TestTheRuntimeFilesAreReplacedNotRewritten` |
| 3 | the prompt library goes back to a raw in-place write | `TestThePromptLibraryIsReplacedNotRewritten` |
| 4 | the address file is written read-only | `TestTheRuntimeFilesAreWritableByTheirOwner` |
| 5 | the token file is written read-only | `TestTheRuntimeFilesAreWritableByTheirOwner` |
| 6 | the prompt library is written read-only | `TestThePromptLibraryIsWritableByItsOwner` |
| 7 | the address is written without its trailing newline | `TestTheRuntimeFilesAreReplacedNotRewritten` |
| 8 | the token is written without its trailing newline | `TestTheRuntimeFilesAreReplacedNotRewritten` |
| 9 | the address is written to the wrong path | `TestTheRuntimeFilesAreReplacedNotRewritten` |
| 10 | the prompt library is written to the wrong path | `TestThePromptLibraryIsReplacedNotRewritten` |
| 11 | the address file is written empty | `TestTheRuntimeFilesAreReplacedNotRewritten` |
| 12 | **a raw write reappears in the control plane** | **archcheck** — `raw-write kernel/controlplane: 1 call site(s), 0 allowlisted` |

**One of the twelve is gate-measured.** "The control plane writes less" is not
something a test can see: the files would hold the same bytes. The archcheck
call-site rule is what measures it.

**The rest measure a behaviour, not a shape:** the reader that must not see a
torn token, the leftover read-only file that must not stop the daemon's own
write, the mode that must not survive from an earlier run, the path that must
be the same after a restart, and the content that must be whole.

### The harness had to learn two things

The **first** run of this slice reported **10 caught, 1 survived, 1 build
error**, and both verdicts were the **harness's** fault, not the code's:

1. **A wrong test name is not a survival.** One mutation named a test that does
   not exist (`…ByTheirOwner` for the prompt library, which is
   `…ByItsOwner`). `go test -run` answered `no tests to run` and **exited 0**,
   so the harness read a survival where **no test had run at all**. The harness
   now classifies `no tests to run` as its own verdict (**NO-TESTS**) and
   refuses to call it a survival.
2. **A build error is not a catch.** The mutation that turned the prompt
   library back into a raw write left the `atomicfile` import **unused**, so
   the package did not compile. That is a **BUILDERR** — the mutation's own
   defect, not a catch. The mutation now removes the import as well, so the
   package compiles and the **test** does the catching.

Both were fixed before the two recorded passes, and both passes are reported
here; the first run is not.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 315 packages placed; 13 allowlisted import violations and 10 allowlisted call sites remain (ratchet)** — **call sites down from 13** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `staticcheck ./...` | clean |
| `go test ./... -count=1` | **all green** |

## What remains

The ratchet stands at **13/10**, and the **10** remaining call sites are the
whole of the remaining raw-write debt:

| Package | Sites | Why it is still there |
|---|---|---|
| `kernel/market` | 4 | two of them write into a **staging** directory that is swapped in (`sources.go:151,159`); the other two are the published pack and index |
| `plugins/tools/codeexec` | 3 | the sandbox's entry file, script and stdin, written under a per-run directory |
| `kernel/executionprofile` | 1 | a host secret file (`secretfiles.go:67`) |
| `kernel/modules/skills/skill` | 1 | a skill bundle, written into a **staging** directory that is swapped in (`bundle_io.go:55`) |
| `plugins/tools/browser` | 1 | a browser action's tab state (`action_tabs.go:152`) |

### The blind spot the ratchet cannot see

`cmd/agt/market_publish.go:103` writes an **Ed25519 private key** with
`os.WriteFile`. `cmd/...` is layer `L7` (root), and the root layer is **exempt**
from the forbidden-call rules — so the ratchet cannot see it, and this slice
did not change it.

It is worth naming because it is the most valuable raw write in the tree: a
**private key** written in place, where a torn write loses the key and a wider
mode exposes it. It is a separate slice, because the fix belongs to the
`market keygen` path, not to the control plane.

## The rules the slice is really about

**A file that is read by another process must be REPLACED, not rewritten.**
The control plane's files are read by the CLI, the SDK and the Web UI — three
other processes, none of them under the daemon's lock. A rename is the only
write that gives them one of two whole files.

**A mode that is only applied on CREATE is not a mode.** `os.WriteFile` applies
its mode when it creates the file and never again, so the mode of a file on
disk is a property of **whoever wrote it last**, not of the writer. The
platform's writer re-asserts the mode on every write, which is what makes
"the token file is private" a property of the **code** instead of a property of
the **filesystem's history**.

**A reader that swallows a parse error turns a torn write into a lie.** The
prompt library's reader drops an unparseable file; without the atomic write
that is not a corrupt file but an **empty library** with no error anywhere. The
writer's guarantee and the reader's honesty are the same guarantee seen from
two sides.

**The platform's behaviour has to be measured, not assumed.** The writer's
Windows behaviour (a refused rename over an open file, a path stat that does
not carry the file index) was measured on this codebase before the tests were
written, and the tests hold on **both** platforms because they assert the
guarantee rather than the mechanism.
