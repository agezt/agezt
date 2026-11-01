# W3.76g — the loop configuration moves into the runs module

**Slice:** W3.76g (the seventh of the run-engine programme, the fifteenth of the
runtime's dissolution)
**Base:** `7b4355b8` (the W3.76f merge)
**Date:** 2026-11-01

---

## 1. The family

The loop configuration is the shape **every governed run's loop must have** —
whether it is a root run (`RunWith`) or a delegated child
(`executeSubAgent`). Before this slice it was the daemon's
`buildLoopConfig`, and it was four files:

| piece | file | non-blank |
|---|---|---:|
| the assembler (which knobs the loop gets, and how the tool surface, the tool selector, the context budget and the summarizer are wired) | `kernel/runtime/loopconfig.go` | 100 |
| the tool-set shaping rules (the run's allowlist, the agent profile's allow/deny policy, the noise policy's prompt-tool pruning) | `kernel/runtime/toolpolicy.go` | 95 |
| the deferred-discovery rule and the tool that serves it | `kernel/runtime/toolsearch.go` | 151 |
| the elided-output summarizer (the elision half of `runexec_helpers.go`) | `kernel/runtime/runexec_helpers.go` | 44 |
| **total** | | **390** |

The family was **split across two kinds of file**: three daemon files that
held a *rule*, and one daemon file that held a *rule and a retry vocabulary
together* — `runexec_helpers.go` had the retry wrappers (`retryReason`,
`agentRetryable`, `retryDelay`) **and** the summarizer, and the two had
nothing to do with each other except that they were both in a file called
"helpers".

The measurement said the family was **port-shaped**: it touches the daemon in
seven ways — the effective configuration, the live tool surface, the tool-phase
service, the bus, the policy hook, the noise-policy hook, the artifact port
and the model catalog — and it owns **no** stored state of its own. A family
with a rule and no state belongs to the module whose thing the rule describes.

## 2. The seam: rule versus state

The move is a decision about **who owns what**:

- The **module** owns the **rule**: what a governed run's loop looks like —
  which knobs it gets, which tools it is offered, which selector chooses them,
  how its context is budgeted, when its outputs are offloaded and summarized.
- The **daemon** keeps the **state** the rule is built from: its effective
  configuration, its live tool surface (the merged MCP attachments and the
  forged script tools), its tool-phase service, its bus, its policy hook, its
  noise-policy hook, its artifact port and its model catalog.

The port is `loopcfg.Deps` (`kernel/modules/runs/loopcfg/config.go`) — seven
collaborators:

| dependency | what it carries | why the module cannot do it itself |
|---|---|---|
| `Tools func(ctx) map[string]toolapi.Tool` | the run's live tool surface | the daemon holds the tool map, the MCP attachments and the script forge |
| `Phases toolphaseapi.Phases` | the batch-aware tool invocation service | the daemon's |
| `Bus *bus.Bus` | the event bus | the daemon's |
| `Policy agent.Policy` | the tool-call policy hook | the daemon's Edict-backed hook |
| `Notify func(...)` | the noise-policy result hook | the daemon's |
| `Artifacts agent.ArtifactPutter` | the artifact offload port | the daemon's blob store |
| `Catalog func() *catalogapi.Catalog` | the live model catalog | the daemon swaps it on reload (M477) |

Two decisions worth naming:

1. **The configuration crosses as a PROJECTION, not as a snapshot.** The
   daemon's effective configuration is **per-run**: the daemon-wide config,
   the operator's live edits and this agent's own overrides (`AGEZT_MAX_ITER`
   and friends). A named agent retuning its own iteration ceiling must be
   honoured — so the daemon reads the effective configuration for **every**
   run and projects the knobs the loop needs into `loopcfg.Config`. The
   projection is a **parameter of the build**, not a snapshot taken once.
2. **The catalog crosses as an ACCESSOR, not a pointer.** The daemon swaps the
   catalog on reload (M477); a captured pointer would freeze the context
   window of the model that happened to be current when the service was built.
   `Catalog` is a `func() *catalogapi.Catalog`.

## 3. What moved, and what stayed

**Moved to `kernel/modules/runs/loopcfg`** (6 files, **519** non-blank): the
package's own documentation (`doc.go`), the configuration and the port
(`config.go`), the rule (`service.go`), the tool-set shaping rules (`tools.go`),
the deferred discovery (`search.go`) and the elided-output summarizer
(`elision.go`).

**Kept in the daemon** (`kernel/runtime/loopcfg_doors.go`, **78** non-blank):
the configuration projection (`loopcfgConfig`), the dependency wiring
(`loopcfgDeps`) and the door the two callers use
(`BuildLoopConfig`). The file is named `loopcfg_doors.go` and not
`loopconfig.go` because the daemon's `loopconfig.go` is **gone** with the
assembler it held — and the ratchet from W3.76c watches the names the run
context left behind.

**Deleted:** the three daemon files (`loopconfig.go`, `toolpolicy.go`,
`toolsearch.go`) and the elision half of `runexec_helpers.go`. The retry half
of that file **stayed** — the retry vocabulary is the daemon's (the delegation
engine's own retry lives in `kernel/modules/runs/retry` since W3.76f).

**The application layer moved one call site.** `kernel/app/runs/run_dryrun.go`
shaped its dry-run plan's tool set with `runtime.AgentTools`; the rule is the
module's now, so the plan asks `loopcfg.AgentTools`. This is deliberate: the
dry-run plan and the run must shape the tool set with the **same** rule, or
the plan could list a tool the run would not offer.

**Not moved, and why:**

- **The run's identity fields** (the model chain, the agent, the actor, the
  correlation, the system prompt, the wake source, the steering control, the
  cost ceiling's source) — they belong to the **caller**: the root run and the
  delegated child each carry their own.
- **The policy hook itself** (`kernel/runtime/policy.go`) — the daemon's Edict
  engine, its approvals and its guards are the daemon's mechanism. What moved
  is the **question** the hook asks about the agent's tool policy
  (`loopcfg.AgentToolPolicyDenial`): the rule of what the agent's allow/deny
  list means is the module's, the hook is the daemon's.
- **The retry vocabulary** — the daemon's, and the delegation engine's (in
  `kernel/modules/runs/retry`).

## 4. The measurement

Against the base `7b4355b8`:

| | base | now | delta |
|---|---:|---:|---:|
| `kernel/runtime` (upper level) | 56 files / 6,573 | **54 / 6,281** | −2 / **−292** |
| `kernel/runtime` (recursive) | 62 files / 7,465 | **60 / 7,173** | −2 / **−292** |
| `kernel/modules/runs` | 37 files / 4,082 | **43 / 4,601** | +6 / **+519** |
| the loop-configuration module | — | 6 / **519** | |
| the daemon's door file | — | 1 / **78** | |
| the daemon's public verbs | **238** | **238** | **0** |
| the contract's rows | **10,406** | **10,406** | **0** |
| the application layer's lines | **38,006** | **38,009** | **+3** (one call site and its comment) |

The daemon's verb count is unchanged: the rule's verbs were all unexported
(`filterTools`, `applyAgentToolPolicy`, `applyAgentNoisePolicyToPromptTools`,
`withToolSearch`, `makeElidedSummarizer`, `buildLoopConfig`), and the daemon's
new file adds no `*Kernel` method — the door it keeps is the **public**
`BuildLoopConfig` that already existed.

**The family's own line count went up** (390 → 597). That is the same shape
as W3.76b, W3.76c, W3.76e and W3.76f: *moving a rule moves the CONDITIONS, not
the lines*. The module's lines are the moved lines **plus** the port, the
configuration projection and the documentation of both — and the port is the
price of the move.

**The project is not spreading.** The total kernel production line count is
**133,801** at the programme's base (`9863c497`), **134,512** after W3.76f and
**134,751** now: **+950** lines over the whole programme, **+0,7%**. The
daemon shrinks while the module grows, and the families that were never
touched are **bit-for-bit identical**.

## 5. The tests

| | count |
|---|---:|
| the loop-configuration module | **38** test functions (5 moved with the code, 33 new) |
| the daemon's runtime | 393 → **400** (+12: 8 door tests, 4 ratchet tests) |

The family was **under-tested**: three files with five test functions between
them, and **nothing** at all for the assembler — the rule that decides what a
run's loop gets. The new tests ask the rule the questions it had never been
asked: which knobs land on the loop; that the cost function is always there
(without it the per-run spend ceiling is inert); that the memo and the rescue
marker are always there; that the run's allowlist, the agent's allow/deny
policy and the noise policy all narrow the tool surface; that the catalog is
deferred above the bound and the small-schema selector is used there; that the
auto context budget comes from the catalog and an explicit budget wins over
it; that an unknown model leaves compaction **off** rather than guessed; that
the summarizer calls the run's own provider with the run's model, correlation
and task type; that a reasoning model gets the roomy summary cap (M926); that
the summarizer's input is capped and its answer trimmed; that the discovery
tool declares a governed capability (it was default-denied once); and that the
limit is clamped and the score ranks the exact name first.

The daemon's own tests (`kernel/runtime/loopcfg_doors_test.go`) are the ones
that prove the **state crosses the port**: the daemon's effective
configuration, its live tool surface, its tool-phase service, its bus, its
policy hook, its noise hook and its catalog all reach the loop.

## 6. The mutations

**56 mutations, 56 caught, 0 survived, 0 broken**, in two independent passes
with **byte-identical** logs (SHA-256
`E1AA0418B539C187F1B025612AA62CDC7DE0FAEF34C1132EC99F5BD53A0DD82C`).

The calibration of the first passes found five things, and every one of them
was either a **gap in the tests** or a **fault in the mutation** — never a
dead rule:

1. **The noise policy's pruning (M02)** — the first mutation weakened the
   condition in the wrong direction: it made the *early return* rarer, which
   leaves the pruning **on**. The mutation now forces the early return, which
   turns the pruning off — and the test catches it.
2. **The deny list's early return (M04)** — removing
   `if len(pol.Deny) == 0 { return out }` changes **nothing**: with an empty
   deny list the function returns the same set either way. It was an
   **equivalent** mutation, not a surviving rule; the mutation now breaks the
   deny check itself.
3. **The deferred selector (M11)** — the first test used an intent that
   happened to match one tool, so both selectors returned the same single
   tool and the mutation was invisible. With an intent that matches **nothing**,
   the deferred selector offers the pinned search tool alone and the
   conservative one offers every schema — the difference the small-schema
   mode exists for.
4. **The discovery bound (M45) and the auto-budget flag (M47)** — these were
   pointed at a test that could not see them. The discovery bound is visible
   in the **tool surface** (the search tool appears), and the auto-budget flag
   in the **derived context budget**; both now have a test that asks.
5. **Eight compile errors**, each the mutation's fault: a removed condition
   left its binding unused (`policy`, `denied`, `allow`, `m`, `maxTok`), and
   two removed values left their **import** unused (`governor`, `toolapi`,
   `strings`). The two rules that could not survive that — the cost function
   and the rescue marker — are now **package-level values** in the module
   (`costFn`, `rescueMarkers`), which is both more readable and a fact the
   mutation can break.

One finding belongs to the **harness**, not to the code, and it is the same
one W3.76f found: a `-run` pattern that matches **no test** exits **0**. The
harness treats "no tests to run" as **BROKEN** — the mutation's fault — so a
wrong test name can never masquerade as a surviving rule.

## 7. The ratchet

`kernel/runtime/loopcfg_ratchet_test.go` holds the move in place:

- the daemon's three loop-configuration files are **gone**, and the module's
  six files are there;
- the daemon holds **no** copy of the rule — none of `filterTools`,
  `applyAgentToolPolicy`, `applyAgentNoisePolicyToPromptTools`,
  `withToolSearch`, `makeElidedSummarizer` or `buildLoopConfig`;
- the daemon **keeps** its doors: the projection, the dependency wiring and
  `BuildLoopConfig`;
- the daemon holds a **pointer** to the module's service, and the module's
  files **import the daemon never**.

## 8. What is left

The run engine's remaining families:

- **the loop** (`kernel/agent`, **2,629** lines across 20 files) — the biggest,
  and the most entangled with the contract layer (123 production files import
  `kernel/contract/llm`). It is the last family of the run engine.
- then the **W3 exit**: `kernel/runtime` and `kernel/controlplane` dissolved,
  the daemon reduced to a host.

And after that: the **W2** domains (seven PRs), **W4** (five), **W5** (four).

## 9. The numbers that did NOT move

- the daemon's **public verbs**: 238 → 238.
- the **contract**: +0 rows. The loop configuration needed no new contract
  type: the tool surface, the policy verdict, the artifact putter and the
  catalog rows all already existed.
- the **application layer**: +3 lines — one call site and the comment that
  says why it asks the module now.

The honest summary of the slice: the daemon lost **292** lines and the module
gained **519**. The project is not shrinking; it is **changing shape** — the
rules are moving to the module that owns the thing they describe, and the
daemon is keeping only the doors and the state.
