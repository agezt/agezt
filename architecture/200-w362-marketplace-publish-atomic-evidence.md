# W3.62 — the published marketplace is never half-written

## What the slice is

W3.61 left the ratchet at **13/6**: thirteen allowlisted **import** edges and
**six** allowlisted **call sites**. This slice is the **third call-site slice**,
and it takes **two** of those six, both in `kernel/market`:

| Site | What it writes |
|---|---|
| `kernel/market/publish.go:160` | the **published pack body** (`packs/<name>.json`) |
| `kernel/market/publish.go:193` | the **marketplace index** (`marketplace.json`) |

The ratchet went **6 → 4** call sites. The import ratchet stayed at **13**:
this package already imports the platform's writer (its own `atomicWrite`
helper is a thin wrapper over `internal/atomicfile`), so the slice adds **no**
import at all.

The four sites that remain are the ones the measurement already explained:
`kernel/market/sources.go:151,159` and
`kernel/modules/skills/skill/bundle_io.go:55` write into a **staging
directory** that is then swapped in as a whole, and
`plugins/tools/browser/action_tabs.go:152` is a single browser-session file.

## Why the publish path

`Publish` writes into a directory that is **meant to be served statically** —
that is the whole point of the command. Two parties read what it writes:

- **remote clients**, over HTTP: `kernel/market/sync.go` fetches a remote
  `marketplace.json` and then every pack the index lists;
- **the publisher itself**, on the next publish: it reads the index back to
  merge the new pack into the catalogue.

A write that truncates in place leaves a window in which the file is short.
A client that fetches a half-written index gets invalid JSON and its sync
fails; a client that fetches a half-written pack cannot verify it.

And the publisher's own read-back was the sharp end, because of what it did
with a file it could not parse:

```go
// kernel/market/publish.go — before
if existing, rerr := os.ReadFile(idxPath); rerr == nil {
    _ = json.Unmarshal(existing, &mp) // a corrupt index is overwritten fresh
}
```

The parse error was **ignored**. A half-written index therefore did not fail
the publish — it was treated as "there is no catalogue here", and the index
was written fresh with **only the pack being published**. Every pack
published before it was **silently dropped from the catalogue**: the pack
*files* stayed on disk, but nothing listed them any more, and the operator
was never told. The same thing happened to an index that was corrupted by
anything else.

That is the "silent lie" class this series has been closing: a **failure that
looks like success**. The catalogue is the whole product of a publish, and the
old code could empty it without saying a word.

## What it does now

Both writes go through the package's own `atomicWrite` — a unique temp file,
`fsync`, `chmod`, then a rename over the target:

```go
// kernel/market/publish.go
if err := atomicWrite(filepath.Join(packsDir, p.Name+".json"), packBytes, 0o644); err != nil {
    return err
}
...
return atomicWrite(idxPath, idxBytes, 0o644)
```

And the index read-back **refuses** instead of resetting:

```go
if existing, rerr := os.ReadFile(idxPath); rerr == nil {
    if err := json.Unmarshal(existing, &mp); err != nil {
        return fmt.Errorf("market: the marketplace index %s is not readable JSON; move it aside to start a new catalogue: %w", idxPath, err)
    }
}
```

### The refusal is a behaviour change — and it is deliberate

**Before:** a corrupt index was overwritten "fresh" — the catalogue was
silently emptied and the publish reported success.

**Now:** the publish **fails** and names the file, so the operator can move it
aside (to start a new catalogue) or repair it. The corrupt file is left
exactly as it was, because it is the operator's evidence and their decision
to make.

This is the same rule the W3.59 board slice applied to the control plane's
read fallback, and the same rule W3.61 applied to the swallowed stdin error:
**a broken input must fail the operation that would destroy it, not be
reinterpreted as an empty one.** The only caller of `Publish` is
`cmd/agt/market_publish.go`, and it already surfaces the error.

What this does **not** change: a marketplace directory with **no** index yet
still publishes a fresh one — "no index" is a first publish, not corruption.

What this does **not** fix: two **concurrent** publishes into the same
directory can still lose an entry, because the merge is a read-modify-write
with no lock. The atomic write prevents a torn file; it does not prevent a
lost update. That is a separate concern, and it is named here rather than
claimed away.

## The proof

### Permanent tests

`kernel/market/publish_atomic_test.go` (five tests):

| Test | What it holds |
|---|---|
| `TestThePublishedFilesAreReplacedNotRewritten` | the pack and the index are **replaced** (the open-handle identity changes) and hold the **new** content |
| `TestThePublishedIndexKeepsEveryPublishedPack` | the index is the **catalogue**: publishing a second pack does not drop the first |
| `TestACorruptIndexIsRefused` | a corrupt index **fails** the publish, the corrupt file is left untouched, and the previously published pack survives |
| `TestThePublishedFilesAreWritableByTheirOwner` | a leftover **read-only** file from an earlier publish cannot stop the next publish, and the result is writable by its owner |
| `TestThePublishedFilesAreReadableByOthers` | the published files are readable by others — the directory is **served** (POSIX permission bits) |

The identity has to be measured through an **open handle**: `os.Stat` on
Windows returns the same identity for a replaced path (the path stat does not
carry the file index), while the handle's own stat does.

The permission-bit test is POSIX-only (`os.PathSeparator != '/'` skips it);
the **owner-writable** guarantee is checked on **both** platforms, because
Windows carries the read-only attribute.

### The mutation run

**10 mutations: 10 caught, 0 survived, 0 build errors, 0 skipped** — in **two**
independent passes, with identical logs.

| # | Mutation | Caught by |
|---|---|---|
| 1 | the published pack goes back to a raw in-place write | `TestThePublishedFilesAreReplacedNotRewritten` |
| 2 | the published index goes back to a raw in-place write | `TestThePublishedFilesAreReplacedNotRewritten` |
| 3 | **a corrupt catalogue is silently overwritten again** | `TestACorruptIndexIsRefused` |
| 4 | the index is written empty | `TestThePublishedIndexKeepsEveryPublishedPack` |
| 5 | the pack is written to another path | `TestThePublishedFilesAreReplacedNotRewritten` |
| 6 | the catalogue merge is dropped (the index is not read back) | `TestThePublishedIndexKeepsEveryPublishedPack` |
| 7 | the published index is written read-only | `TestThePublishedFilesAreWritableByTheirOwner` |
| 8 | the published pack is written read-only | `TestThePublishedFilesAreWritableByTheirOwner` |
| 9 | the published pack is written empty | `TestThePublishedFilesAreReplacedNotRewritten` |
| 10 | **a raw write reappears in the marketplace** | **archcheck** — `raw-write kernel/market` |

**One of the ten is gate-measured.** "The marketplace writes less" is not
something a test can see: the files would hold the same bytes. The archcheck
call-site rule is what measures it.

### The first pass was a BUILDERR, not a catch — twice

The first pass of this slice reported **8 caught, 2 build errors**, and both
were the **mutation's** defect, not the code's:

- the mutation that skipped the index write left `idxBytes` **declared and
  not used** — the package did not compile;
- the mutation that wrote the pack empty left `packBytes` unused — same.

A build error is not a catch: it says nothing about the behaviour. Both
mutations now write an **empty slice of the same variable**
(`idxBytes[:0]`, `packBytes[:0]`), so the package compiles and the **test**
does the catching. The two recorded passes are the ones after that fix; the
first pass is not counted.

## The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 315 packages placed; 13 allowlisted import violations and 4 allowlisted call sites remain (ratchet)** — **call sites down from 6** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `staticcheck ./...` | clean |
| `go test ./... -count=1` | **all green** |

## What remains

The ratchet stands at **13/4**, and the four remaining call sites are:

| Package | Sites | Why it is still there |
|---|---|---|
| `kernel/market` | 2 | the **sync** cache: both files are written into a **staging** directory that is swapped in as a whole (`sources.go:151,159`) — already atomic by another means |
| `kernel/modules/skills/skill` | 1 | a skill bundle, written into a **staging** directory that is swapped in as a whole (`bundle_io.go:55`) |
| `plugins/tools/browser` | 1 | a browser action's **tab state** (`action_tabs.go:152`) |

The browser tab state is the last one, and the measurement says what it is: a
JSON file per session and tab, written in place, holding the final URL and the
snapshot refs. It is read back by later `browser.action` calls to reuse a
`tab_id` / `ref`, it **survives daemon restarts** (it lives under the base
dir), and there is **no mutex** around the save and the read — so two
concurrent `browser.action` invocations can interleave. A truncated state
file that still parses would carry a **wrong URL**, and the driver would
navigate to it.

### The blind spot the ratchet cannot see

`cmd/agt/market_publish.go:103` writes an **Ed25519 private key** with
`os.WriteFile`. `cmd/...` is layer `L7` (root), and the root layer is
**exempt** from the forbidden-call rules — so the ratchet cannot see it, and
this slice did not change it.

It is the most valuable raw write in the tree: a **private key** written in
place, where a torn write loses the credential, a wider mode exposes it, and
a crash between the `.key` write and the `.pub` write leaves a key whose
**public half no longer matches** — so every consumer's signature check fails.
It is also **untested**: `cmd/agt` has no test that calls `cmdMarketKeygen` or
`cmdMarketPublish` at all.

Closing it cannot be a normal slice, because the ratchet cannot see it: it
needs the **rule** to change first — the root layer's exemption from
`raw-write` is what hides it — and then the site to be fixed. That is a
worthwhile slice, and it is the one after this.

## The rules the slice is really about

**A published artifact is read by strangers.** The marketplace directory is
served to remote clients, so its files are not the daemon's private state: a
client must never fetch a half-written index or a half-written pack. The
writer's own guarantee and the reader's need are the same guarantee seen from
two sides.

**A file that cannot be parsed must not be reinterpreted as an empty one.**
That reinterpretation is what turned a corrupt index into a silently emptied
catalogue. The refusal is the honest answer: the operator is told, the file is
kept, and the decision stays theirs.

**An atomic write is not a lock.** The merge is a read-modify-write, and two
concurrent publishes can still lose an entry. The atomic write guarantees that
no reader sees a torn file; it says nothing about a lost update, and the
evidence says so rather than claiming the whole problem away.
