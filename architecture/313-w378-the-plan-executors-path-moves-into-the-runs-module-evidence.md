# W3.78 — the plan executor's path moves into the runs module

**Slice:** W3.78 (the forty-eighth of the runtime's dissolution)
**Base:** `194c5344` (the W3.77 merge)
**Date:** 2026-11-14

---

## 1. The family

A **plan** is a directed acyclic graph of steps that a run executes. The
**executor** of that graph — the scheduler — walks the DAG, publishes each
step's events and reifies the plan's nodes into the run's own loop.

The executor had **no rule and no store of its own**: it was a **body without
a home**. It executed the runs module's plan (`kernel/modules/runs/plan`) and
it produced the runs module's loop, and it lived at `kernel/scheduler` — a
top-level package that the target architecture had already assigned to a
module.

## 2. What the gate already knew

`tools/archcheck/layers.json` placed `kernel/scheduler` in the **runs**
module **before this slice**:

```json
{"pattern": "kernel/scheduler", "layer": 3, "module": "runs"},
```

So the family's boundary was drawn long ago; the **path** was the last thing
out of place. This is the third time the runtime's dissolution ended in a
**pure path move** — the body, the names, the callers and the package's own
tests are exactly as they were.

The move:

```console
git mv kernel/scheduler kernel/modules/runs/scheduler
```

**Eight files** went with it (five production, three test), and git recorded
all eight as **renames** — the bodies are byte-identical apart from the
import path inside the test files and the provenance paragraph in `doc.go`.

## 3. The crossing

Twelve files imported the package; every one of them now imports the module's
path:

| importer | kind |
|---|---|
| `kernel/app/runs/plan.go` | the runs application |
| `kernel/app/runs/plan_test.go` | its test |
| `kernel/app/runs/ports.go` | the runs application's ports |
| `kernel/app/runs/ports_test.go` | its test |
| `kernel/modules/runs/api/plan.go` | the runs module's port package |
| `kernel/modules/runs/plan/plan.go` | the plan package itself |
| `kernel/modules/runs/plan/plan_test.go` | its test |
| `kernel/modules/runs/scheduler/{correlation,coverage,scheduler}_test.go` | the package's own tests |
| `kernel/runtime/compose.go` | the daemon's composition root |
| `kernel/runtime/runtime_kernel.go` | the daemon's kernel |

The import blocks were re-sorted with `gofmt` (one line of diff per file).

The **gate's rule for the old path is GONE** with the package:

```diff
-    {"pattern": "kernel/scheduler", "layer": 3, "module": "runs"},
```

That is the **fail-closed** half of the move: with the rule present, a
package that reappeared at `kernel/scheduler` would be waved through by a
rule that no longer describes anything. With the rule **gone**, a package at
that path matches **no** placement rule and the gate says so.

## 4. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 105 | **105** |
| the daemon's production lines | 5,214 | **5,214** |
| the daemon's production files | 52 | **52** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the plan executor's package | 5 files / 768 lines | **5 files / 775 lines** |
| the executor's package at its old path | 5 files / 768 lines | **0 files / 0 lines** |
| the daemon's surface ceiling | 105 | **105** |

**Nothing moved but the path** — and the numbers say it honestly. The daemon
is **flat on every axis**: 105 verbs, 5,214 production lines, 52 production
files. The executor's package grew by **seven** lines, and file by file those
seven lines are **one comment block**:

| file | before | after |
|---|---|---|
| `doc.go` | 26 | **33** (+7, the provenance paragraph) |
| `nodes.go` | 167 | **167** (0) |
| `scheduler.go` | 137 | **137** (0) |
| `scheduler_publish.go` | 157 | **157** (0) |
| `scheduler_run.go` | 281 | **281** (0) |

The four production bodies are **byte-identical**. The whole slice's
production diff, outside the moved package, is:

```text
kernel/app/runs/plan.go                          | 2 +-
kernel/app/runs/ports.go                         | 2 +-
kernel/modules/runs/agent/doc.go                 | 2 +-
kernel/modules/runs/api/plan.go                  | 6 +-
kernel/modules/runs/plan/plan.go                 | 4 +-
kernel/runtime/compose.go                        | 2 +-
kernel/runtime/runtime_kernel.go                 | 2 +-
tools/archcheck/layers.json                      | 1 -
```

## 5. The ratchets — and the probe

A new ratchet file, `kernel/runtime/scheduler_ratchet_test.go`, holds three
tests:

- **`TestTheSchedulerLivesInTheRunsModule`** — the old path is gone
  (`kernel/scheduler` does not exist), the module's path carries all five
  production files.
- **`TestTheGateHasNoRuleForTheOldPath`** — the gate's rule for the old path
  is gone, so the architecture is fail-closed.
- **`TestTheDaemonAsksTheModulesScheduler`** — no production file in the
  kernel imports the old path. The compiler would refuse it anyway (the
  package is not there), but the ratchet says it out loud, so the path does
  not creep back under another name.

**The needle is assembled from two pieces on purpose.** The probe that
rewrites the import path across the tree rewrites every literal spelling of
that path — **including the ratchet's own needle**. With a single literal,
the probe turned the check into a **tautology**: the needle came back
rewritten into the path it was looking for, and the ratchet flagged every
*legal* importer (six false positives) while missing the illegal one. Two
pieces (`"agezt/kernel/"` + `` `scheduler"` ``) leave **no contiguous
spelling** for a tree-wide rewrite to hit. This is the same lesson as the
W3.76i ratchet that "never fires" — but here it fired **wrongly**, which is
worse, and the fix is the same shape: the ratchet's own vocabulary must not
be rewritable by the probe.

**The probe** put the world back before this slice — the package at its old
home, every caller importing the old path again, the gate's rule back — and
all three ratchets went red at once. Nothing else broke; the probe was
reverted and the suites went green again.

The **surface ceiling did not move**: 105 → 105. A path move takes no verb
off the daemon and adds none, so the ceiling stays exactly where the last
slice left it.

## 6. The guard

The **coverage moved with the subject**: the package's own three test files
moved with it, and only their import path changed. No test was rewritten,
no assertion was weakened, and no coverage was left behind — the executor's
tests still run from the module's home.

The **control plane** needed no change: no verb was retired, so no retired
name joins the retired-verb list and no family rule changes.

## 7. A debt this slice leaves on the table

`tools/archcheck/layers.json` still carries a rule for `kernel/delegation` —
a package that **no longer exists** (it was dissolved in W3.76f). That rule
is **dead weight**: it describes nothing and it is out of this slice's scope
(a path move of the scheduler does not carry a stale rule for a different
family). It is recorded here so the next slice that touches the gate's rule
list can take it out.

## 8. Verification

- the compiler (`go build ./...`, `go vet ./kernel/...`) and `gofmt`;
- the daemon's tests, the runs module's tests and the runs application's
  tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 9. What is left

The daemon holds **105** public verbs and **5,214** production lines; **forty**
application packages still name it.

The **remaining** path moves of the target architecture are now a list, and
each is a path move of the same shape:

| package | production files / lines | eventual home |
|---|---|---|
| `kernel/standing` + `kernel/cadence` | 18 files | **triggers** (`kernel/modules/triggers`, the directory does not exist yet) |
| `kernel/contextselect` | 4 files | runs |
| `kernel/planner` | 5 files | runs |
| `kernel/reflect` | 1 file | runs |
| `kernel/toolexec` | 2 files | runs |
| `kernel/pulse` + `kernel/anomaly` + `kernel/alerter` | 19 files | pulse |
| `kernel/market` | 14 files | market |
| `kernel/update` | 4 files | system |
| the media tools | 3 files | media |

The daemon's **remaining** second holds — the families it still sets and
answers for (workflows, reasoning, board, market, reaper, MCP, script forge)
— are the ones that are not path moves: they are **lifecycle** moves, where
the host gives up a door and keeps a face.
