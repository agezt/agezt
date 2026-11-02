# W3.76r — the objective library's lifecycle moves into the work module

**Slice:** W3.76r (the twenty-sixth of the runtime's dissolution)
**Base:** `b9538379` (the W3.76q merge)
**Date:** 2026-11-02

---

## 1. The family

The **objective library** is the goals-and-key-results store: a goal, its key
results, the tasks linked to them and the rollup rule that turns the board's
live task statuses into a goal's progress.

The library's **file** has been the daemon's since W3.53: the daemon opens
`okr/okr.json`, holds the contract's store port (`okrapi.Store`) and hands it
over. That part never moved and does not move now.

The library's **lifecycle**, on the other hand, was the daemon's too — and it
should not have been. The lifecycle is the **work module's**: the module
(`kernel/modules/work/okr`) keeps the goals, the key results and the rollup
rule, and it already knew how to refuse without a store, how to publish what
happened to a goal and how to roll a goal up from the board's live statuses.

So the daemon held **six forwarders over the module's own operations** —
`CreateObjective`, `AddObjectiveKeyResult`, `LinkObjectiveTask`,
`UnlinkObjectiveTask`, `ArchiveObjective`, `Rollup` — plus the two wiring
doors (`SetObjectiveLibrary`, `ObjectiveLibraryDependencies`) that built the
module's service.

A forwarder over a module's own operation is a **second spelling** of it. And
this one cost a hop: the **application** layer asked the daemon for the
objectives, and the daemon asked the module. So every objective write went
*application → daemon → module*, and `kernel/app/okr` had to import
`kernel/runtime` to say so.

## 2. What moved

**The application binds the MODULE now.** `kernel/app/okr`'s ports take the
work module's `ObjectiveService` and the contract's store port:

```go
type Ports struct {
	Reader  Reader
	Library workapi.ObjectiveService
}
```

The app's own `MutationHost` and `Rollupper` interfaces — the app's second
spellings of the module's operations — are **gone**: one operation has one
name, and the name is the module's.

**The daemon holds two things and no more:**

- `OKR()` — the library's **file**, the contract's store port (W3.53);
- `Objectives()` — the **module's face**, so the composition root's scope can
  hand the module to the application.

**The composition root names the module.** The builder emblemi: the daemon
may not name a module's implementation, so `runtime.Config` gained
`ObjectiveLibrary workapi.ObjectiveServiceBuilder`, the composition root sets
it (`cfg.ObjectiveLibrary = okr.NewService`), and `compose.go` calls it over
the store the daemon opened and its own ports:

```go
objectiveSvc = cfg.ObjectiveLibrary(cfg.OKRStore, workapi.ObjectiveDependencies{
	Publish: objectivePublisher{b: kbus}.Publish,
	TaskStatus: func(taskID string) (workboardapi.Status, bool) { ... },
})
```

The board's status is asked through a **port** (`TaskStatus`), not reached
for: the board's file is the host's business, and the library asks for a
*status*.

**What stayed in the host is the host's own state**: the library's file, the
board's completion glue (`recomputeOKRForTask` — the daemon asks the module to
re-roll the goals that link a finished task) and the module's face.

## 3. The measurement

| measure | before | after |
|---|---:|---:|
| the daemon's public verbs | 201 | **194** |
| the daemon's production lines (non-blank) | 6,121 | **6,107** |
| the app packages that import `kernel/runtime` | 42 | **41** |

The daemon's seven retired verbs: the six forwarders, the setter and the
dependencies builder. The one added: `Objectives`, the module's face — the
daemon needs a door to hand the module over.

**`kernel/app/okr` no longer imports the daemon.** That is the first time in
this program that an application package stopped naming `kernel/runtime`: the
scope builder (`kernel/app/binding`) holds the bridge, and the application's
own package does not.

## 4. The ceiling followed the floor

`kernel/runtime/dead_surface_ratchet_test.go` pinned the daemon's public
surface at **220** — the number W3.76k measured. Since then the surface fell
to 201 (W3.76q) and, with this slice, to 194 — while the ceiling stayed at
220. The ratchet therefore permitted **26 verbs of growth** in a program whose
whole point is that the host's surface shrinks.

The ceiling now follows the floor: **220 → 201 → 194**. A ceiling that stays
put while the surface shrinks permits exactly what the program exists to
remove — and a ratchet that permits what it claims to forbid has never fired.

## 5. The ratchets — and the probes

Three ratchets changed, all honestly:

- **the daemon's surface ceiling** (`dead_surface_ratchet_test.go`):
  220 → 194;
- **the daemon's objective ratchet** (new, `objective_ratchet_test.go`): the
  six forwarders and the two wiring doors do not come back, the daemon does
  not write the library's rows itself (`k.okr.Create(` and friends), and the
  daemon still holds `OKR` and `Objectives`;
- **the application's binding ratchet** (new,
  `kernel/app/okr/binding_ratchet_test.go`): the package's production files
  must not import `kernel/runtime` again. (The test files may — a test that
  opens a kernel to prove the daemon holds nothing is a test *about* the
  daemon.)

**Both probes fired**: putting `CreateObjective` back on `*Kernel` fails the
daemon's ratchet, and putting the daemon import back into the application's
production files fails the binding ratchet.

## 6. The coverage moved with the subject

The daemon's objective coverage (`kernel/runtime/okr_heavycov_test.go`) drove
the daemon's verbs, so it had nowhere to go but the **owner**: the CRUD
lifecycle (create → key result → link → roll up → unlink → archive) and the
blank-title refusal now live in `kernel/modules/work/okr/lifecycle_test.go`,
next to the code they exercise.

The daemon's own tests now pin what the daemon still does: it holds the
library's file, it holds the module's face, and it holds **no** service when
it was given no builder.

## 7. Verification

- the compiler (`go build ./...`, `go vet ./...`);
- the module's own tests (the CRUD lifecycle, the refusal, the publishing, the
  rollup, the recompute);
- the application's tests (the ports, the lifecycle, the live rollup, the
  journaling);
- the daemon's tests (the file, the face, the refusal without a builder);
- the three ratchets and their probes;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 8. What is left

The objective family is done: its file is the host's, its lifecycle is the
module's, and the application binds the module.

The same shape is waiting for the other families that still have forwarders
in the daemon — the **task board** (18 verbs), the **workflow engine** (12),
the **MCP registry** (12), the **script forge** (8), the **standing library**
(9) and the **roster's** remaining read doors (9). Each one is the same
three moves: the module keeps the lifecycle, the composition root hands over
the builder, and the application binds the module instead of the daemon.

The **workboard** is next by size — eighteen verbs and the biggest single
block of the daemon's surface.
