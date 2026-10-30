# W3.73 — the epistemic guard's rule moves into the platform

## What the slice is

The **sixth** slice of the runtime's dissolution, and the **first** one that
moves a **rule** rather than a family.

The epistemic guard lived in the daemon: `kernel/runtime/epistemic.go` (80
lines) and `kernel/runtime/epistemic_helpers.go` (245 lines) — **325 lines**
of rule. It read the journal's memory, weighed the failures it found against
their age, matched the call's conditions and decided whether a tool call needs
a human before it runs.

The rule is not a module's business. It does not own a store, it does not
publish anything, and it has no lifecycle of its own — it is **infrastructure**,
the same kind of thing the Edict engine and the intent model already are. The
target architecture says where infrastructure lives: `kernel/platform`. The
roadmap's candidate list had guessed `modules/knowledge`, and the slice shows
why that guess was wrong: **knowledge** is a *bounded context* (the memory
library, the world's graph, the objectives), and a guard that every tool call
asks is not one context's business — it is the platform's.

So the rule moved to `kernel/platform/policy/epistemic`, next to the other
policy infrastructure the kernel already asks (Edict, intent/regret, the
approval registry).

## What the slice found

The candidate was measured before it was chosen:

- the daemon's guard: **325 non-blank lines** in two files;
- the daemon's copies of the rule's tables: **1** (`defaultEffectConfidence`,
  read by the approval bundle);
- the kernel's verbs that mention it: **1** (`epistemicGate`);
- the daemon's own stores: **0** — the guard reads the journal and the tool
  map, and it owns neither.

**A rule that owns no store is the platform's.** That is the measurement, and
it is why the slice needed no new face, no new contract row and no host
wiring.

## What changed

### The platform keeps the rule

`kernel/platform/policy/epistemic/epistemic.go` (173 lines) is the guard: the
`Decision` (what it decided, why, and the signals it read), the `Ports` and
the `Gate` that decides over them.

`kernel/platform/policy/epistemic/helpers.go` (283 lines) is the rule's
support: the escalation policy, the confidence floors, the schema hashing, the
input-shape heuristics, the history matching and the decay math.

```go
// kernel/platform/policy/epistemic/epistemic.go
type Ports struct {
	Scan   func(visit func(*event.Event) error) error
	Now    func() time.Time
	Lookup func(name string) (toolapi.ToolDef, bool)
}

type Gate struct{ ports Ports }

func (g *Gate) Decide(toolName, cap string, input json.RawMessage, def toolapi.ToolDef, class string, confidence float64) Decision
```

The three ports are the **host's**:

- **Scan** walks the journal's policy decisions and tool results — the history
  the guard reasons over;
- **Now** is the clock the history's age is measured against;
- **Lookup** finds a tool's definition by name.

Every port is a way to **ASK**; none is a way to reach. And every port is
**optional**: a host that hands over no journal answers "nothing is known", and
the guard then treats the tool as novel — which is the **safe** direction,
because unknown means ask.

### The daemon keeps the doors and asks

`kernel/runtime/epistemic.go` (60 lines) is now the host's half:

```go
func (k *Kernel) epistemicGate(toolName string, cap edict.Capability, input json.RawMessage, def toolapi.ToolDef, bundle approvalBundle) epistemic.Decision {
	return k.epistemicGateFor().Decide(toolName, string(cap), input, def, bundle.EffectClass, bundle.Confidence)
}
```

The gate is a struct of three function values, so it is built **per call**
rather than held on the kernel: a held gate would need a lock for a value that
never changes. The daemon's `policyHook` reads `ep.Escalates()` instead of the
old `ep.escalates()` — the same question, asked of a type that now lives
somewhere else.

### The one row that had to be shared

The approval bundle's fallback confidence (`defaultEffectConfidence`) was the
daemon's copy of a table the guard also needed. Two copies of a table is two
tables: they drift, and the drift is invisible. The table is now the
platform's (`epistemic.DefaultEffectConfidence`), and the daemon asks for it.

## What the slice cost

**Nothing to wire.** No host had to learn a new seam, because the rule's ports
are the kernel's **own** facilities — the journal it already opens, the clock
it already has and the tool map it already holds. This is the third shape the
programme has measured, and it is the cheapest one:

- a move into an **existing** face costs **nothing** (W3.70: 0 sites);
- a move that invents a **new** face costs a wiring in every host that has one
  (W3.68: 11, W3.69: 6, W3.71: 6, W3.72: 9);
- a move of a **rule that owns no store** into the **platform** costs
  **nothing** (W3.73: 0 sites) — the host supplies what it already has.

## The rule

The host keeps the **doors**; the platform keeps the **rule**.

A rule that owns no store belongs to the platform, and the host asks it over
ports it already has. The rule is held by the rule's own observer: the daemon's
decision is compared with the platform's decision over the same ports, and the
daemon's three ports are each observed on their own — the journal through a
**real** journal (not a stub), the tool map through a **real** tool map, and
the clock through the platform's own port.

## What did NOT move, and why

- **The journal stayed the host's.** It is the host's file and the host's
  store; the guard asks for a **walk** over it, and the walk is a port.
- **The tool map stayed the host's.** The host builds the tools; the guard
  asks for a definition by name.
- **The verdict's shape did not change.** The same thirteen fields reach
  `agent.PolicyVerdict` (`EpistemicAction/Reason/Signals/Confidence`,
  `FailureMatches`, `WeightedFailures`, `SchemaHash`, `InputShape`,
  `TemporalSensitive`, `NovelTool`) and the same rows reach the
  `policy.decision` journal entry — only the place that computes them moved.
- **The rules did not change.** A read-only tool escalates only on repeated
  weighted failures; an effectful tool escalates on a matching failure, on a
  confidence below its class's floor, or on a permissive schema behind a
  dynamic tool surface; and a failure's weight decays with its age (6 hours
  for a read, 7 days for an irreversible effect).

## The contract row

**None.** A contract row exists when two sides answer the same refusal and a
caller compares them with `errors.Is` (W3.71's `mcpapi.ErrNoRegistry`, W3.72's
`toolforgeapi.ErrNoStore`). The guard has no refusal to share: a host with no
journal is not an error, it is a host with no memory, and the guard's answer
to it is a decision, not a failure.

## The tests

**20 permanent tests**, in three places:

- `kernel/platform/policy/epistemic/epistemic_test.go` — **14 tests**: the
  guard allows an unknown tool (and marks it novel), escalates a low
  confidence, escalates a permissive schema behind a dynamic surface,
  escalates a matching failure, weighs a fresh failure above an old one, does
  **not** escalate a week-old failure (the weight decayed below the bar),
  ignores a failure under other conditions, escalates a read-only tool only on
  repeated failures, looks the tool up when the caller has no definition,
  allows a call with no definition (and says why), reads the temporal signal,
  knows nothing without a journal, keeps the class's own default confidence,
  and falls back to that default when the call declares none.
- `kernel/runtime/epistemic_internal_test.go` — **5 tests**: the daemon's
  decision **is** the platform's decision over the same ports (the delegation
  itself), the tool map resolves the definition through the lookup port, the
  journal is read through the scan port (a **real** journal, with a journaled
  failure matched), a kernel without a journal knows nothing (and says the
  tool is novel), and the default confidence row is the platform's.
- `kernel/runtime/epistemic_test.go` — **1 new test**: a tool that declares no
  confidence is judged under the **platform's** class default (0.95 for a
  read-only call), not under a copy of the table.

The three daemon-level tests that were already there — the journaled signals,
the escalation to approval, and the matched historical failure — were not
touched. They now exercise the platform's rule through the daemon, which is
the point: the daemon's behaviour did not change, so its tests did not have to.

## The mutations

**20 mutations, 20 caught**, in two independent passes with **byte-identical**
logs.

SHA-256: `B576A0B10DCF64C07C697BEB4E9C696B98098AB10E1B96FD93EEB3A9D8193AE9`

The first calibration reported **two BUILDERR** — the mutations that stopped
the failure weight from decaying and that gave the daemon a confidence of its
own both left an import unused, so Go refused to compile. A compile error is
not a caught mutation, it is a mutation that never ran: both now keep the
import alive (`math.Pow(1, …)` and `epistemic.DefaultEffectConfidence("unknown")`).

- **M01**, **M02**, **M03**, **M04**, **M05** the platform's escalation rule
  breaks — it never escalates, a read-only tool escalates on the first
  failure, an old failure escalates anyway, the confidence floor never fires,
  the floor drops.
- **M06**, **M07**, **M08**, **M09**, **M10**, **M11**, **M12**, **M13** the
  platform's signals break — the dynamic surface, the permissive schema, the
  novelty, the tool lookup, the no-definition escape, the confidence fallback,
  the default row and the temporal signal.
- **M14**, **M15** the platform's memory breaks — the input-shape condition is
  ignored, the failure's weight never decays.
- **M16**, **M17**, **M18** the host's ports break — the daemon stops asking
  the platform, the journal port finds nothing, the tool-map port finds
  nothing.
- **M19**, **M20** the daemon's use of the rule breaks — the escalation is
  never raised, the daemon answers the confidence with its own table.

## The measurement that moved

| | before | after |
|---|---|---|
| the daemon's lines of **rule** | **325** | **60** (port wiring) |
| the platform's lines of rule | **0** | **456** |
| the daemon's copies of the confidence table | **1** | **0** |
| the daemon's calls into the platform's rule | **0** | **2** |
| the kernel's verbs | **242** | **242** |
| the application layer's production files that changed | — | **0** |
| the test setups that had to be wired | — | **0** |
| the contract's new rows | — | **0** |
| `kernel/runtime` (lines) | **9,080** | **8,802** |
| `kernel/platform` (lines) | **2,557** | **3,013** |
| the daemon's `epistemic.go` (lines) | **80** | **60** |
| the daemon's `epistemic_helpers.go` (lines) | **245** | **0** (deleted) |
| the platform's `epistemic.go` (lines) | — | **173** |
| the platform's `helpers.go` (lines) | — | **283** |

The daemon's lines of rule fell from **325** to **60** — the 60 are the three
ports and the forwarder, and the rule they forward to is the platform's
**456**. The daemon's calls into the platform's rule are **2**: the decision
and the confidence row.

The **kernel's verb count did not move at all** (242 → 242): the guard was
never a public verb, and it never needed to become one. The **contract's rows
did not move either** (0 new): the guard has no refusal to share.

The lines are counted the way the previous slices counted them: **non-blank**
lines of the package's production files (`git ls-files`, upper level for the
runtime, recursive for the platform). The daemon shrank by **278** lines and
the platform grew by **456**.

### The candidate's guess was wrong, and the slice says why

The roadmap listed the epistemic gate as a candidate for `modules/knowledge`.
It is not one: the guard owns no library, it reads the journal and the tool
map (both the host's), and **every** tool call in the daemon asks it. A
bounded context is a context's business; the guard is the platform's. The
dissolution map in `architecture/20-target-architecture.md` already said
"platform/policy" for it — the candidate list was the thing that drifted, and
this slice is what settles it.

## What is left

The runtime's dissolution stands at **242 verbs** in **8,802 lines**.

The nearest families are the **MCP registry's fourteen** (now forwarders), the
**forge's eight** (now forwarders), the **board's sixteen**, the **workflow
host's twelve** and the **reasoning module's six**.

The next candidates the target architecture names: the **policy hook** (→
`platform/policy` — the same shape as this slice, and the guard's own
neighbour) and — last, because it is the largest and the most entangled — the
**run engine** (→ `modules/runs`).

The self-repair module's edge into the runtime is still in the archcheck
allowlist — it is the other program's work, and it waits there.
