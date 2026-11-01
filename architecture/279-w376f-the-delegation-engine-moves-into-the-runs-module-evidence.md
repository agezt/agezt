# W3.76f — the delegation engine moves into the runs module

**Slice:** W3.76f (the sixth of the run-engine programme, the fourteenth of the
runtime's dissolution)
**Base:** `3578e258` (the W3.76e merge)
**Date:** 2026-11-01

---

## 1. The family

The delegation engine was the run engine's **most entangled** family, and the
measurements said so before a line was written.

| piece | file | non-blank |
|---|---|---:|
| the sub-agent's types | `kernel/runtime/subagent.go` | 15 |
| the spawn preparation (guards, identity, journal) | `kernel/runtime/subagent_prep.go` | 303 |
| the child loop + the retry policy | `kernel/runtime/subagent_exec.go` | 102 |
| the async spawn + the collect | `kernel/runtime/subagent_run.go` | 131 |
| the spend walk (the M48 cap) | `kernel/runtime/subagent_spend.go` | 54 |
| the `delegate` / `delegate_await` tools | `kernel/runtime/subagent_tool.go` | 189 |
| the shared vocabulary (depth key, system prompt, payload helpers) | `kernel/delegation/*.go` | 100 |
| **total** | | **894** |

The family was split across **two** trees: the daemon held the engine, and a
package at the kernel's root (`kernel/delegation`) held the vocabulary the
engine spoke. That split was itself the smell — a vocabulary package outside
the module tree, imported by exactly the family it belonged to and by nothing
else.

W3.76b's measurement of the steering said the family touches the daemon in
**four** ways and is therefore "port-shaped". The delegation family touches it
in far more: the roster, the live model, the live identity, the halt latch, the
correlation mint, the bus, the journal, the steering registry, the skill forge,
the run registry, the fan-out tally, the tree tally, the spawn handles, the
child's loop configuration and the child loop itself. **Fifteen.**

That is why the slice did not begin with a copy: it began with a port.

## 2. The seam: policy versus mechanism

The move is not a file shuffle. It is a decision about **who owns what**:

- The **module** owns the **policy**: who may delegate, under which bounds,
  with which identity, on which model, with which retry policy — and the state
  that policy needs (the fan-out tally, the tree-total tally, the async spawn
  handles).
- The **daemon** keeps the **mechanism**: it builds the child's loop
  configuration and runs the child's loop, because the loop is the next family
  to move (`kernel/agent`, 2,629 lines) and the loop configuration the one
  after (`kernel/runtime/loopconfig.go`, 99 lines).

The port is `delegation.Host` (`kernel/modules/runs/delegation/host.go`) —
**fourteen doors**:

| door | what it carries | why the module cannot do it itself |
|---|---|---|
| `RosterGet` | the named agent's profile | the roster is the daemon's |
| `ProfileModel` | the profile's effective model | the override vocabulary is the daemon's configuration |
| `Model` | the LIVE default model | hot-swapped on provider reload |
| `System` | the LIVE default identity | editable at runtime |
| `Halted` / `HaltedError` | the halt latch and its error identity | the daemon owns the sentinel — `errors.Is(err, runtime.ErrHalted)` still holds |
| `NewCorrelation` | a fresh correlation id | the daemon's mint |
| `Publish` | the bus | the daemon's bus |
| `RangeJournal` | the journal | the daemon's journal (the spend walk reads it) |
| `RegisterSteer` / `UnregisterSteer` | the shared steering registry | the daemon's root runs use it too — a child must be steerable from the cockpit (M631) |
| `RecordSkillOutcome` | the skill forge's outcome | the forge is the daemon's |
| `BaseLoopConfig` | the child's base loop configuration | the loop configuration is the next family to move |
| `CompleteChildLifecycle` | the durable lifecycle of a finished child | the run lifecycle is the daemon's |

The **Deps** carry the rest: the live config provider, the child-runner
callback, the prompt service, the live-run registry and the sleeper.

Two decisions worth naming:

1. **The config is a live provider, not a snapshot.** The prompt module takes a
   snapshot because the daemon's config is read and never written. The
   delegation caps are **operator settings** — an operator raising
   `SubAgentMaxDepth` at runtime must see the change on the next delegation.
   A snapshot would have silently frozen them.
2. **The retry vocabulary became its own package.** `retryReason`,
   `agentRetryable` and `retryDelay` lived in the daemon's unexported space and
   were used by BOTH the root-run retry and the delegation's child retry. The
   module could not reach them. Rather than duplicate them or ask the daemon
   for every decision, they moved to `kernel/modules/runs/retry` — one
   implementation, two callers — and the daemon keeps thin wrappers so its own
   API (and the `runexec` sub-package's) is unchanged. The halt sentinel is a
   **parameter** there: the daemon owns the identity of its halt error and the
   module asks for it.

## 3. What moved, and what stayed

**Moved to `kernel/modules/runs/delegation`** (10 files, **1,149** non-blank):
the vocabulary (`vocabulary.go`), the policy (`config.go`), the port
(`host.go`), the plan (`plan.go`), the service (`service.go`), the preparation
(`prep.go`), the execution (`exec.go`), the spend walk (`spend.go`) and the
tools (`tool.go`).

**Moved to `kernel/modules/runs/retry`** (1 file, **91**): the retry
vocabulary.

**Kept in the daemon** (`kernel/runtime/subagent_doors.go`, 154 lines): the
fourteen doors, the config projection and the child-runner callback. The file
is named `subagent_doors.go`, not `delegation.go`, because the daemon's
`delegation.go` never existed and the `runctx` ratchet from W3.76c watches the
names the run context left behind.

**Deleted:** the six daemon files, the four files of `kernel/delegation`, and
the daemon's own fan-out, tree and spawn maps (with their three mutexes) —
those maps are the **engine's** state; no root run ever touched them.

**Not moved, and why:**

- **The child's loop configuration and the child loop** — the next two
  families (`loopconfig.go`, `kernel/agent`). They are the daemon's
  *mechanism*.
- **The steering registry** (`k.steers`) — the daemon's root runs use it too.
  The module publishes and withdraws the child's control surface through two
  doors.
- **`kernel/runtime/runexec_delegates.go`** (54 lines) — `FoldRunTools`,
  `Why`, `Causes`, `ParentOf`, `Verify`. These are the **provenance** verbs,
  not the delegation engine's: they are one-line passthroughs to the runexec
  sub-package's runner and they belong to the run-exec family, not to the
  delegation policy.

## 4. The measurement

Against the base `3578e258`:

| | base | now | delta |
|---|---:|---:|---:|
| `kernel/runtime` (upper level) | 61 files / 7,269 | **56 / 6,573** | −5 / **−696** |
| `kernel/runtime` (recursive) | 67 files / 8,161 | **62 / 7,465** | −5 / **−696** |
| `kernel/modules/runs` | 26 files / 2,842 | **37 / 4,082** | +11 / **+1,240** |
| the delegation module | — | 10 / **1,149** | |
| the retry vocabulary | — | 1 / **91** | |
| the daemon's public verbs | **238** | **238** | **0** |
| the contract's rows | — | — | **0** |
| the application layer's files | — | — | **0** |

The daemon's verb count is unchanged: the engine's verbs were all unexported
(`prepareSubAgent`, `executeSubAgent`, `runSubAgent`, `runSubAgentAsync`,
`awaitSubAgent`, `subAgentSpendMicrocents`, `validateDelegationManager`), and
the daemon's new file adds no `*Kernel` method at all — the doors are methods
on an unexported adapter (`delegationHost`).

**The family's own line count went up** (894 → 1,240 with the retry
vocabulary). That is the same shape as W3.76b and W3.76c: *moving a rule moves
the CONDITIONS, not the lines*. The module's lines are the moved lines **plus**
the port, the plan, the configuration and the documentation of both — and the
port is the price of the move.

## 5. The tests

| | count |
|---|---:|
| the delegation module | **47** test functions (10 moved with the code, 37 new) |
| the retry vocabulary | **9** test functions (new) |
| the daemon's runtime | 388 → **395** (+7: 5 ratchet + 2 door tests) |

The family was **under-tested**: it had nine vocabulary tests and no test at
all for its guards, its identity resolution or its async path. The new tests
ask the engine the questions it had never been asked: what happens at the
depth bound, at the fan-out bound, at the tree-total bound, at the spend
bound; which identity the child runs as; what the spawn event carries; what
happens when a foreign run tries to collect someone else's spawn; what the
retry policy does when it is exhausted; what the spend walk does with a cyclic
journal.

The daemon's own integration tests (`kernel/runtime/subagent_test.go`) stayed
where they are: they exercise the daemon's wiring end-to-end, and they are the
ones that prove the **policy crosses the port** — the fan-out bound, the spend
bound and the tree-total bound are all configured in the daemon and enforced in
the module, and three of the mutations below break exactly that crossing.

## 6. The mutations

**55 mutations, 55 caught, 0 survived, 0 broken**, in two independent passes
with **byte-identical** logs (SHA-256
`42B6AC05F384D62615B52064DA280A7EBDFB8F724E9F9011B48B70538990D51D`).

The calibration of the first pass found four things, and every one of them was
a **gap in the tests**, not a dead rule:

1. **The tree root** — the mutation "the child does not carry the tree root"
   survived, because the test's context already carried the root: the child
   inherited it anyway. The gap was a case where the spawning run **is** the
   tree root (no root on the context → the root must be seeded from the
   parent). Now there is a test for it.
2. **The spawn's subject** — the test asserted the payload and the
   correlation, never the subject. Now it does.
3. **The child's parent correlation** — the loop configuration's
   `ParentCorrelation` was never asserted. Now it is.
4. **The final backoff clamp** — the mutation "the backoff is not clamped"
   survived because the *exponential* path already caps itself inside the
   growth loop, so the final clamp is only observable on a **flat** backoff.
   Now there is a test for that.

One more finding belongs to the **harness**, not to the code: a `-run` pattern
that matches **no test** exits **0**, and the harness read that as a survivor.
Two mutations were pointed at a test name that did not exist. The harness now
treats "no tests to run" as **BROKEN** — the mutation's fault — instead of
letting it masquerade as a surviving rule.

## 7. The ratchet

`kernel/runtime/subagent_ratchet_test.go` holds the move in place:

- the daemon's six delegation files and the `kernel/delegation` package are
  **gone**;
- the daemon holds **no** fan-out, tree or spawn tally, and **no** delegation
  verb;
- the daemon **keeps** its fourteen doors and the child-runner callback;
- the daemon holds a **pointer** to the module's engine, and the module's
  files **import the daemon never**.

The `registry_ratchet_test.go` from W3.76d was updated too: it used to watch
the daemon's `subagent_run.go` for the registry's map; that file is the
module's now, so the ratchet watches
`kernel/modules/runs/delegation/exec.go` and asks the same question there.

## 8. What is left

The run engine's remaining families:

- **the loop configuration** (`kernel/runtime/loopconfig.go`, 99 lines) — the
  next cheapest move; the daemon's shared governance/capacity base.
- **the loop** (`kernel/agent`, 2,629 lines across 20 files) — the biggest,
  and the most entangled with the contract layer (123 production files import
  `kernel/contract/llm`).
- then the W3 exit: `kernel/runtime` and `kernel/controlplane` dissolved, the
  daemon reduced to a host.

And after that: the **W2** domains (seven PRs), **W4** (five), **W5** (four).

## 9. The numbers that did NOT move

- the daemon's **public verbs**: 238 → 238.
- the **contract**: +0 rows. The delegation engine needed no new contract
  type: the profile, the retry policy and the tool surface are all
  `agentapi`/`toolapi` rows that already existed.
- the **application layer**: +0 files. The daemon's 52 direct calls into the
  application layer are untouched — the delegation engine is behind the
  daemon's tools, not behind the app.

The honest summary of the slice: the daemon lost **696** lines and the module
gained **1,240**. The project is not shrinking; it is **changing shape** — the
rules are moving to the module that owns the thing they describe, and the
daemon is keeping only the doors and the mechanism.
