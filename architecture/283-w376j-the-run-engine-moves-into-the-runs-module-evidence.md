# W3.76j — the run engine moves into the runs module

**Slice:** W3.76j (the tenth of the run-engine programme, the eighteenth of the
runtime's dissolution)
**Base:** `ed1b43c7` (the W3.76i merge)
**Date:** 2026-11-02

---

## 1. The family

The **run engine** is the thing that executes a run: it registers the run,
stamps the context, frames the intent, builds the system prompt, assembles
the loop's configuration, claims the resume ticket, calls `agent.Run`, and
then does the post-run work (distill, forge, the agent lifecycle).

It lived in `kernel/runtime/runexec` — a sub-package of the daemon — with
**892 non-blank lines** across six production files:

| file | non-blank | what it holds |
|---|---:|---|
| `api.go` | 126 | the `KernelAPI` port — the runner's only channel to its host |
| `runner.go` | 209 | `Runner`, `New`, `Run`, `RunAssured` |
| `runner_lifecycle.go` | 201 | `RunWith` — **the real run body** — and the lifecycle |
| `runner_retry.go` | 233 | `RunWithRetry`, the journal re-exports (`Why`, `Causes`, `ParentOf`, `Verify`) |
| `helpers.go` | 86 | the transcript builder, the heuristic bypass, the vision limits |
| `doc.go` | 37 | the package's own documentation |

The daemon kept a second half of the family — **387 lines** across four
files that are the **doors** and the **state**:

| file | non-blank | what it holds |
|---|---:|---|
| `runexec.go` | 100 | `Run`, `RunAssured`, `RunWithRetry` wrappers + the retry vocabulary |
| `runexec_lifecycle.go` | 201 | `RunWith` (the correlation check and the delegation), `setupRunState`, `cleanupRunState` (the five-mutex release) |
| `runexec_delegates.go` | 54 | `FoldRunTools`, `Why`, `Causes`, `ParentOf`, `Verify` |
| `runexec_helpers.go` | 32 | the retry helpers and the elision constants |

**1,279 lines** in all: an engine and its doors.

## 2. Why the seam was already the right one

The run engine was extracted from the daemon early (the Day 12-23
sub-package split) with a **port** between them: `runexec.KernelAPI`. The
runner never imports the daemon; it calls its host through that interface,
and the daemon implements it (`var _ runner.KernelAPI = (*Kernel)(nil)`).

So the slice did not have to invent a seam — it had to **move a package that
already had one**. That is the same shape as W3.76f (the delegation engine's
`delegation.Host` port) and W3.76g (the loop configuration's `loopcfg.Deps`):
the module owns the rule, the daemon keeps the state.

The measurement says the seam held: the runner's imports are **eleven
kernel packages** (approval, bus, configcenter, edict, event, intent,
journal, reflect, scheduler, state, warden), **fourteen contract rows** and
**two module packages** (`modules/runs/agent`, `modules/runs/resume`). **No
module outside the runs module** — so the move creates no cross-module edge,
and the archcheck allowlist stays at **one** entry.

## 3. What moved, and what stayed

**Moved:** `kernel/runtime/runexec` → `kernel/modules/runs/runner`
(`git mv`, seven files including the test), and the package name changed from
`runexec` to `runner` — the runs module names its parts by role (registry,
lifecycle, prompt, loopcfg, delegation, retry, steer), and "the run engine"
is the role. The daemon's four door files were renamed with it
(`runexec.go` → `runner.go` and so on), so the tree speaks one word.

**Stayed in the daemon:** the doors. `(*Kernel).Run`, `RunAssured`,
`RunWith`, `RunWithRetry` and the run-state setup are unchanged, and the
daemon's **public verbs stayed at 238**.

**Stayed the port:** `KernelAPI` — unchanged, and now declared in the
module.

**Changed in the daemon's own ratchets:** two of them, and both changes are
the move showing up honestly:

- `TestTheKernelHoldsOnlyTheSubManagersItCalls` used to expect the daemon to
  hold exactly one sub-manager — `runexec`. With the runner in the module,
  the daemon holds **none**: a field that points into a module is the
  module's, not a second copy of a surface.
- `TestTheDaemonTalksToTheModuleThroughTheRegistry` watched
  `runexec_lifecycle.go` by name; it now watches `runner_lifecycle.go`.

## 4. The measurement

Against the base `ed1b43c7`:

| | base | now | delta |
|---|---:|---:|---:|
| `kernel/runtime/runexec` | 892 | **0** | **−892** |
| `kernel/modules/runs` | 8,123 | **9,017** | **+894** |
| the runner package | — | 6 files / **894** | |
| `kernel/runtime` (upper level) | 54 / 6,281 | **54 / 6,281** | **0** |
| `kernel/runtime` (**recursive**) | 60 / **7,173** | **60 / 6,281** | **−892** |
| the daemon's public verbs | **238** | **238** | **0** |
| the contract's rows | **10,406** | **10,406** | **0** |
| the application layer's lines | **38,009** | **38,009** | **0** |
| the control-plane's lines | **7,570** | **7,570** | **0** |

The line that matters is the **recursive** one. The daemon's *own* level did
not change at all — the doors are still the doors — but the daemon's
**sub-tree** lost **892** lines, because the engine is no longer under it.
The module gained **894**: the engine's 892 and **+2** lines of package
documentation (`doc.go` now says why the runner lives in the runs module and
which side of the seam it is on).

The project's own total moved by **+2** — and nothing else in the tree
changed size.

## 5. The tests

The daemon's and the module's test functions are unchanged in count; one test
file moved with the package (`helpers_test.go`) and one new file arrived.

**The ratchet** is new: `kernel/modules/runs/runner/home_internal_test.go`,
four tests that hold the move in place:

1. **The runner lives in the runs module** — the package's own path ends in
   `/kernel/modules/runs/runner`.
2. **The old path is gone from the whole tree** — a repository-wide scan for
   `"github.com/agezt/agezt/kernel/runtime/runexec"`, with the needle closed
   by a quote so that a longer path cannot trip it, and with this file
   excluded because it names the old path on purpose.
3. **The runner never names the daemon** — no file in the package may import
   `kernel/runtime`: the engine speaks to its host through the port.
4. **The host still implements the port** — `KernelAPI` keeps its shape and
   carries the methods the runner calls.

**Both probes fire.** Putting the old import path back into a file makes the
scan fail; putting a daemon import into the package makes the third test
fail. (A ratchet that is never probed is a ratchet that never fires — the
lesson W3.76i paid for.)

## 6. Why this slice has no mutation run

No rule changed: the engine's code is byte-for-byte the same code, its port
is the same port, and its behaviour is the same behaviour. A mutation run
would measure the daemon's rules, which this slice did not touch.

What this slice needs is the evidence of a **move**, and that is what it has:
the compiler (every import and every call site is type-checked), the
repository scan (the old path is gone), the port probe (the seam still
holds), the daemon's two structural ratchets (updated to the move, and
re-verified), the full test suite, and the thirteen gates.

## 7. The documentation

The living codemap was corrected, because a map that names a path the tree
no longer has is a map of a different tree:

- `architecture/00-README.md` — the package index, the flow diagram and the
  run-engine row;
- `architecture/04-agent-runtime.md` — the scope, the dependency map, the
  daemon's file table and the run-engine row;
- `architecture/05`, `06`, `07` — the **fan-in tables**, row by row: the
  packages the runner imports (`configcenter`, `edict`, `warden`, `approval`,
  `state`, `journal`, `scheduler`) now list the module's runner beside the
  daemon; the packages it does **not** import (`memory`, `worldmodel`,
  `artifact`, `cadence`, `standing`, `skill`, `toolforge`, the media tools)
  simply lose `runexec`;
- `architecture/20-target-architecture.md` — the runs module's row.

The **historical** records were left alone on purpose: the old evidence
documents and the earlier roadmap rows describe the tree as it was when the
runner was the daemon's sub-package.

## 8. What is left

The daemon now holds **no sub-manager of its own**, and its sub-tree is
**6,281** lines — the same as its own level. What is left of the exit:

- the daemon's **composition root** (`compose.go`, 497 lines) and its
  **Kernel** (`runtime.go`, 665 lines) — the state the daemon holds;
- the **accessors** family (965 lines) — the daemon's read surface;
- the **control plane** (`kernel/controlplane`, 7,570 lines), which the exit
  dissolves alongside the runtime;
- the loop's address: once the daemon, the scheduler and `cmd/agezt` stop
  importing the loop, `modules/runs/internal/loop` becomes reachable.

After the exit: the **W2** domains (seven PRs), **W4** (five), **W5** (four).
