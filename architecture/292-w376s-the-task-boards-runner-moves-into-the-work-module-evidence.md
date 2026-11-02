# W3.76s — the task board's runner moves into the work module

**Slice:** W3.76s (the twenty-seventh of the runtime's dissolution)
**Base:** `2bde9134` (the W3.76r merge)
**Date:** 2026-11-02

---

## 1. The family

The **task board** is the durable work record: a task, its lifecycle (create,
claim, heartbeat, comment, block, fail, unblock, complete, review, archive),
its relations (link, retry policy, dependency, stale reclaim) and its proof.

The board's **data** has been the work module's for a long time, and the
daemon's file has been the host's (W3.47). What was left in the daemon was
**sixteen forwarders** over the module's own operations:

`CreateWorkboardTask`, `ClaimWorkboardTask`, `HeartbeatWorkboardTask`,
`CommentWorkboardTask`, `BlockWorkboardTask`, `FailWorkboardTask`,
`UnblockWorkboardTask`, `SetWorkboardRetryPolicy`, `CompleteWorkboardTask`,
`ReviewWorkboardTask`, `ArchiveWorkboardTask`, `LinkWorkboardTask`,
`AddWorkboardDependency`, `ReclaimStaleWorkboardTask`,
`SweepStaleWorkboardClaims` — and `ProveTask`, in a file of its own.

Each one did the same three lines: take the module's service out of the
daemon, call the module, return the module's answer. A forwarder over a
module's own operation is a **second spelling** of it — and the daemon held a
second copy of the whole board surface, sixteen doors wide, for the
application layer and the plugin to call through.

## 2. The port the board already had

The **contract** (`kernel/contract/workboardapi.Runner`) is the surface the
agent-facing plugin speaks, and it names the board's operations the way the
board's users say them: *claim a workboard task*, *comment a workboard task*.
The daemon implemented that port.

So the port was never the daemon's to implement — it was the contract's, and
the **module** now implements it over its own service
(`kernel/modules/work/workboard/runner.go`, 83 lines). The names did not
change and no caller outside the daemon had to be rewritten: the application
layer's contracts already spoke the contract's names, and the plugin already
spoke the contract's port.

One addition: three verbs the contract does not name but the application
needs — the **review**, the stale **sweep** and the **proof**. They live on
the module's new `BoardRunner` port
(`kernel/modules/work/api/workboard.go`), which embeds the contract's `Runner`
and adds those three.

## 3. What moved

- **the module** implements the contract's runner port over its own service;
- **the daemon** keeps three things and no more: the board's **FILE**
  (`Workboard`), the board's **WIRING** (`WorkboardDependencies` — the bus it
  publishes on, the completion judge, the model, the artifact index and the
  journal) and the module's **FACE** (`BoardRunner`, `SetBoardRunner`);
- **the application layer** binds the board's runner: `kernel/app/workboard`
  no longer imports `kernel/runtime` at all;
- **the plugin** binds the module's runner: `workboardPort(k)` answers
  `k.BoardRunner()` instead of the daemon itself.

**The correlator split.** The dispatcher needed one thing the board does not
have: a correlation to name an operation with. That is the **host's** own
capability, so it left the dispatch host interface and became its own narrow
port (`Correlator.Mint()`) — a port may not carry the name of the kernel verb
it stands for.

## 4. The measurement

| measure | before | after |
|---|---:|---:|
| the daemon's public verbs | 194 | **179** |
| the daemon's production lines (non-blank) | 6,107 | **5,979** |
| the daemon's production files | 55 | **54** |
| the app packages that import `kernel/runtime` | 41 | **40** |
| the board module's production lines | 1,469 | **1,552** |

**Fifteen verbs off the daemon's surface in one slice** — the biggest single
drop since the runtime's dissolution began. The board module grew by 83
lines: the runner adapter. *Moving a rule moves the conditions, not the
lines* — the module's lines are the moved lines **plus** the port.

`kernel/app/workboard` is the second application package to stop naming the
daemon (after `kernel/app/okr` in W3.76r).

## 5. The ratchets — and the probes

Two ratchets, **both probes fired**:

- the daemon's new board ratchet (`kernel/runtime/workboard_ratchet_test.go`)
  pins that the sixteen forwarders and the setter do not come back, and that
  the daemon still holds `Workboard`, `WorkboardDependencies` and
  `BoardRunner`;
- the application's new binding ratchet
  (`kernel/app/workboard/binding_ratchet_test.go`) pins that the package's
  **production** files do not import `kernel/runtime` again. (The test files
  may: a test that opens a kernel to prove the daemon holds nothing is a test
  *about* the daemon.)

Putting `ClaimWorkboardTask` back on `*Kernel` failed the daemon's ratchet and
the surface ceiling; putting the daemon import back into the application
failed the binding ratchet.

## 6. The coverage moved with the subject

The daemon's board coverage drove the daemon's verbs, so it went where the
verbs went: the refusal of every operation without a store now runs against
the module's runner (`kernel/runtime/workboard_port_test.go`), and the daemon's
own tests pin what the daemon still does — it holds the file, it hands the
wiring over, and it holds no face until the composition root wires one.

## 7. Verification

- the compiler (`go build ./...`, `go vet ./...`);
- the module's own tests (the lifecycle, the proof, the runner);
- the application's tests (the ports, the lifecycle, the relations, the
  dispatch, the execution);
- the daemon's tests (the file, the wiring, the face, the refusals);
- the two ratchets and their probes;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 8. What is left

The board is done: its file is the host's, its rules and its runner are the
module's, and the application binds the module.

The same shape is waiting for the families that still have forwarders in the
daemon: the **workflow engine** (12), the **MCP registry** (12), the
**standing library** (9), the **roster's** remaining read doors (9) and the
**script forge** (8). Each is the same three moves: the module keeps the
lifecycle, the composition root hands over the builder, and the application
binds the module instead of the daemon.

The daemon's own surface is now **179 verbs** — and the W3 exit criterion that
no type has more than 60 methods is still the daemon's own `*Kernel` that
breaks it.
