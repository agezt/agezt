# W3.59 — the board's PORT is the contract's

## What the slice is

After W3.58 the ratchet stood at **16/13** and the allowlist still named the
board module from three places. This slice takes **all three** edges:

| Edge | Why it existed |
|---|---|
| `adapter-bypass kernel/controlplane -> kernel/modules/board/board` | the control plane held the module's concrete `*board.Store` — and **opened the board's file itself** as a read fallback |
| `adapter-bypass kernel/restapi -> kernel/modules/board/board` | the REST mailbox held the module's concrete `*board.Store` |
| `cross-module kernel/modules/agents/selfrepair -> kernel/modules/board/board` | the self-repair module named the module for **one row** — the message |

The ratchet went **16 → 13**. The **13** allowlisted call sites are unchanged.

This slice is **not** "the file is the daemon's" — the board's file was
**already** the daemon's (`cmd/agezt` opened it and every holder shared that
one instance). The slice is about the **port**: the board's surface had
**three shapes**, and two of them were concrete types the layers above the
module had to name.

## What it did before

```go
// kernel/controlplane/server.go
boardStore *board.Store

// kernel/controlplane/board.go
func (s *Server) boardReader() (*board.Store, error) {
    if s.boardStore != nil {
        return s.boardStore, nil
    }
    return board.Open(filepath.Join(s.baseDir, "board")) // a second opener
}
```

```go
// kernel/app/board/board.go — a THIRD copy of the same eleven methods
type Store interface {
    Read(string, int) []shared.Message
    Topics() map[string]int
    // ... eleven methods, declared again
}
```

The contract `kernel/contract/boardapi` **already existed** and already
carried the board's row, the wildcard recipient and the store's **eleven-verb
port** — the board **tool** (a plugin, and a plugin may reach the contract
and nothing else) has been taking the store through it since W3.21, and the
tool registry has held it since W3.39a. What was left was the **adapters**
and the **module** naming the module anyway.

## What it does now

### The contract needs nothing new

`boardapi.Store` was already the whole surface. The slice did not widen it,
did not split it, and did not move a rule: the board's rules are the store's
own (`Validate` lives in the module, and the module's `Validate` delegates to
the contract's — the row's rule was already there).

The one thing the slice closed is a **duplicate**: `kernel/app/board` declared
its own interface with the same eleven methods. The board's surface had three
shapes — the contract's port, the module's concrete store and the app's own
copy — and the app's was the only one that did not need to exist. It is now an
**alias** of the contract's port.

### The adapters speak the contract's port

The control plane holds `boardapi.Store`, the REST mailbox holds
`boardapi.Store`, and the self-repair module names `boardapi.Message` — the
contract's row — instead of the module's alias of it. The row is the **same
type** on both sides (`type Message = boardapi.Message`), so the rename is
mechanical, and the edge disappears because the module is no longer named.

### The daemon converts where the value is still concrete

This is the part the slice had to teach, and it was not theoretical.

```go
// cmd/agezt/tooldeps.go
func boardPort(st *board.Store) boardapi.Store {
    if st == nil {
        return nil
    }
    return st
}
```

A **nil `*board.Store` handed to an interface parameter is a NON-NIL
interface**. The board store is opened **late** and the daemon leaves it nil
when it could not be opened, so before this slice the daemon passed the
concrete store straight into self-repair's `Mailbox` interface
(`cmd/agezt/main.go`): the callee's `mailbox == nil` guard would **not**
fire, and the first escalation would have dereferenced a board that was not
there. The conversion now happens where the value is still concrete, and the
rule is pinned by a test.

The same rule was already in place for the board **tool** (`lateBoard`), which
is why the board tool had never crashed on a board that could not be opened.

### The control plane's read fallback is GONE — and that is a behaviour change

The control plane used to fall back to a **fresh read-only `Open`** of the
board's file when nothing was wired. Three tests pinned it, and one of them
was named for it (`TestBoardAppHostRetainsReadFallbackAndSharedWriter`).

The slice removes it, and the removal is deliberate:

- the board's **file** is the daemon's business — the control plane may not
  name the module, and it does not open the module's file;
- a board that **could not be opened** used to look like a board with **no
  messages**: the reads answered an empty board while the daemon's banner
  said the board was unavailable. That is a silent lie, and the writes were
  already honest about it;
- so the reads now answer **"the board is not available on this daemon"**,
  exactly like the writes did.

**What this changes**: a control plane with **no board wired** now refuses its
board **reads** too. In the daemon's real topology nothing changes — the
daemon opens the board and wires it, so a board-less daemon is one whose file
could not be opened, and that daemon already said so in its banner.

The one fresh read-only open that **remains** is the overseer app's
`OpenHelp`: the app layer is the layer that may see the module (it needs both
the kernel and the board), and it opens the same file **read-only**.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/boardapi`: `TestStorePortCarriesTheWholeBoardAndNothingMore` | the board's port is **exactly eleven** verbs |
| `kernel/contract/boardapi`: `TestEveryoneIsTheWildcardRecipient` | the wildcard recipient is pinned by **value** |
| `kernel/app/board`: `TestTheAppsStorePortIsTheContractsPort` | the app's store port **IS** the contract's port — one shape, not three |
| `kernel/controlplane`: `TestTheControlPlaneHoldsTheBoardsContractPort` | the control plane's face is **statically** the contract's port (the declared parameter type is compared) and the notifier speaks the contract's row |
| `kernel/controlplane`: `TestBoardAppHostRefusesWithoutABoardAndWritesThroughTheSharedOne` | a control plane with **no** board refuses on the reads **and** the writes |
| `kernel/controlplane`: `TestBoardWrite_Validation` | the unwired writes and reads both refuse |
| `kernel/restapi`: `TestMailbox_ValidationAndAuth` | an unwired REST mailbox answers **503** |
| `cmd/agezt`: `TestTheDaemonOpensTheBoardsFile` | the daemon's board is a **file on disk** under the base dir, and the port it hands over is live |
| `cmd/agezt`: `TestTheBoardsNilPortRule` | a board that could not be opened stays a **nil PORT** |
| `kernel/modules/agents/selfrepair`: `TestTheMailboxSpeaksTheContractsRow` | the self-repair mailbox port is served by the contract's store surface, and carries the contract's row |

### The mutation run

**10 mutations: 10 caught, 0 survived, 0 skipped, 0 build errors** — in **two**
independent passes, both honest (every catch named the expected test or the
expected edge).

| # | Mutation | Caught by |
|---|---|---|
| 1 | the control plane names the board module again | **archcheck** — `adapter-bypass kernel/controlplane -> kernel/modules/board/board` |
| 2 | the REST API names the board module again | **archcheck** — `adapter-bypass kernel/restapi -> kernel/modules/board/board` |
| 3 | the self-repair module names the board module again | **archcheck** — `cross-module kernel/modules/agents/selfrepair -> kernel/modules/board/board` |
| 4 | the control plane's board face is an anonymous interface, not the contract's port | `TestTheControlPlaneHoldsTheBoardsContractPort` |
| 5 | the contract's board port grows a twelfth verb | `TestStorePortCarriesTheWholeBoardAndNothingMore` |
| 6 | the app's store port is its own interface again | `TestTheAppsStorePortIsTheContractsPort` |
| 7 | **the daemon's nil-port rule breaks** | `TestTheBoardsNilPortRule` |
| 8 | **the control plane's refusal answer changes** | `TestBoardAppHostRefusesWithoutABoardAndWritesThroughTheSharedOne` |
| 9 | **the REST API's 503 becomes a 200** | `TestMailbox_ValidationAndAuth` |
| 10 | **the daemon's board file drifts to another path** | `TestTheDaemonOpensTheBoardsFile` |

**Three of the ten are gate-measured.** "The control plane reaches less" is
not something a test can see: the board's answers would be exactly the same,
because the module and the contract carry the same row and the same rules.

**Four of the ten measure a BEHAVIOUR, not a shape:**

- the **nil-port rule** is the difference between a daemon that says "the
  board is not available" and one that binds a board that is not there — and
  a nil func-like value does not fail when it is called;
- the **refusal answer** is what the operator reads. "Nothing was sent" and
  "there is no board" are different answers, and only the second is true;
- the **503** is what an SDK client sees. A successful empty mailbox is not
  "the board is unavailable";
- the **file's path** is the contract with the operator: the same file has to
  be there after a restart.

**The harness needed one new capability.** The control plane has **two**
guards with the same shape (the reads and the writes), and the daemon's
`boardPort` has a nil check that another function also has — so the harness
replaces the first line matching a pattern *after* an anchor. Replacing the
first occurrence alone would have measured the wrong guard.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 315 packages placed; 13 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 16** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rules the slice is really about

**A concrete type in a port row is a dependency with an escape hatch.** The
control plane's `boardStore *board.Store` was not a port — it was a way of
saying "I need the module". The moment the module moved behind a contract,
that row had to become the contract's port, or the adapter would keep naming
the module and the edge would stay.

**The conversion belongs where the value is still concrete.** The rule is not
"check for nil at the call site" — it is "convert while you still know it is
nil". A nil pointer handed to an interface parameter is a non-nil interface,
and every `== nil` guard downstream becomes a lie. The board's store is
opened late and really is nil on a live path, so the rule was not academic.

**A surface with three shapes drifts.** The board's port existed in the
contract, in the module and in the app — all three with the same eleven
methods. They agreed, and then one of them would have grown a verb, and only
the contract's copy would have been the real one.

**A board that could not be opened must not look like a board with no
messages.** The read fallback was a convenience with a cost: it turned a
broken daemon into a silent one. The refusal is not a worse answer — it is the
only true one.
