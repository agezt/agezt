# W5.4a — the forbidden-call ratchet is zero

## What the slice is

W5.4 asks for two ratchets at zero: the **import** allowlist and the
**forbidden-call** allowlist. This slice takes the **call-site** one — the
last **three** `raw-write` call sites — and the import allowlist (seven edges)
and the docs archiving stay.

The three sites were the marketplace cache's two staged writes and the skill
bundle's staged write:

```
raw-write kernel/market                2
raw-write kernel/modules/skills/skill  1
```

## What the measurement said, and where it was half right

Two earlier slices measured these three sites and called them **"already
atomic by another means"** — they are written into a **staging directory**
that is swapped in as a whole, so a reader never sees a half-written file.
That was **half right**, and the half that was wrong was the half that
mattered:

1. **The staging writes were not fsynced.** `os.WriteFile` writes and closes;
   the bytes reach the disk whenever the OS gets to them. A tree that is
   swapped in can therefore hold a file whose content was never durable — and
   a power loss right after the swap leaves a **half-written file inside a
   published tree**, which is exactly the failure the staging was supposed to
   prevent.

2. **The swap deleted the old tree before the new one was in place.** A
   directory cannot be renamed over another directory, so the code did what
   the platform requires:

   ```go
   os.RemoveAll(final)          // the old tree is GONE from here on
   os.Rename(staging, final)    // and if THIS fails, the caller has nothing
   ```

   The window between the two calls is tiny, but the failure is not rare in
   the way a window is: on **Windows** a rename is refused while a scanner
   (an indexer, an antivirus, a reader without `FILE_SHARE_DELETE`) still
   holds a handle on a file in the tree — and the old tree has **already
   been deleted** at that point. The content was deleted, not moved: it
   cannot be recovered, and the operator is left with a cache (or a skill
   bundle) that is simply **gone**.

Both holes were in the *policy*, not in the calls: two packages had each
grown their own version of "delete, then rename".

## What changed

### One shared policy: `internal/dirswap`

A new **foundation** package (`internal/...` is layer `L0`, so naming it costs
no import edge) owns the whole tree publish:

1. The new tree is written into a **staging sibling**. Every file goes
   through the platform's atomic writer (`internal/atomicfile`: unique temp →
   write → **fsync** → chmod → rename), so a reader never sees a half-written
   file and the content is durable before the tree is published. The parent
   directory is fsynced afterwards (best effort — a directory handle cannot
   be synced on Windows, and that guarantee is for the deploy target).
2. The old tree is **moved aside** — a **rename**, never a delete. It is
   still there, under another name, for the whole swap.
3. The staged tree is renamed into place.
4. Only then is the old tree removed, and the destination's directory entry is
   fsynced so the published tree survives a power loss.

If step 3 fails, the old tree is **moved back** and the error is returned:
**the destination is never left missing**. The leftover of a failed cleanup is
bounded to **one** path per destination, which the next swap removes — a
destination can never accumulate more than one.

The **Windows retry** — a rename that is refused while a scanner holds a
handle is retried, because the handle is released in milliseconds and a
hard failure would mean the built-in skills are not seeded — moved into the
shared policy with it. The skill package's own `renameBundle` is **gone**:
one implementation of the retry, not two.

### The two callers

- `kernel/market` (`SaveMarketplace`): the staging tree and its two writes go
  through the policy, and the swap is `dirswap.Swap`. The listing now skips
  the policy's working directories through `dirswap.IsTransient`, so a
  leftover is never offered as a published marketplace.
- `kernel/modules/skills/skill` (`BundleStore.Write`): the same, and the
  staging tree is removed on every path (a successful swap renames it away,
  so the removal is a no-op then).

## The proof

### Permanent tests

**Fourteen**, in three packages: eleven over the policy itself, three over
the two callers.

| Test | What it holds |
|---|---|
| `TestTheSwapPublishesTheStagedTree` | the staged tree becomes the destination and the tree it replaced is gone |
| `TestTheSwapKeepsTheOldTreeWhenThePublishFails` | **the guarantee**: a publish that fails leaves the destination exactly as it was, and the rollback leaves no tree behind |
| `TestTheSwapPublishesATreeThatDidNotExistBefore` | the first publish creates the destination |
| `TestTheSwapLeavesNoLeftoverBehind` | the tree the swap replaced is removed, and does not linger next to the destination |
| `TestTheSwapClearsTheLeftoverOfAnEarlierAttempt` | the leftover of an earlier attempt is cleared, so a destination never accumulates more than one |
| `TestTheSwapRefusesToPublishAPathOntoItself` | the guard: the same path twice is refused, and the tree is untouched |
| `TestTheStageClearsTheLeftoverOfAnEarlierAttempt` | a staging tree from an interrupted attempt is removed, so a new attempt never publishes half of the old one |
| `TestTheWritePublishesTheWholeFile` | the file that lands is the file that was written |
| `TestTheWriteReassertsTheMode` | the mode is the FILE's, not the property of whoever wrote last — a leftover read-only file cannot stop the next write |
| `TestTheWriteLeavesNoTempFileBehind` | the writer's temp file does not stay in the published tree |
| `TestATransientNameIsNotAPublishedTree` | a reader that lists a directory skips the working directories |
| `TestSaveMarketplaceKeepsThePreviousCacheWhenThePublishFails` | a failed publish leaves the cached marketplace exactly as it was, and the listing offers the published one only |
| `TestSaveMarketplaceReplacesTheWholeCache` | a successful publish replaces the tree — the previous catalogue's packs are gone, not merged in |
| `TestBundleStoreKeepsThePreviousBundleWhenTheWriteFails` | a write that fails leaves the published bundle intact **and** leaves no staging tree behind |
| `TestBundleStorePublishesTheWholeBundle` | a successful write publishes every file |

Two of them are **measurements** rather than assertions of a shape:

- The **mode** is asserted as "writable" everywhere and as exactly `0600`
  only where POSIX permission bits exist — Windows has none (a writable file
  reports `0666`, a read-only one `0444`), and a test that asserted `0600`
  there would fail on the platform the developer uses and pass on the one the
  deploy uses.
- The **failure** the skill's test provokes is a file and a directory with the
  same name. Whichever of the two is written first, the other cannot be
  written — so the test fails on every platform and in every map order. (The
  first version of that test made the collision with the **published** bundle
  instead, which cannot happen: the staging tree is new and empty. It passed
  on a wrong premise, and the premise is what was fixed.)

### The mutation run

| # | Mutation | Caught by |
|---|---|---|
| 1 | the **rollback** is skipped, so a failed publish leaves the destination empty | `TestTheSwapKeepsTheOldTreeWhenThePublishFails` |
| 2 | the old tree is **deleted** instead of moved aside, so it cannot be restored | `TestTheSwapKeepsTheOldTreeWhenThePublishFails` |
| 3 | the **leftover** of an earlier swap is not cleared | `TestTheSwapClearsTheLeftoverOfAnEarlierAttempt` |
| 4 | the tree the swap replaced is **never removed** | `TestTheSwapLeavesNoLeftoverBehind` |
| 5 | the write **skips the platform writer**, so the mode is not re-asserted | `TestTheWriteReassertsTheMode` |
| 6 | the staging tree of an earlier attempt is not cleared | `TestTheStageClearsTheLeftoverOfAnEarlierAttempt` |
| 7 | the old tree is **never moved aside**, so the rename over it fails | `TestTheSwapPublishesTheStagedTree` |
| 8 | the marketplace cache is **never published** | `TestSaveMarketplaceKeepsThePreviousCacheWhenThePublishFails` |
| 9 | the cached marketplace index is written **under the wrong name** | `TestSaveMarketplaceKeepsThePreviousCacheWhenThePublishFails` |
| 10 | the packs directory is **never created**, so the packs cannot be written | `TestSaveMarketplaceKeepsThePreviousCacheWhenThePublishFails` |
| 11 | the bundle is **never published** | `TestBundleStoreKeepsThePreviousBundleWhenTheWriteFails` |
| 12 | the staging tree of a failed write is **left behind** | `TestBundleStoreKeepsThePreviousBundleWhenTheWriteFails` |

**12 mutations, 12 caught**, in two independent passes with **byte-identical**
logs.

The **first** pass reported one BUILDERR, and it was the **mutation's**
defect, not the code's: skipping the index write left `idx` **declared and
not used**, and a build error says nothing about the behaviour. The mutation
now writes the index under the wrong name — a real defect the test can catch.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 315 packages placed; 7 allowlisted import violations and 0 allowlisted call sites remain (ratchet)** — **call sites down from 3** |
| `go run ./tools/deadcodecheck` | OK — 31 public SDK findings and 1 cross-package test seams allowlisted |
| `go run ./tools/depscheck` | OK — 24 core dependencies, all justified |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | OK |
| `go run ./tools/docclaimscheck -base origin/main` | OK |
| `staticcheck ./...` | clean |
| `go test ./... -count=1 -timeout=40m` | green |

## What the ratchet says now

**7 import edges / 0 call sites.** The forbidden-call allowlist is **empty**
— the file keeps its header and nothing else, and the tool that keeps it
honest is the one that says so: `archcheck -update` tightened two entries
that no longer occur, and the check now passes with **0 call sites**.

The **import** ratchet is unchanged at **7**, and the traps are the same ones
the roadmap has been naming:

- **`controlplane → runtime`** — the control plane calls **37** verbs on the
  kernel's god-object. A redesign, not a slice.
- **`selfrepair → runtime`** — the six agent rows could move to the reaper's
  `api` package, but `RunWith`/`RunWithRetry`/`ReaperScan`/`NewCorrelation`
  need a **runs port that does not exist** yet, so that port is a slice of
  its own.
- **`channel → bus`** — 26 plugin `Config`s hold `*bus.Bus` and nil-check it.
  An interface parameter turns a nil `*bus.Bus` into a **non-nil** interface,
  the guard stops firing, and the publish panics inside the deferred recover.

And W5.4's other two parts are still open: the **import** allowlist at zero
and the **`docs/`** archiving.

## The rules the slice is really about

**"Already atomic by another means" is a measurement, and it can be half
right.** The staging tree made the three sites look safe: a reader never saw
a half-written file. What the measurement did not ask was whether the staged
bytes were **durable**, or what happened when the **second half** of the swap
failed. Both halves were wrong, and both were invisible to a rule that only
looked at the call site.

**Deleting is not moving.** The swap's shape — delete, then rename — reads
like a small window. It is not: the delete is where the **content** goes, and
a failure after it is not recoverable. The old tree is now **renamed** aside,
so the failure has something to put back.

**A policy that two packages grew separately is two policies.** The skill
package had its own Windows rename retry; the marketplace had its own
staging dance. One shared package now holds the policy, the retry and the
names, so the next tree publish inherits them instead of growing its own.

**The leftover of a failed cleanup must be bounded.** "Remove the old tree
afterwards, best effort" is only safe if the leftovers cannot accumulate. The
old tree always sits at **one** fixed path next to the destination, and the
next swap removes it first — so a destination can never hold more than one.
