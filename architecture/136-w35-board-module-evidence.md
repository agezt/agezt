# W3.5 the board module

The shared message board was a module without a home: the layer map named
`kernel/board` as layer 3 in the module `board`, but the package sat at the top
of `kernel/`, outside the `kernel/modules/` tree W3 establishes as the target
home of the bounded contexts.

The roadmap's W3 step 5 is `modules/board` — with no defect named beside it.
The slice found one anyway, and it is the kind that hides in plain sight: a
write that *failed* was on the board anyway.

## What moved

| the package | before | after |
|---|---|---|
| the message board | `kernel/board` | `kernel/modules/board/board` |

The **package name does not change** (`board`), so the move is an import-path
change and nothing else: 49 Go files were repointed, and no call site, type
name or behaviour moved with them.

The placement is now *derived* rather than declared: the layer map's
`kernel/modules/*/...` rule (layer 3, module `@1`) places the package in the
module `board`, exactly where the hand-written `kernel/board` rule used to
place it. That rule is gone.

The allowlist moved with the edges: six entries changed their path (the same
six violations, relocated). The ratchet therefore stays at **128 import
violations and 13 call sites**, and the package count stays at 273.

## A write that failed was on the board anyway

The board is a plain JSON file with the whole message list in memory. Every
write followed the same shape:

```go
s.msgs = append(s.msgs, m)          // in memory first
if len(s.msgs) > MaxMessages { ... } // the cap drops the OLDEST
return m, s.save()                   // then it is persisted
```

When the save **failed**, the message stayed in memory. So:

1. A post the caller was told had failed was on the board anyway — and the
   next successful save wrote it to disk. The caller had been told the post
   did not happen; it had happened.
2. At the cap, the failed post had also dropped the **oldest** message from
   memory. The next successful save persisted that drop — so a single disk
   hiccup silently discarded board history that was never really removed.

The proof:

```
a post whose save failed is on the board anyway
```

The same shape was in `Ack`: the ack was taken in memory before the write, so
a failed write left the board claiming a read that was never persisted — and
the reader's inbox was cleared in memory, which the next successful save made
permanent.

## The fix: persist first, adopt second

The board now writes the **prospective** state and adopts it in memory only
once the write succeeded:

```go
next := make([]Message, 0, len(s.msgs)+1)
next = append(next, s.msgs...)
next = append(next, m)
if len(next) > MaxMessages {
    next = next[len(next)-MaxMessages:]
}
if err := s.save(next); err != nil {
    return m, err
}
s.msgs = next
```

`Ack` follows the same rule (it copies the list, appends the reader to the
copy, persists the copy, and adopts it only on success). Nothing else changes:
the same messages, the same file, the same JSON shape, the same events, the
same cap.

The store is a **pure** store (no bus, no kernel dependency) — it still is.
The daemon publishes `board.posted` for every write through any door, and
that is unchanged: the notification follows the *successful* write, exactly
as before.

## What did not change

- the same package name, the same exported API, the same messages, the same
  `board.json` on disk, the same cap (`MaxMessages=1000`, the oldest dropped
  first), the same inbox semantics (directed, broadcast, help, per-reader ack,
  replies), the same events and the same subjects;
- the same **single shared store** invariant: the daemon opens one store and
  hands it to the board tool, the control plane, the REST mailbox and
  self-repair; two writer instances would still silently drop each other's
  messages, and that is unchanged.

## Runnable evidence

Permanent tests:

- `kernel/modules/board/board/save_failure_test.go` — the proof
  (`TestSend_FailedSaveDoesNotLandOnTheBoard`: the board file is replaced by a
  directory that cannot be removed, the post fails, and the failed message is
  on neither the in-memory board nor the disk — and the next successful post
  does not resurrect it), and the same rule for the ack
  (`TestAck_FailedSaveDoesNotLandOnTheBoard`: a failed ack leaves the
  reader's inbox alone);
- `kernel/modules/board/board/board_test.go` — the cap test is **stronger**
  than it was: it counted the messages, and now it also checks *which* ones
  survive (the newest stay, the oldest are gone). The count alone could not
  tell a cap that keeps the oldest from one that keeps the newest.

The mutations fail tests: **10 mutations, 10 caught** — the failed message
lands anyway, the failed save is swallowed, the cap is not enforced, the cap
keeps the oldest, the sender sees their own broadcast, the sender is excluded
from broadcasts, one reply clears a broadcast for every reader, an answered
directed message stays in the inbox, the failed ack lands anyway, and the ack
duplicates the board.

The run produced honest findings about the **tests**, which are now fixed and
named: the cap test asserted only the *count* (a cap that kept the oldest
messages would have passed it), and the ack's failed-write path was not
covered at all.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (273 packages, 128/13 — the ratchet is unchanged, the edges
moved with their packages), `tools/structure-md`, `tools/changelog-lint`,
`tools/docclaimscheck` and staticcheck. The full `go test ./...` passes.

The move also broke a live reference **in CI**, and it was caught by the exact
head run: the `test (linux)` job keeps a 100% statement-coverage ratchet over
ten named packages, and the board was one of them — listed by its old path,
so the job would have pointed at a package that no longer exists. The list
follows the package (`./kernel/modules/board/board`), and the package is still
at **100.0%** statement coverage after the fix (the two new failure paths are
covered by the new tests).

## Not done here

**The board still has one writer.** The store holds the whole list in memory
and saves it whole, so two writer instances would silently drop each other's
messages. The daemon opens exactly one store and shares it; the read paths
that open a fresh copy are read-only. Making the store safe for several
writers is a different (bigger) change, and it is not this slice's.

**The module has no `api` package yet.** The board's types are imported by the
runtime, the control plane, the REST API, self-repair and two plugins — those
are the six relocated allowlist entries. The reasoning module's shape (an
`api` package that owns the vocabulary, the host keeping its public names as
aliases) would shrink that surface; it is a separate slice.
