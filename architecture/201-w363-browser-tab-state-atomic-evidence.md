# W3.63 — the browser's tab state is replaced, not rewritten

## What the slice is

W3.62 left the ratchet at **13/4**. This slice takes **one** of those four
sites — `plugins/tools/browser/action_tabs.go:152`, the browser action's **tab
state** — and the ratchet goes **4 → 3** call sites. The import ratchet stays
at **13**: `internal/atomicfile` is a **foundation** package (layer `L0`), so
the browser tool names the platform's writer **directly** and the downward
import costs no edge.

The three sites that remain are the ones the measurement already explained:
`kernel/market/sources.go:151,159` and
`kernel/modules/skills/skill/bundle_io.go:55` write into a **staging
directory** that is then swapped in as a whole, so a torn write there is
already prevented by the directory swap.

## What the file is

The tab state is the browser session's **memory**. One JSON file per session
and tab, at `<sessionRoot>/<session_id>/.agezt-tabs/<tab_id>.json`, holding:

- the tab's **final URL**, and
- the **snapshot refs** a later call can reuse.

It is written by `saveTabState` at the end of every `browser.action` that
produced a final URL, and it is **read back** by later calls:

- `resolveTabURL` reads it to fill in the URL of a `tab_id` the caller
  passed instead of a URL;
- `ResolveTabRef` reads it to turn a `ref` into a selector.

Two facts make the file's integrity matter more than its size suggests:

- **it survives daemon restarts** — it lives under the session root, not in
  memory, so a later process reads what an earlier one wrote;
- **there is no mutex** around the save and the read, so two concurrent
  `browser.action` invocations can interleave.

A write that truncates in place leaves a window in which the state file is
short. A state that does **not** parse fails the next call loudly ("read
browser tab state %q"), which is honest. But a state that **does** parse can
carry a **truncated URL** — and the driver would then navigate to it. That is
the silent kind of failure, and it is the one this slice closes.

## What it did before

```go
// plugins/tools/browser/action_tabs.go
if err := os.WriteFile(p, append(data, '\n'), 0o600); err != nil {
    return fmt.Errorf("write browser tab state %q: %w", strings.TrimSpace(tabID), err)
}
```

The mode has the same problem as everywhere else in this series:
`os.WriteFile` applies its mode only when it **creates** the file, and the
state file survives across calls and restarts, so a leftover **read-only**
state from an earlier run made the next save **fail outright** — opening a
read-only file for writing fails with `EACCES` on POSIX and "Access is
denied" on Windows. The browser would then stop recording the tab at all.

## What it does now

```go
if err := atomicfile.WriteFile(p, append(data, '\n'), 0o600); err != nil {
    return fmt.Errorf("write browser tab state %q: %w", strings.TrimSpace(tabID), err)
}
```

The platform's canonical writer: a unique temp file in the target's
directory, `fsync`, `chmod`, then a rename over the target. So the state is
**replaced**, the mode is **re-asserted** on every write, and a reader always
sees a whole state — the old one or the new one.

## The proof

### Permanent tests

`plugins/tools/browser/tab_state_atomic_test.go` (five tests):

| Test | What it holds |
|---|---|
| `TestTheTabStateIsReplacedNotRewritten` | the state file is **replaced** (the open-handle identity changes) and holds the **new** URL and refs |
| `TestTheTabStateIsWholeWhileAReaderHoldsItOpen` | a reader that already holds the state open reads a **whole** file, and the path holds a whole, parseable state — on both platform outcomes |
| `TestTheTabStateIsWritableByTheirOwner` | a **leftover read-only** state cannot stop the next save, and the result is writable by its owner |
| `TestTheTabStateIsPrivateToTheirOwner` | the state — the URLs the session visited and the refs it captured — is not readable by others (POSIX permission bits) |
| `TestTheTabStatePathStaysUnderTheSessionRoot` | the state file stays inside the session root |

The identity has to be measured through an **open handle**: `os.Stat` on
Windows returns the same identity for a replaced path (the path stat does not
carry the file index), while the handle's own stat does.

The permission-bit test is POSIX-only (`os.PathSeparator != '/'` skips it);
the **owner-writable** guarantee is checked on **both** platforms, because
Windows carries the read-only attribute.

### The mutation run

**6 mutations: 6 caught, 0 survived, 0 build errors, 0 skipped** — in **two**
independent passes, with identical logs.

| # | Mutation | Caught by |
|---|---|---|
| 1 | the tab state goes back to a raw in-place write | `TestTheTabStateIsReplacedNotRewritten` |
| 2 | the tab state is written read-only | `TestTheTabStateIsWritableByTheirOwner` |
| 3 | the tab state is written empty | `TestTheTabStateIsReplacedNotRewritten` |
| 4 | the tab state is written to another path | `TestTheTabStateIsReplacedNotRewritten` |
| 5 | **the saved URL is truncated** | `TestTheTabStateIsReplacedNotRewritten` |
| 6 | **a raw write reappears in the browser tool** | **archcheck** — `raw-write plugins/tools/browser` |

Mutation 5 is the one that names the hazard this slice is about: a state that
still parses but carries a **half URL**, which the driver would then navigate
to. Mutation 6 is **gate-measured** — "the browser writes less" is not
something a test can see, because the file would hold the same bytes.

### The harness's own bug, and what it taught

The **first** pass reported **5 caught, 1 build error** — and the build error
was the **harness's** fault, not the code's. The mutation that reverted the
tab state to a raw write left the `atomicfile` import **unused**, so the
package did not compile — and the harness was supposed to remove that import
as part of the mutation, but its `extra` edit block was **missing from the
script**. The verdict was honest (there *was* a build error), the cause was
not: the harness had not applied the second half of its own mutation.

That is the fourth time this series has run into the same shape, and the
shape is worth naming: **a mutation that leaves an import unused does not
compile, and a harness that does not apply all of its edits measures its own
omission.** The rule is now mechanical: a mutation that reverts a package's
*last* atomic write must also remove the import, and the harness's `extra`
block is part of the mutation, not an afterthought.

## The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 315 packages placed; 13 allowlisted import violations and 3 allowlisted call sites remain (ratchet)** — **call sites down from 4** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `staticcheck ./...` | clean |
| `go test ./... -count=1` | **all green** |

## What remains

The ratchet stands at **13/3**, and **all three** remaining call sites are
**already atomic by another means**:

| Package | Sites | Why it is still there |
|---|---|---|
| `kernel/market` | 2 | the **sync** cache: both files are written into a **staging** directory that is swapped in as a whole (`sources.go:151,159`) |
| `kernel/modules/skills/skill` | 1 | a skill bundle, written into a **staging** directory that is swapped in as a whole (`bundle_io.go:55`) |

So the **call-site** debt is down to three writes that the measurement
already showed to be protected. The allowlist is not emptied by this series —
it is emptied of everything that was **not** already protected.

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
`raw-write` is what hides it — and then the site to be fixed, with a test
where there is none today.

## The rules the slice is really about

**A file that outlives the process that wrote it is a contract.** The tab
state is read by later calls and later processes, and it is the only memory
the session has. A file like that must be **replaced**, because the reader
cannot ask the writer whether it is looking at a whole one.

**A state that still parses can be wrong.** The dangerous torn write is not
the one that fails to parse — that one is loud. It is the one that parses
into a **half URL**, because the driver will happily navigate to it.

**A harness that does not apply all of its edits measures its own
omission.** This is the fourth time a mutation's unused import has produced
a build error instead of a catch, and the fix is mechanical every time: the
mutation removes the import it made unused.
