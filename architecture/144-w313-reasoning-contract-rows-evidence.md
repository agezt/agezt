# W3.13 — the reasoning rows move to the contract layer: the tools stop reaching into the kernel

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 step 8 — the agents
module's remaining item, and step 10's follow-through: the plugin layer's
"reach into the kernel" edges.

**Scope:** the rows the reasoning module produces (a consensus, a transcript, a
cited report) move down to the contract layer, so the three agent-facing tools
that expose them stop importing `kernel/runtime` **for a type**.

## What moved

| Before | After | What it is |
|---|---|---|
| `kernel/modules/reasoning/api` (the row definitions) | `kernel/contract/reasoningapi` (new, L1) | the rows: `CouncilResult`, `ConductorConfig/Step/Exec/Result`, `ResearchOptions/Source/Claim/Report` (+ `CouncilMember`/`Opinion`, already in `contract/types`) |
| `kernel/modules/reasoning/api` | unchanged in shape | the module's **ports** (`Dependencies`), its **surface** (`Service`), its pure helpers, and **aliases** of the contract's rows |
| `kernel/runtime` | unchanged | its public names stay aliases of the module's rows, so the host's API does not move |
| `plugins/tools/{research,conductor,council}` | import `kernel/contract/reasoningapi` | they no longer import `kernel/runtime` at all |

The module, the host and the tools now speak **one** type: the contract's row.
The module's api re-exports it under its own names (so the host's aliases and
the module's surface are untouched), and the tools name the contract's row
directly.

## Why

Archcheck's plugin rule is strict: a plugin (L6) may reach the **contract**
and the **platform** layers, and nothing else. The council, the conductor and
the research tools each reached into `kernel/runtime` — and each reached it for
**one reason only**: to name the reasoning module's rows in their `Runner`
interfaces.

```go
// Runner is the slice of *runtime.Kernel the tool needs — an interface so the
// tool is testable without a kernel.
type Runner interface {
	Research(ctx context.Context, corr, question string, opts runtime.ResearchOptions) (runtime.ResearchReport, error)
}
```

The interface was already the right shape (a port!). Only its **types** were
on the wrong side of the layer line: a row the agent is shown is a contract
between the module that produces it and the tool that shows it, so it belongs
at the bottom, not in the kernel that happens to host the module.

## The ratchet went DOWN again

**277 packages, 120 import violations, 13 call sites** — from 123/13, because
three plugin-reach edges are gone:

- `plugin-reach plugins/tools/research -> kernel/runtime`
- `plugin-reach plugins/tools/conductor -> kernel/runtime`
- `plugin-reach plugins/tools/council -> kernel/runtime`

No allowlist entry was added. The tools' `Runner` interfaces are unchanged in
shape, and the daemon still satisfies them with the kernel — the wiring only
compiles because the contract's row and the module's row are ONE type, which
is now asserted at the seam.

## The proof

### Permanent tests

The three tools already had tests; what they lacked was an assertion that the
**whole** result reaches the agent. Three were added:

- `TestResearchTool_SurfacesTheWholeReport` — the claims and their verdicts,
  the verification flag, the cited-source count, the sub-questions, the notes
  and the source rows (with their citation ids and ranks). A mapping that
  quietly drops a field leaves the agent with a thinner report than the run
  paid for.
- `TestConductor_SurfacesTheWholeTranscript` — the answer, the verdict, the
  resolved role models, the round count, the plan and every recorded step
  (with its role, verdict and reason).
- `TestCouncil_SurfacesTheWholeDebate` — the consensus, the dissent, the panel
  that sat, the round count, every recorded opinion (with its seat, model and
  text), and the dated brief.

And the **identity assertion** (`kernel/modules/reasoning/api/identity_test.go`):
the module's row and the contract's row must be the *same* type. The
assertion is written as a **function type** on purpose — an assignment would
also accept a copy with the same fields, but a function type requires
*identity*. So if the module ever declares its own copy instead of aliasing the
contract's, the break is reported at the seam instead of at the far end of the
wiring.

### Mutation run

**9 mutations, 9 caught; 1 build error, recorded separately.**

| # | Mutation | Result |
|---|---|---|
| M01 | the research tool drops the verified claims | caught |
| M02 | the research tool drops the verification flag | caught |
| M03 | the research tool drops the cited-source count | caught |
| M04 | the research tool's source rows lose their rank | caught |
| M05 | the conductor tool drops the transcript steps | caught |
| M06 | the conductor tool drops the resolved role models | caught |
| M07 | the council tool drops the recorded opinions | caught |
| M08 | the council tool drops the panel | caught |
| M09 | the council tool drops the dated brief | caught |
| M10 | the module copies the contract row instead of aliasing it | **build error** — the identity assertion fires at the seam |

M10 is the honest record of the slice: the first run **let it through**. The
module's own tests passed, because the module and its tests speak the module's
names consistently — the divergence only shows up where the host satisfies the
tools' `Runner` interfaces, at the far end of the tree. The identity assertion
is what closed it, and it is the reason the slice is safe to merge: the type
identity the whole slice rests on is now measured where the seam is.

### Gates

The build, gofmt, vet, dead-code, archcheck (**277 packages, 120/13**),
structure (and its check), changelog-lint, docclaimscheck and staticcheck are
green, and so is the full test suite.

## What is deliberately left

- The **overseer tool** still holds the runtime kernel and reaches into the
  agents and board modules — three more plugin-reach edges, and the one that
  needs the app layer (the only layer that can see both the roster and the
  run controls). That is the next piece of the agents module, and it is a
  bigger slice than this one: the tool's 24 operations need typed app
  operations first.
- The other plugin-reach edges (the config tool, the schedule tool, the
  guardians, the built-in tools, the introspector) each need their own slice.
- The **dated** reports (`.project/PHASE-*`, the dated scans and audits, the
  earlier evidence) describe the tree as it was and are left alone.
