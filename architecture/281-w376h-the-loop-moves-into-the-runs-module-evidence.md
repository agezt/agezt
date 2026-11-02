# W3.76h — the loop moves into the runs module

**Slice:** W3.76h (the eighth of the run-engine programme, the sixteenth of the
runtime's dissolution)
**Base:** `bfd72355` (the W3.76g merge)
**Date:** 2026-11-02

---

## 1. The family

The loop is the **engine** of every governed run. Before this slice it was
`kernel/agent` — a top-level package of **20 production files** and **2,629**
non-blank lines:

| piece | file | non-blank |
|---|---|---:|
| the entry point and the loop driver | `agent_run.go` | 331 |
| the loop's configuration and the segment arithmetic | `agent_loop.go` | 216 |
| the context assembly, the budget, the compaction and the defaults | `agent_context.go` | 218 |
| the tool gate (resolve → decide → announce → execute → settle) | `run_tools_gate.go` | 328 |
| the observation trust boundary and the injection window | `observation.go` | 217 |
| the run's state and the tool surface shaping | `run_tools.go` | 133 |
| the prologue (validation, normalization, the task receipt) | `run_setup.go` | 138 |
| the provider call and the completion request | `run_provider.go` | 100 |
| the middleware chain | `middleware.go` | 239 |
| the structured-output path | `generate.go` | 197 |
| the read-only tool memo | `memo.go` | 95 |
| the tool selectors | `toolselect.go` | 193 |
| the steering port | `agent_steer.go` | 45 |
| the contract aliases | `agent.go` | 53 |
| the middleware-from-env | `middleware_env.go` | 58 |
| the compatibility forwards (artifact, policy, schema, tool context) | `agent_context_offload.go`, `agent_context_policy.go`, `schema.go`, `toolctx.go` | 64 |
| the package's own documentation | `doc.go` | 14 |
| **total** | | **2,629** |

The family was **already a module in everything but its address**. Its rules
(the segment arithmetic, the auto-continue budget, the observation trust rules,
the causal window, the loop guard, the memo) all describe *a run*. Its
dependencies are all contract or platform rows (`llm`, `policyapi`, `toolapi`,
`toolphaseapi`, `policyctx`, `schema`, `toolaudit`, `tooloutput`,
`toolpipeline`, `kernel/event`, `kernel/bus`). It owns **no** store, **no**
bus, **no** clock of its own.

What it lacked was **the tree's own word for it**. `kernel/agent` sat at the
top level, so the layer classifier had to be told about it by hand: the
archcheck layer map carried a special rule for `kernel/agent` because the path
alone did not say which module the package belonged to.

## 2. Why the package kept its name

The target architecture
(`architecture/20-target-architecture.md:119,182`) puts the loop at
`modules/runs/internal/loop`. This slice could **not** go there, and the reason
is structural rather than stylistic: the loop is imported from **outside** the
runs module.

| importer | why |
|---|---|
| `kernel/runtime` | the daemon's run engine |
| `kernel/runtime/runexec` | the runner that builds the loop's configuration |
| `kernel/scheduler` | scheduled runs |
| `cmd/agezt` | the composition root |
| `kernel/modules/runs/delegation` | the delegated child loop |
| `kernel/modules/runs/loopcfg` | the loop configuration's types |
| `kernel/modules/runs/steer` | the `Steerer` port's implementation |

`internal/` means *importable only from inside the tree*. The daemon is the
daemon, and the daemon runs the loop. So the loop's address is
`kernel/modules/runs/agent` — the module's own subtree, one level below the
module's root — and the package name **stayed** `agent`, because the name is
the thing every caller already writes (`agent.Run`, `agent.LoopConfig`,
`agent.Provider`) and the target architecture's phrase *"the kept
`agent.Run`"* asks for the symbol to survive.

The `internal/loop` address becomes reachable in the **W3 exit** — when
`kernel/runtime` and `kernel/controlplane` are dissolved and the daemon is
reduced to a host, the loop's only remaining importer is the runs module
itself.

## 3. What moved, and what stayed

**Moved:** the whole package, `git mv kernel/agent kernel/modules/runs/agent` —
**51 files** (20 production, 31 test), renamed in the index as renames, so the
history follows the code.

**Repointed:** **45 files** changed their import path from
`github.com/agezt/agezt/kernel/agent` to
`github.com/agezt/agezt/kernel/modules/runs/agent` — **17** in seven
production packages (`cmd/agezt`, `kernel/runtime`, `kernel/runtime/runexec`,
`kernel/scheduler`, and the module's own `delegation`, `loopcfg` and `steer`)
and **28** in nine test-only packages (16 of them the module's **own** test
files; the platform's `policyctx`, `schema` and `toolaudit` keep their
compatibility ratchets, which pin `agent.Policy == policyapi.Policy`). Every
one of them was reformatted afterwards, because `gofmt` reorders an import
block when the path's segment count changes.

**The classifier lost a special case.** `tools/archcheck/layers.json` carried a
hand-written rule for `kernel/agent`. It is **gone**: the tree now says the
same thing by itself. The rule had been *equivalent* to the generic
`kernel/modules/*/...` pattern, so the classification did not change — the
allowlist stayed at **one** entry (`cross-module
kernel/modules/agents/selfrepair -> kernel/runtime`, the parallel session's
edge) and the checker stayed green across **335** packages.

**Stayed in the daemon:** nothing was added to it. The daemon already held the
loop's *state* (its providers, its tools, its policy hook, its bus, its
artifact port); this slice moved the loop's *rule* without touching the
daemon's shape.

**Stayed in the module:** the `Steerer` and `Directive` ports. They are the
**consumer's** ports — the loop declares what it needs from a steering
controller, and the steering module implements it. This is the same seam
W3.76f drew for the delegation engine (`delegation.Host`).

**Left for a later slice:** the alias block in `agent.go` (53 lines) — the
~1,403 internal uses of `agent.X` still resolve through it. Removing it means
repointing every use at `llm.X` / `toolapi.X` / `policyapi.X`, which is a
mechanical slice of its own.

## 4. The measurement

Against the base `bfd72355`:

| | base | now | delta |
|---|---:|---:|---:|
| `kernel/agent` | 20 files / 2,629 | **0 / 0** | −20 / **−2,629** |
| `kernel/modules/runs` | 43 files / 4,608 | **63 / 7,264** | +20 / **+2,656** |
| the loop module | — | 20 / **2,656** | |
| `kernel/runtime` (upper level) | 54 / 6,281 | **54 / 6,281** | **0** |
| `kernel/runtime` (recursive) | 60 / 7,173 | **60 / 7,173** | **0** |
| the daemon's public verbs | **238** | **238** | **0** |
| the contract's rows | **10,406** | **10,406** | **0** |
| the application layer's lines | **38,009** | **38,009** | **0** |
| the control-plane's lines | **7,570** | **7,570** | **0** |

Three readings, and they are the point of the slice:

1. **The daemon did not move.** Not one line, not one public verb, not one
   file. The loop was never the daemon's code — it was a package the daemon
   imported — and this slice only changed where that package lives. The
   daemon's line count is **6,281** before and after, and the shape of the
   slice is a **move**, not a rewrite.
2. **The family grew by 27 lines** (2,629 → 2,656), and every one of them is
   in the package's own `doc.go`: the file now says **why the loop lives in
   the runs module** and which half of the family is the contract's. The
   code is byte-for-byte the same code; only its address and its package
   documentation changed.
3. **The project is not spreading.** The total kernel production line count
   is **133,801** at the programme's base (`9863c497`), **134,749** at this
   slice's base, and **134,785** in the working tree — of which **+9** belongs
   to a parallel session's own dirty files (`kernel/modules/agents` +2,
   `kernel/modules/reasoning` +7). This slice's own share is **+27** — and
   **2,629** lines changed address while the project's own total moved by
   **27**.

## 5. The tests

| | count |
|---|---:|
| the loop module | **139** test functions (115 moved with the code, **24** new) in 34 files (31 moved, 3 new) |
| the daemon's runtime | 400 → **400** (+0) |

The module's 24 new test functions are two different kinds of test:

- **4 ratchet tests** (`home_internal_test.go`) that hold the move in place:
  the loop lives in the runs module; the old path `kernel/agent` appears
  **nowhere** in the tree (a repo-wide scan, with the closing quote in the
  needle so `kernel/agentgw` is not a false positive); the module's files
  never name the daemon; and the loop keeps its contract aliases.
- **20 calibration tests** (`rules_test.go`, `rules_internal_test.go`) that
  close the loop's rules from the side they were not closed on — see §6.

## 6. The mutations

**62 mutations, 62 caught, 0 survived, 0 broken**, in two independent passes
with **byte-identical** logs (SHA-256
`74189F5CCE83A32128E0E6F9EB3E9E94E5391930058D14A80F0E43F8C82582CA`).

The loop's rules had been tested on **one** side only — the happy path, plus
the boundary each test happened to touch. The calibration of the first pass
found **six** survivors, and every one was a **gap in the tests** or a
**fault in the mutation** — never a dead rule:

1. **The empty actor (M05)** — the loop refuses a config with no actor, and
   the refusal must come from the **loop**, before the run starts. The first
   test accepted any error that mentioned "actor" — and the **event layer**
   says "actor required" too, so the mutation survived. The test now pins
   the loop's own message (`agent: actor required`) **and** that nothing
   reached the journal.
2. **The segment arithmetic (M16)** — a continuation grants a **full**
   `MaxIter` segment. The first test gave the run a budget of three
   continuations, which a broken segment (one round instead of two) could
   still spend. The test now grants **two** and counts the `task.continued`
   events: a short segment needs a third continuation, and the run stops at
   the cap.
3. **The selector's intent (M24)** — the tool selector is told **what the run
   is for**. The first test used the lexical selector, which reads the intent
   *and* the conversation — and the intent is in both, so emptying one left
   the other. The test now hands the loop a selector that records the
   request's intent and asserts it is the run's.
4. **The memo store (M53)** — the first mutation removed the error check from
   the gate's store line, which changes nothing: the memo's own `Set` refuses
   an error result too. It was an **equivalent** mutation. The mutation now
   disables the store entirely, which the memo test can see.
5. **The trusted rendering (M57)** — the renderer returns trusted content
   untouched. The loop only calls the renderer for **untrusted** boundaries,
   so the mutation was invisible from the loop; the test now calls the
   renderer directly, which is where the rule actually lives.
6. **The taint merge (M60)** — the merge keeps the directive-like flag. The
   policy does not read that field (it reads the **causal window**), so the
   mutation was invisible through the run; the test now calls the merge
   directly.

Two findings belong to the **harness** — one is a repetition of W3.76f's, the
other is new:

- **A `-run` pattern that matches no test exits 0** — the W3.76f finding,
   reproduced. The harness now runs `go test -v`, so the `[no tests to run]`
   marker is in the output and a wrong test name is **BROKEN** (the
   mutation's fault) instead of a surviving rule.
- **Windows keeps a user-mapped section on a source file** right after a
   `go test` that compiled it, and the harness's restore write failed with
   *"the requested operation cannot be performed on a file with a user-mapped
   section open"*. The restore (and the mutation write) now retries. The
   failure is worth recording: it left a **mutated file on disk**, and the
   next run's preflight would have measured the wrong tree.

## 7. The ratchet

`kernel/modules/runs/agent/home_internal_test.go` holds the move in place:

- the loop lives under `kernel/modules/runs/`;
- the old path `kernel/agent` is **gone from the whole tree** — the test
  scans the repository (about three seconds) for the old import path, with
  the needle closed by a quote so `kernel/agentgw` cannot trip it;
- the loop's files never name the daemon (`kernel/runtime`);
- the loop keeps its contract aliases (`agent.Provider` is `llm.Provider`).

The classifier's side of the move is held by `tools/archcheck`: the
hand-written `kernel/agent` rule is gone from `layers.json`, and the checker
still places **335** packages with **one** allowlisted import violation (the
parallel session's `kernel/modules/agents/selfrepair -> kernel/runtime`
edge) and **zero** remaining allowlisted call sites.

## 8. What is left

The run engine's families are all moved. What remains of W3.76 is
**mechanical**:

- the **alias block** (`agent.go`, 53 lines) — ~1,403 internal uses of
  `agent.X` still resolve through it; removing it means repointing them at
  `llm.X` / `toolapi.X` / `policyapi.X`.

And then the **W3 exit**: `kernel/runtime` and `kernel/controlplane`
dissolved, the daemon reduced to a host. That exit is what finally makes
`modules/runs/internal/loop` reachable: today the daemon, the scheduler,
the runner and `cmd/agezt` all import the loop, and `internal/` means
*importable only from inside the tree*.

After that: the **W2** domains (seven PRs), **W4** (five), **W5** (four).

## 9. The numbers that did NOT move

- the daemon's **public verbs**: 238 → 238.
- the **contract**: +0 rows. The loop needed no new contract type.
- the **application layer**: +0 lines. The loop's address is not the
  application's business.
- the **control plane**: +0 lines.
- the **daemon's own lines**: +0.

