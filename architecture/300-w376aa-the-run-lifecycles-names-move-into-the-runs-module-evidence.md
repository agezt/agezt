# W3.76aa — the run lifecycle's names move into the runs module

**Slice:** W3.76aa (the thirty-fifth of the runtime's dissolution)
**Base:** `4c84edea` (the W3.76z merge)
**Date:** 2026-11-04

---

## 1. The family

The **run lifecycle** is the daemon's own switchboard over the live runs: the
**halt latch** (new runs are refused), the **drain** (the runs in flight are
cancelled and waited for), the **cancel** (one run stops), and the
**read-side queries** (is the daemon halted, how many runs are in flight,
which are they).

It belongs to the **runs module**: the module owns the runs, so it owns the
runs' control.

W3.76d moved the **registry** (the live correlations, their cancels, the halt
latch, the drain accounting and the suspend latch) and the **lifecycle
manager** down into the module. What it left behind was a **second spelling**
of one family.

## 2. Two spellings

| where | the names |
|---|---|
| the **module**'s manager (`lifecycle.Manager`) | `IsHalted`, `Halt`, `HaltWith`, `Resume`, `ResumeWith`, `CancelRun`, `DrainAndHalt`, `ActiveRuns`, `ActiveRunIDs` |
| the **daemon**'s forwarders | the very same nine |

This is the shape W3.76y found in the roster and W3.76z found in the steering:
the module already answered in its own names, and the daemon re-answered the
same operations under names of its own. Here the spellings are **identical** —
which makes the duplication easier to miss and the fix easier to see: there is
no second vocabulary to reconcile, only nine forwarding copies to delete.

The slice took them away. The daemon speaks **no** verb per lifecycle
operation any more.

## 3. The port grew in the module's own api package

The steering slice (W3.76z) opened `kernel/modules/runs/api` — the runs
module's **port package**, the sixth module with one, under the same rule
(P5): a module may import another module's **api** package and nothing else.

The run lifecycle has no contract row and needed none: `api.LifecycleService`
joins `api.SteeringService` in the module's own vocabulary, and the manager
says so to the compiler:

```go
var _ api.LifecycleService = (*Manager)(nil)
```

The port answers with **what happened**, never with a "the daemon is not
there" error — and the manager is **nil-receiver safe**: a daemon that was
never opened answers "nothing is halted, no runs are in flight" instead of
panicking. A nil interface's method call is a panic, and a panic is not an
answer.

## 4. What the daemon keeps

- **`LifecycleService`** — the module's **FACE** (`api.LifecycleService`);
- **`NewCorrelation` and `SubjectForRun`** — the run **identity**, in their own
  file (`kernel/runtime/run_identity.go`). These are the **host's** doors: the
  adapters (the REST and the OpenAI surfaces) subscribe to a run's events by
  its subject, and the run engine and the delegation engine ask the host for
  the id of a run they are about to start. The module's manager mints the same
  ids — it lives in the same module and holds the same registry — but the
  *name* of a run is the host's vocabulary, and the run engine's port asking
  the host for it is a **different slice** (the run engine's own port).
- **`k.registry`** — the module's **STORE** door. The run-start path registers
  and unregisters the live runs (`runner_lifecycle.go`), and that bookkeeping
  is the host's own; the module's manager holds the same store.

## 5. Who asked the module now

| the caller | what it asks |
|---|---|
| the **daemon-lifecycle application** (`kernel/app/lifecycle`) | the drain the update flow needs |
| the **overseer** (`kernel/app/overseer`) | the halted flag, the live run ids, the halt and the resume |
| the **overview** (`kernel/app/system`) | the halted flag, the run count, and its **gate** — the halt, the resume and the journal's hash-chain check, now an adapter (`gateOver`) over the face |
| the **introspector** (`kernel/app/introspect`) | the halted flag and the run count |
| the **runs application** (`kernel/app/runs`) | the cancel |
| the **steering application** (`kernel/app/steer`) | the cancel, beside the steering face — the cancel moved out of the daemon's own verbs, so the port's adapter now holds **two** module faces |

The **plugin** did not change: the overseer tool's `Source` port is filled
over the **overseer application's** ports, and a plugin may not name a module.

The **composition root** asks the module's face for the update flow's drain
(`cmd/agezt/boot_ops.go`: a small `lifecycleDrainer` over the face — the
update flow's own `updateDrainer` interface keeps its two verbs, because the
update module's contract names them that way).

## 6. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 143 | **135** |
| the daemon's production lines | 5,635 | **5,648** |
| the daemon's production files | 54 | **55** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the runs module's api package | 43 (1 file) | **86** (2 files) |
| the runs module's lifecycle package | 205 | **246** |
| the daemon-lifecycle application | 65 | **65** |
| the overseer application | 1,312 | **1,313** |
| the overview application | 689 | **714** |

**Nine verbs retired** — nine forwarders gone, the module's face added: the
daemon's surface is **eight** verbs smaller (143 → 135).

The **lines went UP** (5,635 → 5,648) and the **files** went up (54 → 55).
That is the honest shape of this slice: the nine one-line forwarders gave way
to the face, to the identity door and to the account of the move — and the
account is worth more than the forwarding was. A host's surface is read from
the **verb** count, not the line count; the line count is the size of the
body, and this slice made the body say more.

## 7. The ratchets — and the probe

The daemon's own registry ratchet **turned around**: it used to pin that the
daemon KEEPS the halt/resume/cancel verbs (the "doors"), and it now pins that
it holds **no** verb per lifecycle operation
(`TestTheDaemonHoldsNoLifecycleForwarder`) and that it does not grow the
operation back into its own file (`TestTheDaemonKeepsTheLifecycleFace`).

A **new** binding ratchet in the daemon-lifecycle application
(`kernel/app/lifecycle/binding_ratchet_test.go`) pins the application's half:
the port asks the **module's face**, not a verb of the daemon's.

That ratchet needed a shape, not a name: the verb names are the **same** on
both sides — that is the whole point of one operation having one name — so
the regex looks at the **receiver**:

```go
var theDaemonsLifecycleVerbs = regexp.MustCompile(`\bk\.(IsHalted|Halt|HaltWith|DrainAndHalt|CancelRun|Resume|ResumeWith|ActiveRuns|ActiveRunIDs)\s*\(`)
```

The module's face is reached as `k.LifecycleService().DrainAndHalt(…)`, which
is **not** that shape; the daemon's own verb would be `k.DrainAndHalt(…)`,
which is.

The daemon's own surface ceiling followed the floor again: **143 → 135** (220 →
201 → 194 → 179 → 173 → 166 → 164 → 156 → 152 → 148 → 143 → 135), and the nine
retired names joined the retired-verb list.

**The probe** put the daemon's `IsHalted` and `DrainAndHalt` verbs back and
had the application ask the daemon for the drain again: the retired-verb
ratchet, the surface ceiling (137 > 135), the daemon's registry ratchet (both
halves) **and** the application's binding ratchet all went red at once —
**five** ratchets in one shot. The probe was reverted and the suites went
green again.

## 8. The coverage moved with the subject

- the **daemon's** tests pin what the daemon still does: it hands over the
  module's face, it holds **no** verb per lifecycle operation, and its
  lifecycle file spells out none of the operations;
- the **module's** tests were already there — they are the module's — and the
  manager's nil-receiver safety is what makes the face safe to hand over;
- the **application's** tests pin the binding: the port asks the module's face.

## 9. Verification

- the compiler (`go build ./...`, `go vet ./...`) and `gofmt`;
- the daemon's tests, the applications' tests, the control plane's tests, the
  composition root's tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 10. What is left

The run lifecycle's operations are the runs module's, the daemon hands over
the module's face and keeps the identity doors and the store, and every
caller above asks the module.

The daemon holds **135** public verbs and **5,648** production lines; **forty**
application packages still name it. The run engine's own verbs are the next
large block — and they are the W3 exit criterion's real weight: no type may
have more than sixty methods, and the daemon is the type that breaks it.

The **standing package** itself still lives at `kernel/standing`, and its
eventual home is the **triggers module** (`kernel/modules/triggers`) — the
architecture gate already places both `kernel/cadence` and `kernel/standing`
in that module, so the family's boundary is drawn; the module's directory does
not exist yet, and the move is a path move (four production importers), not a
lifecycle one.
