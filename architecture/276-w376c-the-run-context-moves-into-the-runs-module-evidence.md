# W3.76c — the run context moves into the runs module

## What the slice is

The **eleventh** slice of the runtime's dissolution and the **third** slice of
the **run engine** programme (W3.76). W3.76a took the durable **tickets**;
W3.76b took the **steering**; this one takes the **run context** — the
vocabulary a run carries.

The family was split across **six** files of the daemon:

- `kernel/runtime/runctx.go` (**66** non-blank lines) held the **keys** (the
  `ctxKey` enum), the run's own rows (`agentToolPolicy`, `agentNoisePolicy` and
  its state) and the `WakeContext` alias.
- `kernel/runtime/runctx_agent.go` (**143**) held the **profile binding**:
  `WithAgentProfile`, `WithAgentIdent` and the noise-policy resolution.
- `kernel/runtime/runctx_basic.go` (**160**) held the **dispatch knobs**:
  `WithTrustCeiling`, `WithImages`, `WithJSONMode`, `WithModel`, `WithSystem`,
  `WithRunTimeout`, `WithMaxCost`, the root correlation and their getters.
- `kernel/runtime/runctx_security.go` (**99**) held the **grants**:
  `WithAutoApproveCapabilities` and its merge, `WithTrustedObservations`, and
  the prompt-injection **mode** (the platform's rule, W3.74).
- `kernel/runtime/runctx_wake.go` (**94**) held the **wake half**:
  `WithWakeContext`, the system-agent / tool-policy / retry-policy getters,
  `AgentConfigOverrides`, `WithModelChain`, `WithTools` and the actor /
  correlation readers.
- `kernel/runtime/prompt.go` (**88**) held the **profile's system prompt**:
  `AgentProfileSystem` and the task renderer.

## What the slice found

The run context was the **cheapest** of the four measured run-engine families,
and the measurement said so before anything was moved: **562** non-blank lines
across **5** files, **zero** `*Kernel` methods, **zero** stored state, and the
`ctxKey` enum's **22** keys living in a single file. A family with no store, no
rule and no handle is a **vocabulary** — and a vocabulary belongs to the module
whose thing it describes.

The measurement also found the three parts of it that were already elsewhere
and had to **stay** there:

- the **wake provenance** (`agentapi.WakeContext`) and the **tool allowlist**
  (`toolapi`) are **contract rows**: a host states them without naming a
  module, and the module's setters are thin delegates over them;
- the **prompt-injection mode** is the **platform's** rule (W3.74) — the guard
  that reads it lives in `kernel/platform/policy/guards`, so the daemon keeps
  the name (`kernel/runtime/promptinjection.go`);
- the **resume handshake** (`WithResumeOwned`, `WithResumeSeed`) is the
  **daemon's** boot protocol, not the run's vocabulary — it stays in
  `kernel/runtime/resume.go` with its own key type.

## What changed

### The module owns the vocabulary

```
kernel/modules/runs/runctx/keys.go      (89)   the keys, the run's own rows
kernel/modules/runs/runctx/agent.go     (147)  the profile binding
kernel/modules/runs/runctx/basic.go     (182)  the dispatch knobs
kernel/modules/runs/runctx/wake.go      (102)  the wake half
kernel/modules/runs/runctx/security.go  (65)   the grants
kernel/modules/runs/runctx/profile.go   (87)   the profile's system prompt
kernel/modules/runs/runctx/noise.go     (20)   the severity ranking
```

The keys, the setters, the getters and the run's own types moved together —
that is the whole point of a vocabulary: one spelling, one place.

### The daemon keeps the doors — and nothing else

There is **no facade**: the daemon holds no package-level alias of a run-context
setter, so a host names the module directly (`runctx.WithModel(...)`). What
the daemon keeps is:

- the **Kernel accessors** (`kernel/runtime/accessors_ctx.go`), which are the
  doors a host that holds a *kernel handle* uses: `k.SystemAgentFromCtx`,
  `k.ActorFromCtx`, `k.WithActorCorrelation`, `k.WithAgentProfile`,
  `k.WithMaxCost` and the rest;
- its **own** resume handshake (`kernel/runtime/resume.go`), with its own
  `resumeCtxKey` type — the module has no business knowing it;
- the **prompt-injection mode's name** (`kernel/runtime/promptinjection.go`),
  because the rule behind it is the platform's.

### One spelling, no new edge

The hosts name the module, and the module names the contract. The daemon's
`kernel/runtime` and the module's `kernel/modules/runs/runctx` are **the same
module** (`runs`) as far as the architecture is concerned, so the move creates
**no** new import edge — and it closes one: `kernel/modules/agents/selfrepair`
no longer reaches into the daemon for the profile binding, because the daemon
exposes it as a **door** on the handle it already holds.

## The rule

**The module owns the vocabulary; the daemon keeps the doors and its own
handshake.** W3.76a's rule was *the host keeps the doors; the module keeps the
rows and the rules that judge them*; W3.76b's was *the module owns the
operations; the host owns the doors*. This slice adds the fourth: when a
family is a **vocabulary** — no store, no rule, no handle — the whole thing
moves, and the host does not keep a copy under a different name.

The exception is what the host owns by definition: the daemon's own boot
protocol (the resume handshake) and the name of a rule that is not the
daemon's (the prompt-injection mode).

## What the slice cost

- **Six** daemon files deleted (`runctx.go`, `runctx_agent.go`,
  `runctx_basic.go`, `runctx_security.go`, `runctx_wake.go`, `prompt.go`), one
  daemon test file deleted with the function it tested
  (`agentnoise_internal_test.go`), **seven** module files written and **two**
  daemon files written (`promptinjection.go`, `runctx_ratchet_test.go`).
- **52 files** now import the module (**26** production, **26** test) — the
  vocabulary is read from one place, and the call sites had to be repointed.
- **The daemon's own files**: **49** touched (7 deleted, 2 new, 40 modified).
- The application layer (**5** files), the composition root (**8** files in
  `cmd/agezt`), the control plane's test (**1**) and the scheduler's test
  (**1**) follow the vocabulary.
- **No new contract row**: the wake provenance uses the existing
  `agentapi.WakeContext`, the tool allowlist the existing `toolapi`.
- **No new face**: the kernel's verbs keep their names and their meanings.

## The tests

**39 new permanent tests** in the module and **8 new** in the daemon, on top of
the **7** that moved with the code they test:

- `kernel/modules/runs/runctx/noise_test.go` — **7 tests**: the four noise
  policies that moved from the daemon, plus the new ones (a profile with no
  noise policy gets the **empty** policy, not an invented default; the
  severity ranking orders the severities and ranks an unknown one lowest; the
  resolved policy rides on the context).
- `kernel/modules/runs/runctx/agent_test.go` — **11 tests**: the profile
  binding — the trust ceiling, the config overrides (a copy), the retry policy
  (a copy of the caller's own slice), the run's identity and daily ceiling,
  the tool policy, the model and its fallbacks, the soul as the system prompt
  (with its **source**), the system-agent flag, the unique append, and the
  rule that the profile does **not** apply the cost ceiling.
- `kernel/modules/runs/runctx/basic_test.go` — **10 tests**: the model and
  system settings (and their **source**), the empty model as a no-op, the
  images, the JSON mode, the two budgets, the trust ceiling that **never
  loosens** and the "no clamp" that records **no** ceiling, the delegation
  root, and the map copy.
- `kernel/modules/runs/runctx/wake_test.go` — **5 tests**: the wake
  provenance, the tool allowlist (an empty one is "no tools"), the actor and
  the correlation, the model chain, and the retry policy's absence.
- `kernel/modules/runs/runctx/profile_test.go` — **7 tests**: the soul, the
  instructions, the task list split by scope (with the finished tasks
  dropped), the all-done list that renders nothing, the **cap** at twenty
  tasks, and the task description.
- `kernel/runtime/runctx_ratchet_test.go` — **7 tests**: the **ratchet** (the
  daemon's six files are gone and the module's are there; the daemon declares
  **no** run-context key; the daemon's own resume keys stay), the direction of
  the move (the daemon reads the run's dispatch **through** the module), the
  doors that stay, the module's own rows, and the daemon's **profile door**
  (the one the mutation run asked for).

The daemon's `agentprofile_internal_test.go` keeps the two tests that are the
daemon's — the config-override **parsers** and the issue reporting — and gave
the other three to the module. The **eighth** new daemon test is
`TestTheResumeHandshakeIsTheDaemons` (in `resume_internal_test.go`): it pins
the daemon's **own** half of the resume protocol — the owned marker and the
seed — and it is there because the mutation run asked for it (the owned marker
that never travelled was invisible to every test that used the handshake from
the outside).

## The mutations

**54 mutations, 54 caught**, in two independent passes with **byte-identical**
logs.

SHA-256: `58E51D9E1997145F81A30FA5AD167EA9B8A385058C649167ADF7651885E4CA78`

The calibration recorded **seven** compile errors, and every one of them was
the mutation's fault — the mutation left the code in a state Go refuses:

- the trust ceiling that never applied left `lvl` declared and not used (it now
  applies the **tightest** ceiling instead, so the value is still wrong but the
  code compiles);
- the child that loosened the parent's cap left `existing` and `ok` unused (it
  now keeps the **looser** one, which is the bug the mutation was after);
- the finished task that stayed on the list became `status == "done" &&
  status == "retired"`, which `go vet` refuses as a *suspect and* (it now
  empties the status instead);
- the uncapped task list left `i` unused (it now caps at two hundred);
- the task description that never rendered left `desc` unused (it now renders
  the description only when it is **empty**);
- the overrides door that read nothing returned a bare `nil` and left `ctx`
  unused (it now reads a **fresh** context).

What the 54 mutations cover:

- **M01**–**M15** the profile binding: the system-agent flag, the noise
  policy on the context, the memory-write floor, the notify interval, the two
  notify floors, the dropped fallbacks, the dropped denylist, the missing
  identity, the copied retry policy, the trust ceiling, the model chain, the
  model setting's source, the system setting's source, and the unique append.
- **M16**–**M24** the dispatch knobs: the erased cap, the loosened cap, the
  empty model that overwrites the run, the images, the JSON mode, the run
  timeout, the cost cap, the delegation root and the map copy.
- **M25**–**M32** the wake half: the actor, the correlation, the model chain,
  the tool allowlist, the wake provenance, the absent retry policy, and the two
  getters that read **another** key.
- **M33**–**M34** the grants: the auto-approve grant and the trust grant.
- **M35**–**M40** the profile's system prompt: the soul, the instructions, the
  finished task, the cap, the cycle tasks and the description.
- **M41** the severity ranking.
- **M42**–**M45** the daemon's resume half: the ticket's source, the ticket's
  set flag, the seed and the owned marker.
- **M46**–**M49** the daemon's policy doors: the memory-write denial, the
  notify floor, the tool allowlist and the tool denylist.
- **M50**–**M54** the daemon's doors: the actor door, the actor reader, the
  profile door, the cost door and the overrides door.

### What the mutation run found

Five of the 54 mutations **survived** the first pass, and every one of them was
a **gap in the tests**, not a dead rule:

- the **retry policy that was not copied** was invisible because the *getter*
  copies as well — so the test now mutates the caller's own profile after the
  binding and reads the run's copy back;
- the **"no clamp" that recorded a ceiling** was invisible because the
  inherited cap won either way — so the test now asks the ceiling to do
  nothing at all and requires that **no** ceiling is recorded;
- the **empty model that overwrote the run** was invisible because an empty
  value reads as no value — so the test now binds a profile's model first and
  asks the empty override not to erase it;
- the **tool allowlist that was not applied** was invisible because the
  profile's allowlist happened to name every registered tool — so the daemon's
  test now registers a tool the profile does **not** allow;
- the **profile door that bound nothing** was invisible because the test read
  the overrides through the module rather than through the daemon's door — so
  there is now a test that binds the profile through the door and reads it
  back.

## The measurement that moved

| | before | after |
|---|---|---|
| the daemon's copies of the run context | **6 files / 650 lines** | **0 / 0** |
| `kernel/runtime` (upper level, lines) | **8,391** | **7,809** |
| `kernel/runtime` (upper level, files) | **69** | **64** |
| `kernel/modules` (recursive, lines) | **26,337** | **27,038** |
| `kernel/modules/runs` (recursive, lines) | **1,071** | **1,763** |
| the module's lines (the family) | **0** | **692** |
| the kernel's verbs | **242** | **244** |
| the contract's new rows | — | **0** |
| the files that name the module | **0** | **52** |

The daemon's copies of the run context went from **six files** to **zero**, and
a **ratchet test** holds them there: it asserts that the six files are gone,
that the module's seven are there, and that the daemon declares **no**
run-context key.

The daemon's verb count went from **242** to **244**: this slice gave the
daemon **two** doors (`k.WithAgentProfile` and `k.WithMaxCost`) so that a host
holding a kernel handle — `kernel/modules/agents/selfrepair` — can bind a run
without reaching into another module and without the daemon keeping a
package-level alias of the module's setters.

## The measurement that did NOT move

The **family's** own line count went **up**: **650 → 692** non-blank lines,
and the daemon's **run-dispatch doors** went **up** with it: **645 → 702**.

That is the same shape W3.74, W3.75 and W3.76b explained — **moving a rule
moves the CONDITIONS, not the lines** — with this slice's own addition: the
module's lines are not only the moved lines, they are the moved lines **plus
the package's own documentation** (the `keys.go` doc comment explains what the
run context is and why it lives in the runs module) **plus the exported
surface** the daemon's unexported names became (every `xFromCtx` is now
`XFromCtx`, and each has a doc comment of its own).

The daemon's **total** went down — **8,391 → 7,809** — and that is the number
that measures the move: the daemon no longer holds a copy of a vocabulary that
another module now holds, and it keeps only the doors and its own handshake.

## What is left

The runs module now holds three families: the **durable tickets** (W3.76a),
the **steering** (W3.76b) and the **run context** (this slice).

Three of the run engine's measured families are still in the daemon: the
**delegation** (**881** lines) is the most entangled — nine `*Kernel` methods,
three bus publications, a journal read — and it needs a real port design
before it can move; the **prompt** (**578** lines) reads memory, world and
skills through contract ports and owns no state, and the prompt's own build
(`kernel/runtime/prompt*.go`) is still the daemon's; the **loop config**
(**99** lines) is the smallest and is a rule the loop reads. The **loop**
itself (`kernel/agent`, **2,629** lines across 20 files) is the biggest and
the most entangled with the contract layer: **123** production files import
`kernel/contract/llm` (48 of them under `plugins/providers`) and **14** import
`kernel/agent` itself.
