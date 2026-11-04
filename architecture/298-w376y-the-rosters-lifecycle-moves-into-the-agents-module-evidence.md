# W3.76y — the roster's lifecycle moves into the agents module

**Slice:** W3.76y (the thirty-third of the runtime's dissolution)
**Base:** `214fc978` (the W3.76x merge)
**Date:** 2026-11-04

---

## 1. The family

The **roster** is the agent list: the durable **agent profiles** — who an agent
is, what it may do, who owns it, whether it is paused or retired — and the
**rules** around them (a shipped guardian may be paused or retired but never
removed).

W3.56 moved the roster's **file** out of the daemon: the composition root
opens `roster/roster.json` and hands the kernel the contract's store port
(`cfg.Roster`), and the kernel opens no roster of its own. W3.76n moved the
**lifecycle** down too: the module keeps the profile rules, the events and the
guardian rule, and the daemon forwarded five verbs to the module's service.

What W3.76n left behind was a **third spelling** of one family.

## 2. Three spellings

| where | the names |
|---|---|
| the contract's **file** port (`agentapi.Store`) | `Add`, `SetEnabled`, `SetRetired`, `Update`, `Remove`, `Get`, `List`, `Count` |
| the module's **lifecycle** port (`agentsapi.RosterService`) | `Add`, `SetEnabled`, `SetRetired`, `Update`, `Remove` |
| the **daemon**'s forwarders | `AddProfile`, `SetProfileEnabled`, `SetProfileRetired`, `UpdateProfile`, `RemoveProfile` |

The contract and the module already agreed — the module's service answers the
**contract's** names. The daemon's five were the odd ones out: a host that
forwarded the module's operations under names of its own said the same five
operations a third way, and every caller above it had to learn the third
spelling.

The slice took the third spelling away. The daemon speaks **no** verb per
roster operation any more.

## 3. What moved

- **the contract did not move at all** (603 lines before and after). It already
  named the family — the file port's write half carries the contract's names,
  and the refusal row `agentapi.ErrNoRoster` was already there. This is the
  W3.76w shape (the contract was already right), not the W3.76x shape (where
  the contract's port was too narrow and had to grow).
- **the module** did not move either (1,016 lines, 207 in its api package): the
  agents module already kept the lifecycle, the events and the guardian rule.
  W3.76y only stopped the daemon from answering under names of its own.
- **the daemon** lost **five** verbs and keeps:
  - `Roster` — the roster's **FILE** (the contract's store port; the
    composition root opens it);
  - `RosterGet`, `RosterList` — the **read gates** (the host's nil-safe view
    over the contract's port; a read over a port is the host's);
  - `AgentImpact` — the read that answers "what depends on this agent", which
    stays with the host because the standing orders are **another** domain's
    rows and a module may not name another module's data;
  - `RosterService` — the module's **FACE**.
- **the application layer** binds the face:
  - the **roster application** (`kernel/app/roster`) — the control plane's
    roster handlers go through it — now asks the module's service for the five
    writes and keeps the host's reads;
  - the **overseer** (the fleet's supervision: pause, retire, edit, create,
    delete) asks the same face;
  - the **config tool's adapter** (the agent-scoped write path the plugin
    reaches) asks the same face — the plugin's own port
    (`configapi.Kernel.UpdateProfile`) did **not** change, because the plugin
    may not name the module and the adapter is the layer that may;
  - the **guardian seeder** (the fleet's own self-healing agents) asks the same
    face.
- **the agents module's self-repair** (the escalation that pauses or retires a
  runaway agent) asks the same face through the daemon it already holds.

## 4. The refusal moved with the verbs

The daemon's five forwarders refused with the contract's row
(`agentapi.ErrNoRoster`) when the host held no roster. The daemon holds **no
face** in that case now, so the refusal belongs to the layer that asks:

- the daemon's reads still answer "no agents" (they need no face);
- the applications' writes refuse with `agentapi.ErrNoRoster` — the same row,
  the same comparison with `errors.Is`.

The refusal is **one row** and it is the contract's. A daemon that holds no
roster holds no face rather than a face that calls into nothing: a nil
interface's method call is a panic, and a panic is not an answer.

## 5. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 152 | **148** |
| the daemon's production lines | 5,657 | **5,635** |
| the daemon's production files | 54 | **54** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the agents module's roster package | 1,016 | **1,016** |
| the agents module's api package | 207 | **207** |
| the agent contract | 603 | **603** |
| the roster application | 6,665 | **6,724** |
| the overseer application | 1,281 | **1,312** |
| the config tool's adapter | 119 | **132** |
| the guardian seeder | 311 | **322** |

**Four verbs off the daemon's surface in one slice** — five forwarders gone,
the module's face added.

The module and the contract did not move: the slice was about the **daemon's**
third spelling, and the two below it were already right. The lines that went
**up** are the applications' — they carry the conditions now (the face may be
absent, and every write asks it first).

## 6. The ratchets — and the probe

The daemon's own roster ratchet (W3.76n) **grew** rather than being replaced:

- it now pins that the daemon holds **no** verb per roster operation
  (`TestTheDaemonHoldsNoRosterForwarder`);
- it pins that the daemon still holds the reads and the face
  (`TestTheDaemonKeepsTheRosterNames`);
- it pins that the daemon does not write through the roster's **file port** —
  the port it holds for the reads — in either shape (`k.Roster().Add(…)` and
  `store.Add(…)`).

A **new** binding ratchet in the roster application
(`kernel/app/roster/binding_ratchet_test.go`) pins the same for the
application: a write straight into the store would plant a profile without
journalling its transition.

The daemon's own surface ceiling followed the floor again: **152 → 148** (220 →
201 → 194 → 179 → 173 → 166 → 164 → 156 → 152 → 148), and the five retired
names joined the retired-verb list.

**The probe** put the daemon's `AddProfile` verb back and wrote through the
file port in the application: the retired-verb ratchet, the surface ceiling
(149 > 148), the daemon's roster ratchet (both halves) **and** the
application's binding ratchet all went red at once — **five** ratchets in one
shot. The probe was reverted and the suites went green again.

## 7. The coverage moved with the subject

- the **daemon's** tests pin what the daemon still does: it holds the file,
  the read gates, the impact read and the module's face; it holds **no** verb
  per operation; and a daemon with no roster holds **no** face at all
  (`TestAKernelWithoutARosterAnswersNoAgents`,
  `TestTheDaemonHoldsNoFaceWithoutAService`);
- the **roster application's** tests pin the wiring and the refusal: the
  application's writes go through the module's face, and a daemon with no
  roster refuses with the contract's row
  (`TestTheProfilePortsRefuseWithoutARoster`);
- the module's own tests were already there — they are the module's, and the
  slice did not move them.

## 8. Verification

- the compiler (`go build ./...`, `go vet ./...`);
- the daemon's tests, the applications' tests, the control plane's tests, the
  plugins' tests, the composition root's tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 9. What is left

The roster's lifecycle is the agents module's, the daemon holds the roster's
file, its read gates and the module's face, and every caller above asks the
module.

The daemon holds **148** public verbs and **5,635** production lines; **forty**
application packages still name it. The run engine's own verbs are the next
large block — and they are the W3 exit criterion's real weight: no type may
have more than sixty methods, and the daemon is the type that breaks it.

The **standing package** itself still lives at `kernel/standing` rather than
`kernel/modules/standing`. The move is cheap — three production importers —
and it is its own slice.
