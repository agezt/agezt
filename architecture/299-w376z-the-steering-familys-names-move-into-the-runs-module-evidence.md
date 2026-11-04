# W3.76z — the steering family's names move into the runs module

**Slice:** W3.76z (the thirty-fourth of the runtime's dissolution)
**Base:** `5a9de6cb` (the W3.76y merge)
**Date:** 2026-11-04

---

## 1. The family

The **steering** family is the live control of a run that is already in
flight: **pause** it, **resume** it, **step** it one iteration, **steer** it
with an operator directive, **observe** its steering state and **intervene**
with a protocolized primitive.

The target architecture gives the family to the **runs module**: the module
owns the run, so the module owns the run's control. W3.76b moved the
**operations** down — the service (`kernel/modules/runs/steer`), the per-run
control surface (`steer.Control`) and the intervention grammar
(`kernel/modules/runs/intervention`) — and the daemon kept the doors.

What W3.76b left behind was a **second spelling** of one family.

## 2. Two spellings

| where | the names |
|---|---|
| the **module**'s service (`steer.Service`) | `Pause`, `Resume`, `Step`, `Steer`, `State`, `Intervene` |
| the **daemon**'s forwarders | `PauseRun`, `ResumeRun`, `StepRun`, `SteerRun`, `RunControlState`, `InterveneRun` |

The module already answered in its own names; the daemon re-answered the same
six operations under names of its own, and every caller above had to learn the
second spelling.

The slice took the second spelling away. The daemon speaks **no** verb per
steering operation any more.

## 3. There was no contract to lean on — the module grew a port package

The roster slice (W3.76y) found the contract already naming its family; the
standing slice (W3.76x) found the contract's port **too narrow** and widened
it. The steering family had **neither**: there is no `runsapi` in
`kernel/contract`, and the steering verbs were never a contract row.

So the module grew **its own port package**: `kernel/modules/runs/api`. It is
the **sixth** module with an `api` package (after `agents`, `extensions`,
`providers`, `reasoning` and `work`), and it follows the same rule (P5): a
module may import another module's **api** package and nothing else, so this
is the only door between the runs module and the host.

`api.SteeringService` is the family's one vocabulary — `Pause`, `Resume`,
`Step`, `Steer`, `State`, `Intervene` — and the module's service says so to
the compiler, not to a comment:

```go
var _ api.SteeringService = (*Service)(nil)
```

## 4. What moved

- **the contract did not move** — there is nothing to move: the steering
  family never had a contract row.
- **the module's api package is new** (`kernel/modules/runs/api/steer.go`,
  43 production lines): the port and the rows the port carries
  (`intervention.Request` / `intervention.Result`).
- **the module's service** answers the port (`kernel/modules/runs/steer`).
- **the daemon** lost **six** verbs and keeps:
  - `SteeringService` — the module's **FACE** (`api.SteeringService`);
  - `controlFor` — the **door**: the live run the module steers (private, and
    it stays: the module reaches the live run through its own ports, the
    daemon holds the map).
- **the application layer** binds the face and speaks the module's
  vocabulary: `kernel/app/steer`'s `Runs` port now carries the daemon's own
  `CancelRun` **plus** the module's `api.SteeringService` (embedded), and the
  adapter (`runsFromKernel`) answers the six steering verbs from the module's
  face. A daemon that does not exist yields `refusingRuns` — the port refuses
  rather than calling into nothing.

**The cancel stays with the daemon on purpose**: cancelling a run stops the
run *and* the daemon's own bookkeeping (the registry's cancel, the drain
accounting), so it is the host's verb, not the module's.

## 5. The refusal did not have to move

The daemon's forwarders could not answer "there is no run" with an error: a
run that is not live is not steered, and the caller learns that from the
answer. The module's service already answered that way (`false`, `ok=false`),
so there was **no refusal row** to relocate — the daemon's face is never nil
(the composition root builds the service while opening the daemon), and the
module's service is nil-receiver safe.

That is the one family so far where the slice needed **no** refusal row at
all: the port answers with what happened, never with "the daemon is not
there".

## 6. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 148 | **143** |
| the daemon's production lines | 5,635 | **5,635** |
| the daemon's production files | 54 | **54** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the runs module's api package | — | **43** (new) |
| the runs module's steering package | 428 | **433** |
| the steering application | 302 | **346** |

**Five verbs off the daemon's surface in one slice** — six forwarders gone,
the module's face added.

The daemon's **line count did not move**: the six forwarders' lines were the
six names the module already answered in, and the account of the move (the
face's own documentation) took exactly their place. A line count that hides
a five-verb reduction is a reminder to read the **verb** count, not the line
count, for a host's surface.

The lines that went **up** are the module's port (new) and the application's
(its port now names two families and says which is which).

## 7. The ratchets — and the probe

The daemon's own steering ratchet **turned around**: it used to pin that the
daemon *holds* the six names, and it now pins that the daemon holds **none**
of them (`TestTheDaemonHoldsNoSteeringForwarder`) and that it does not grow
the operation back into its own file
(`TestTheSteeringOperationsLiveInTheRunsModule`).

A **new** binding ratchet in the steering application
(`kernel/app/steer/binding_ratchet_test.go`) pins the application's half: the
port speaks the **module's** vocabulary (not six verbs spelled the daemon's
way) and does not bind the daemon **handle** as its port. Both of its regexes
look for a **parenthesis** (`\b(PauseRun|…)\s*\(`) and for the value **being**
the handle (`(?m)^\s*return k\s*$`), so the file's own history — the comment
that names the retired verbs — cannot turn the ratchet red.

The daemon's own surface ceiling followed the floor again: **148 → 143** (220 →
201 → 194 → 179 → 173 → 166 → 164 → 156 → 152 → 148 → 143), and the six
retired names joined the retired-verb list.

**The probe** put the daemon's `PauseRun` verb back and wrote the daemon's
verbs (and the daemon handle) into the application's port: the retired-verb
ratchet, the surface ceiling (144 > 143), the daemon's steering ratchet (both
halves) **and** the application's binding ratchet (both halves) all went red
at once — **five** ratchets in one shot. The probe was reverted and the suites
went green again.

## 8. The coverage moved with the subject

- the **daemon's** tests pin what the daemon still does: it hands over the
  module's face, it holds **no** verb per steering operation, and a daemon
  without a steering service steers nothing
  (`TestTheDaemonKeepsTheDoors`);
- the **steering application's** tests pin the port's shape: the port carries
  the module's vocabulary and the daemon's own cancel, and the adapter answers
  the six verbs from the module's face;
- the module's own tests were already there — they are the module's, and the
  slice did not move them.

## 9. Verification

- the compiler (`go build ./...`, `go vet ./...`) and `gofmt`;
- the daemon's tests, the steering application's tests, the control plane's
  tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 10. What is left

The steering family's names are the runs module's, the daemon hands over the
module's face and keeps the doors, and every caller above asks the module.

The daemon holds **143** public verbs and **5,635** production lines; **forty**
application packages still name it. The run engine's own verbs are the next
large block — and they are the W3 exit criterion's real weight: no type may
have more than sixty methods, and the daemon is the type that breaks it.

The **standing package** itself still lives at `kernel/standing`, and its
eventual home is the **triggers module** (`kernel/modules/triggers`) — the
architecture gate already places both `kernel/cadence` and `kernel/standing`
in that module, so the family's boundary is drawn; the module's directory does
not exist yet, and the move is a path move (four production importers), not a
lifecycle one.
