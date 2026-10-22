# W3.1 the reasoning module

The Council of Elders, the Conductor and the deep-research harness were
**three features of the composition root**: they were `*runtime.Kernel` methods
(`k.Council`, `k.Conduct`, `k.Research`) spread over ten files in
`kernel/runtime`, and they reached into the kernel's own fields for everything
they needed — the provider, the default panel, the web-grounding flag, the
aux-completion funnel, the tool registry, the event bus and the sandbox.

The migration roadmap's W3 step ("dissolve `runtime.Kernel` into modules")
lists `modules/reasoning` as its first entry. With this slice the three
features **are** a module.

## What moved

The ten files moved to `kernel/modules/reasoning`, with the `Kernel` receiver
replaced by a `Service` over an explicit set of ports.

| the feature | before | after |
|---|---|---|
| the Council | `kernel/runtime/council.go`, `council_helpers.go` | `kernel/modules/reasoning/council.go` |
| the Conductor | `kernel/runtime/conductor.go`, `conductor_helpers.go`, `conductor_steps.go` | `kernel/modules/reasoning/conductor.go`, `conductor_steps.go` |
| the research harness | `kernel/runtime/research.go`, `research_helpers.go`, `research_prompts.go`, `research_run.go`, `research_verify.go` | `kernel/modules/reasoning/research.go`, `research_helpers.go`, `research_prompts.go`, `research_verify.go` |

The module is **stateless**: it holds no provider, no tool registry, no bus and
no store. Everything it needs arrives through `api.Dependencies`:

| the kernel's field | the module's port |
|---|---|
| `k.completeAux` (the aux-completion funnel) | `Complete` |
| `k.cfg.Provider != nil` | `HasProvider` |
| `k.RunTool` (the governed tool invoker) | `RunTool` |
| `k.LookupTool` | `ToolAvailable` |
| `k.bus.Publish` | `Publish` |
| `k.cfg.CouncilMembers` / `CouncilDefaultMembers` | `Members` |
| `k.cfg.CouncilWebSearch` | `WebSearch` |
| `k.conductorExec` | `CodeExecAvailable` + `RunCode` |

## The module boundary

A module may import another module's **api** package and nothing else
(archcheck's principle P5), and `kernel/runtime` is a module (`runs`) just like
`kernel/modules/reasoning` is. So the two sides meet in exactly one place:

- **`kernel/modules/reasoning/api`** (new) — the module's public vocabulary:
  the rows and results, the pure helpers both sides share
  (`NormalizeMembers`, `OrPlaceholder`, `Clip`), the module's ports
  (`Dependencies`) and its surface (`Service`). The module's implementation
  cannot be imported from the kernel, and the kernel's implementation cannot be
  imported from the module — each side sees only the other's api package.
- **`kernel/runtime/reasoning.go`** (new) — the runtime's face of the module:
  the public names stay (`runtime.CouncilResult`, `runtime.Opinion`,
  `runtime.ConductorConfig`, `runtime.ResearchReport`, …) as **aliases** of the
  module's rows, the three calls delegate to the injected service, and
  `ReasoningDependencies()` builds the host's ports.
- **`cmd/agezt/main_toolinvoker.go`** — the composition root. `openAppKernel`
  builds the service over the kernel's own ports and injects it with
  `SetReasoning`. The primary kernel and every tenant kernel go through the
  same function, so both are wired.

The app layer names the module's vocabulary where it declares its own
(`kernel/app/council`, `kernel/app/research` now use
`kernel/modules/reasoning/api`); the **control plane keeps the kernel's public
names**, because an adapter may not import a module at all (archcheck's
adapter-bypass rule) — it reaches the module through the kernel's API, which is
the host's public surface.

## What did not change

**No behaviour changed.** The same error texts ("council: question required",
"council: no members available (no keyed providers?)", "conductor: task
required", "conductor: no models available (set thinker/worker/verifier, or
configure keyed providers)", "research: question required", "research: no
provider configured"), the same event subjects and kinds (`council.<corr>`,
`conductor.<corr>`, `council.started/brief/convened/opinion/consensus`,
`conductor.started/step/done`), the same token budgets (per-opinion 900,
consensus 1200, thinker 900 / worker 1600 / verifier 600 / plan 700), the same
audit arcs and the same wire JSON.

The **audit arcs are the strongest evidence that the move was honest**: the
council's web grounding still runs through the shared `RunTool`, so it keeps
its policy decision, its approval request, its unique search call id and its
terminal result; the Conductor's verifier still runs its code through the
host's invocation-local adapter, so a refusal still reports `Ran=false`, a
false live execution flag and no invocation event. Those arcs are asserted by
the pre-existing host-side tests, unchanged.

The only new string is the host's guard for a kernel whose composition root
forgot the wiring: "runtime: reasoning module not wired".

## Runnable evidence

Permanent tests cover:

- the **module's own vocabulary** (`kernel/modules/reasoning/api/api_test.go`):
  the panel rule (trim, drop the modelless seats, name the blank ones, dedupe),
  the empty-panel list rule, the answer placeholder and the rune-safe clip;
- the **module's own parsers** (moved with the code): the chair's
  consensus/dissent split, the verifier's verdict, the role briefs, the
  runnable-code extraction, the research planner's sub-questions, the search
  hits, the cited sources, the claims, the confidence and the adversarial
  verdict;
- the **module's guards** (`kernel/modules/reasoning/service_internal_test.go`):
  the council's empty question and empty panel, the Conductor's empty task and
  missing models, the research harness's missing provider and its degradation
  when no tool is wired, and the default panel arriving over the port;
- the **host's wiring** (`kernel/runtime/reasoning_test.go` and the two
  `reasoning_wiring*_test.go` helpers): a kernel without the module says so,
  and the same kernel, once the module is injected, answers through it;
- the **host-side behaviour** (the pre-existing runtime tests, unchanged apart
  from the injection): the deliberation and the chair's synthesis, the dated
  grounding and the shared web brief, the approval arc, the code-execution
  audit arcs, the role routing and the plan briefs;
- the **application's wiring** (the pre-existing app and control-plane tests):
  the legacy wire shapes of `council_ask`, `conductor_ask` and `research_ask`
  are unchanged.

The mutations fail tests: **23 mutations, 23 caught** — five in the api package
(the dedupe, the trim, the modelless seat, the clip, the placeholder), fourteen
in the module (the council's three guards, its default round, its panel port,
the Conductor's task guard, the js alias, the verdict, the role briefs, the
research provider guard, the plan's question, the confidence clamp, the verdict
classification, the uncited claim, the out-of-range citation, the completion's
task type) and four in the host (the injection, the panel port, the refused run
that claimed it ran, the completion's task type).

The mutation run found a **test gap** and it is fixed and named: nothing
asserted the council's **default** deliberation round, so the mutation that
dropped it to zero escaped. The module's test now asserts both the default
round count and the resulting opinion count, and the mutation is caught on the
repeat.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (273 packages — the two new ones — with the ratchet
unchanged at 129 allowlisted import violations and 13 allowlisted call sites),
`tools/structure-md` (the two new packages are placed), `tools/changelog-lint`,
`tools/docclaimscheck` and staticcheck. The affected suites — the module, the
api package, the runtime, the control plane, the two application packages and
the three tools — pass.

## Not done here

**The kernel keeps its public names.** `runtime.CouncilResult`,
`runtime.Opinion`, `runtime.ConductorConfig`, `runtime.ResearchReport` and
their kin stay, as aliases of the module's rows. They are not a second
declaration — an alias is the same type — but they are a second *name*. They go
when the rest of `kernel/runtime` goes (W3.11, `modules/runs`).

**The next modules are untouched.** `modules/knowledge` (memory, world, taste,
profile distill — with the profile-facet supersede bug to fix in its rewrite
PR), `modules/skills`, `modules/artifacts` (artifact + datalake, with the
journal `raw_ref` GC), `modules/board`, `modules/work`, `modules/workflows`,
`modules/agents`, `modules/extensions`, `modules/providers` and `modules/runs`.

**The application layer still reaches the kernel for the rest.** Only the
reasoning vocabulary moved; the other app packages still speak to
`kernel/runtime`.
