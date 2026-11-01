# W3.76e — the prompt assembly moves into the runs module

## What the slice is

The **thirteenth** slice of the runtime's dissolution and the **fifth** slice of
the **run engine** programme (W3.76). W3.76a took the durable **tickets**,
W3.76b the **steering**, W3.76c the **run context**, W3.76d the **run
registry**; this one takes the **prompt** — the layer that decides what a run
knows before it starts.

The family was split across **six** daemon files (**714** non-blank production
lines in all):

- `kernel/runtime/prompt_run.go` (**189**) held the layered injection — the
  live identity, the operator profile, the taste exemplars, the recalled
  memory, the resolved world entities, the activated skills — and published
  the selection manifests.
- `kernel/runtime/prompt_context.go` (**109**) held the five injection
  helpers (`injectMemory`, `injectUserProfile`, `injectTaste`, `injectWorld`,
  `injectSkills`).
- `kernel/runtime/prompt_environment.go` (**178**) held the host preamble:
  OS/arch, the shell hint, the workspace, the date, the tool briefing, the
  capability briefing, the forge bias and the shell guidance.
- `kernel/runtime/prompt_env.go` (**15**) held the daemon's gate on that
  preamble.
- `kernel/runtime/context_selection.go` (**78**) held the selection publisher
  and the three candidate adapters — and the failure analysis, which stayed.
- `kernel/runtime/subagent_inject.go` (**145**) held the **child's** half of
  the same injection — the sub-agent's memory and skills — next to the
  delegation family's budget rule, which stayed.

## What the slice found

Two measurements settled it, and both were surprises.

**The family has ZERO public verbs.** Every one of its functions was
unexported: `buildRunPrompt`, `injectHostEnvironment`, `injectMemory`,
`injectTaste`, `injectWorld`, `injectSkills`, `injectUserProfile`,
`injectEnvironment`, `capabilityBriefing`, `forgeBias`, `shellGuidance`,
`firstSentence`, `publishContextSelection` and the three candidate adapters.
The daemon's two doors (`BuildRunPrompt`, `InjectHostEnvironment`) were the
ONLY way in. A family with no public surface is a family's **body**, and a
body belongs to whoever owns the thing it describes.

**The family's dependencies are eight wide, and three of them are all it
needs from a host.** The four stores are **contract rows the daemon already
holds** — the memory's manager, the taste library, the world model's graph and
the skill library's curator — so the module names the contract and never an
implementation. The rest is a **three-verb host port**: the live identity (who
the daemon is right now), the bus (the manifests are journaled) and the clock
(the candidates are scored against "now" and the preamble states today's
date). The configuration is a **snapshot**, because the daemon's `Config` is
read and never written.

That is the same shape W3.76b found in the steering family and W3.76d found in
the registry: **a family is port-shaped when its operations touch the host in a
small, enumerable way.**

## What changed

### The module owns the assembly

```
kernel/modules/runs/prompt/doc.go         (35)   the package contract
kernel/modules/runs/prompt/config.go      (78)   Config, Host, Deps
kernel/modules/runs/prompt/service.go    (332)   Service: Build, BuildEnvironment, BuildSubagent, PublishSelection
kernel/modules/runs/prompt/inject.go     (108)   the five injection helpers
kernel/modules/runs/prompt/environment.go (174)   the preamble, the briefing, the bias, the shell guidance
kernel/modules/runs/prompt/selection.go   (29)   the three candidate adapters
```

The `Service` holds the config snapshot, the host port and the four stores.
`Build` assembles a **root** run's prompt; `BuildSubagent` assembles a
**child's** (less of it: no operator profile, no taste exemplars — a sub-agent
is given the task, not the operator's persona — but the same memory and
skills, and its own manifests); `BuildEnvironment` states the machine.

### The daemon keeps the doors and the host port

```go
type promptHost struct{ k *Kernel }

func (h promptHost) System() string { return h.k.System() }
func (h promptHost) Publish(spec event.Spec) (*event.Event, error) { return h.k.bus.Publish(spec) }
func (h promptHost) Now() time.Time { return time.Now() }
```

The daemon's public verb count stays **238**: `BuildRunPrompt` and
`InjectHostEnvironment` are the same two doors it always had. The contract
grows by **0** rows and the application layer by **0** files.

### What stayed, and why

- the **prompt-injection mode** (`kernel/runtime/promptinjection.go`): it is
  the platform's rule (W3.74), and the daemon keeps the name;
- the **failure analysis** (`publishContextFailureAnalysis`): it reads the
  run's JOURNAL, which is the daemon's. The manifest it derives is published
  by the module — the module knows what a manifest is, the daemon knows what
  the run wrote;
- the **sub-agent spend walk** (`subAgentSpendMicrocents`): a budget rule
  belongs to the delegation family, not to the prompt. The file was renamed
  `subagent_spend.go` so its name says what it holds;
- the **daemon's file is named `prompt_doors.go`, not `prompt.go`**: the
  daemon's `prompt.go` was the run context's profile renderer, and it moved to
  the runs module in W3.76c. The ratchet in `runctx_ratchet_test.go` watches
  that name, and "the daemon has no prompt.go" stays a true statement.

## The measurements

Base `4de7b862` (the W3.76d merge), working tree after this slice:

| Measurement | Base | Now | Delta |
|---|---|---|---|
| `kernel/runtime` (recursive, production) | 70 files / 8,686 | 67 files / 8,161 | −3 files / −525 lines |
| `kernel/runtime` (upper level, production) | 64 files / 7,794 | 61 files / 7,269 | −3 files / −525 lines |
| `kernel/modules/runs` | 20 files / 2,086 | 26 files / 2,842 | +6 files / +756 lines |
| the family's own line count | 714 (6 files) | 756 (6 files) | +42 |
| the daemon's public verbs (`func (k *Kernel) [A-Z]`) | 238 | **238** | **0** |
| `kernel/runtime`'s test functions | 392 | 388 | −4 (−8 moved out, +4 ratchet) |
| the module's test functions | — | 23 | 8 moved + 15 new |
| the contract's new rows | 0 | 0 | 0 |
| the application layer's changed production files | 0 | 0 | 0 |

The **+42** in the family's own count is the slice's real cost: the package
documentation, the exported surface the daemon's unexported names became, and
the config snapshot and host port that the module needs and the daemon did
not have to name.

## The tests

- `kernel/modules/runs/prompt/service_test.go` — **15** test functions over a
  fake host and four **embedded** contract fakes (a struct embedding the
  interface it stands in for, overriding only the verbs the test uses): the
  live identity, the run's own identity, the system-agent rule, the operator
  profile, the taste injection and its event, the memory injection and its
  manifest, the world injection and its manifest, the skill injection and its
  manifest, the **explicit** activation directive, the nil-port rule, the
  environment gate, the empty-manifest rule, the manifest on the wire, the
  child's cleaned intent and the child's system-agent exit;
- `kernel/modules/runs/prompt/{environment,profile,taste}_internal_test.go` —
  **8** test functions, **moved** from the daemon with the code they test;
- `kernel/runtime/prompt_ratchet_test.go` — **4** test functions: the family's
  files are gone and the module carries them, the daemon holds no body of the
  assembly, the daemon keeps the doors, the kernel holds the module's
  service.

## The mutation run

**40 mutations, 40 caught, 0 survived, 0 broken.** Two byte-identical runs,
SHA-256 recorded in the roadmap and NEXT.md entries.

The calibration recorded three things worth keeping, and **two of them were
gaps in the new tests** rather than dead rules:

1. **A manifest that says "something was chosen" without saying WHAT is not a
   selection.** The three candidate adapters could be broken to return nothing
   and every test still passed: the manifest was published, the event arrived,
   and only the candidate list was empty. The tests now read the manifest's
   `Chosen` list and name the record, the entity and the skill in it.
2. **"The system agent is not injected into" is not "the system agent has no
   stores".** The first version of that test ran with the taste library filled
   and everything else switched off, so the mutations that removed the
   `!systemAgent` guard from the profile, memory, world and skill paths all
   survived. The test now switches every injection ON and fills every store.
3. **The child's system-agent exit was the same story**: with the stores empty
   the child had nothing to be injected with, so removing the guard changed
   nothing. The child test now fills the memory and the skills.

And one that is about the mutation itself: replacing `h.Skill.ID` with a
literal leaves the loop variable `h` unused, which is a **compile error**, not
a kill. The mutation keeps the variable alive (`"wrong-"+h.Skill.ID`) so it
compiles and still breaks the behaviour.

## What is left

The runs module now holds everything the target architecture gives it — the
run registry, the resume tickets, the steering/interventions — plus the run
context and the **prompt assembly**. What is left in the daemon's run engine
is the **delegation** surface (939 lines, 13 verbs, which touches the bus,
the journal, the roster and the spend limits and therefore needs a port
design before it moves), the **loop config** (99 lines) and the **loop** itself
(`kernel/agent`, 2,629 lines across 20 files, the most contract-bound family
of all). The prompt family was the cheapest of the four *by the surface*, but
not by the ports: eight dependencies for 520 lines. The delegation family
looks four times bigger, and it will need a port design of its own.

Evidence for the neighbouring slices:
`architecture/276-w376c-the-run-context-moves-into-the-runs-module-evidence.md`,
`architecture/277-w376d-the-run-registry-moves-into-the-runs-module-evidence.md`.
