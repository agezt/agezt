# W3.76v — the reasoning capability's lifecycle moves into the reasoning module

**Slice:** W3.76v (the thirtieth of the runtime's dissolution)
**Base:** `7ccb24cf` (the W3.76u merge)
**Date:** 2026-11-03

---

## 1. The family

The **reasoning capability** is the family that thinks: the **Council of Elders**
(a multi-model panel that deliberates and reconciles), the **Conductor** (a
verify-driven thinker→worker→verifier loop) and the **deep-research harness**
(plan → gather → synthesize → adversarially verify).

The capability is the **reasoning module's** — `kernel/modules/reasoning` — and
the module's `*Service` has answered the contract's `reasoningapi.Runner` port
since W3.1 (`var _ reasoningapi.Runner = (*Service)(nil)`).

What the daemon still held was **three forwarders** over that module's own
operations: `Council`, `Conduct` and `Research`. Each one did the same four
lines: take the service out of the daemon, refuse when there was none, call it,
return its answer.

## 2. The port the contract already had

The **contract** named this family in **four spellings**:

- `reasoningapi.CouncilRunner` — one verb: convene the panel;
- `reasoningapi.ConductorRunner` — one verb: run the loop;
- `reasoningapi.ResearchRunner` — one verb: run the harness;
- `reasoningapi.Runner` — the three together, and the port the registry hands
  to the three agent-facing tools.

That is **one vocabulary in four widths** — the shape W3.76u settled for the
workflow family: the narrow ports are one-verb views of the same vocabulary,
and the wide port is their union. The module's service answers all four
without an adapter, because its own methods already carry the contract's
names.

The daemon's three forwarders were a **fifth** copy, and they are **gone**.

## 3. What moved

- **the module** did not change: its service already answered in the contract's
  names. The slice removed the second copy of the vocabulary, not the
  vocabulary;
- **the contract** gained the refusal row `reasoningapi.ErrNoService` — the
  missing capability is one fact, and the row is the contract's so that every
  layer that feels the absence says it the same way (and a caller compares
  with `errors.Is`);
- **the daemon** keeps three things and no more: the module's **WIRING**
  (`ReasoningDependencies` — the host's own governed paths: its attributable
  completion funnel, its governed tool invoker, its bus, its default panel, its
  sandboxed code runner), the **SETTER** that injects the module
  (`SetReasoning`, called once by the composition root) and the module's
  **FACE** (`ReasoningService`). The daemon's private `reasoningService`
  helper stayed — the public face is the one that hands the module over;
- **the application** binds the module's face: `kernel/app/council`'s
  `EnginesFromKernel` asks `k.ReasoningService()` and drives `Council` and
  `Conduct` on it; `kernel/app/research`'s `ResearchFromKernel` asks the same
  face for `Research`. A daemon that holds no module holds **no** face — and
  the engines then **refuse** with the contract's row
  (`reasoningapi.ErrNoService`) instead of calling into nothing. The council's
  engines also became **nil-safe**: `EnginesFromKernel(nil)` used to panic on
  `k.NewCorrelation`, and now refuses like every other missing daemon;
- **the plugin** binds the module's face too: `reasoningPort(k)` answers
  `k.ReasoningService()`, and a daemon with no module hands over a **nil**
  port — the three tools then answer "unavailable" themselves rather than
  binding a runner that is not there.

The daemon's own **read** over the panel (`CouncilDefaultMembers`) stayed: a
read over the host's own configuration is the host's, exactly as W3.76n and
W3.76u left the roster's and the workflow library's reads in place.

## 4. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 166 | **164** |
| the daemon's production lines | 5,813 | **5,796** |
| the daemon's production files | 54 | **54** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the reasoning module's production lines | 1,545 | **1,545** |
| the reasoning contract's production lines | 169 | **185** |
| the council application's production lines | 426 | **458** |
| the research application's production lines | 173 | **185** |

**Two verbs off the daemon's surface in one slice** — the three forwarders
gone, the module's face (`ReasoningService`) added.

The **module did not move**: the vocabulary was already the module's, and the
slice deleted the copy that stood in front of it. The lines that went **up**
are the ones that carry the conditions: the contract by sixteen (the refusal
row), the council application by thirty-two (the nil-guards and the refusal
the daemon's forwarders used to carry) and the research application by twelve
(same).

The app package count did not move, and the reason is on the record:
`kernel/app/council` and `kernel/app/research` still name the daemon for the
**type** they fill their engines from (the kernel handle the composition root
hands over) and for the panel's seats, so the reasoning **binding** was
narrowed, not the packages'.

## 5. The ratchets — and the probes

Three ratchets, **one probe firing all of them at once**:

- the daemon's new reasoning ratchet
  (`kernel/runtime/reasoning_ratchet_test.go`) pins that the three forwarders do
  not come back, and that the daemon still holds the module's wiring, the
  setter and the face;
- the council application's new binding ratchet
  (`kernel/app/council/binding_ratchet_test.go`) pins that the council package
  does not take the reasoning capability from the **daemon** any more;
- the research application's new binding ratchet
  (`kernel/app/research/binding_ratchet_test.go`) pins the same for the
  research package.

The daemon's own surface ceiling (`kernel/runtime/dead_surface_ratchet_test.go`)
followed the floor again: **166 → 164** (220 → 201 → 194 → 179 → 173 → 166 →
164), and the three retired names joined the retired-verb list.

**The probe** put the daemon's `Council` forwarder back and bound the daemon as
the council application's capability: the retired-verb ratchet, the surface
ceiling (165 > 164), the daemon's reasoning ratchet **and** the council
application's binding ratchet all went red at once. The probe was reverted and
the suites went green again.

## 6. The coverage moved with the subject

The daemon's reasoning coverage drove the daemon's verbs, so it went where the
verbs went:

- the **contract** carries the refusal row, and the four ports keep their own
  tests (three one-verb ports and their union);
- the **module's** tests exercise the module's service under the contract's
  names — they did before and they do now, unchanged;
- the **daemon's** tests pin what the daemon still does: it holds the wiring,
  the setter and the face, it holds **no** verb per operation, and a daemon
  with no module holds **no** face at all
  (`TestReasoningUnwiredHoldsNoFace`);
- the **applications'** tests pin the refusal: a daemon with no module yields
  engines that refuse with the contract's row
  (`TestTheCouncilEnginesRefuseWithoutTheReasoningModule`,
  `TestTheResearchEnginesRefuseWithoutADaemon`);
- the **composition root's** tests pin the wiring: the port the tools receive is
  the module's face, and a daemon with no module hands over a **nil** port
  (`TestToolKernelDepsSuppliesTheReasoningPort`,
  `TestToolKernelDepsLeavesTheReasoningPortNilWithoutTheModule`).

## 7. Verification

- the compiler (`go build ./...`, `go vet ./...`);
- the contract's own tests, the module's tests, the applications' tests, the
  daemon's tests, the plugins' tests, the composition root's tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 8. What is left

The reasoning family is done: its vocabulary is the module's, the daemon holds
the module's face, and the applications and the plugins ask the module.

The same shape is waiting for the families that still have forwarders in the
daemon: the **MCP registry** (12), the **standing library** (9) and the
**roster's** remaining write doors (5). The MCP registry is the mixed one: the
registry's lifecycle is the module's, but the **attach/detach** verbs are the
host's (they hold live child processes). The standing library has **no module
home** yet — it needs one before its four writes can move.
