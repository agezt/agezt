# W3.76ad — the journal's provenance walks lose their second and third spellings

**Slice:** W3.76ad (the thirty-eighth of the runtime's dissolution)
**Base:** `a6cda8d2` (the W3.76ac merge)
**Date:** 2026-11-06

---

## 1. The family

The **event log** is the host's own file (`kernel/journal`), and it answers four
provenance operations in its own names:

| operation | the journal's method |
|---|---|
| the events of a correlation | `Journal.Why` |
| the causation ancestry of an event | `Journal.Causes` |
| the lead run of a sub-agent | `Journal.ParentOf` |
| the BLAKE3 hash-chain check | `Journal.Verify` |

One family had **three** spellings of these four:

1. the **journal's** methods — the body's own home;
2. the **run engine's** re-exports — `Runner.Why`, `Runner.Causes`,
   `Runner.ParentOf`, `Runner.Verify`, every one of them a one-line passthrough
   to `r.k.Journal().…`;
3. the **daemon's** copies — `k.Why`, `k.Causes`, `k.ParentOf`, `k.Verify`,
   one-line passthroughs to the engine's.

## 2. What the daemon held

The daemon's four verbs had **one** production caller between them:

| the daemon's verb | production callers |
|---|---|
| `Why` | **none** — only the daemon's own tests |
| `Causes` | **none** — only the daemon's own tests |
| `ParentOf` | **none** — only the daemon's own tests |
| `Verify` | **one** — the daemon-overview's gate (`kernel/app/system`) |

The gate is the daemon's own: the halt that stops new runs, the resume that
lets them start again, and the journal's hash-chain check. The first two are
the runs module's operations (the gate is an adapter over the module's face,
`LifecycleService`, since W3.76aa). The third was the daemon's own copy of the
journal's check, routed through the run engine.

The daemon's journal door (`k.Journal()`) was already there — the daemon
overview's own `events` port holds it — so the check needed no new door.

## 3. What the slice took away

- the **daemon**: `Why`, `Causes`, `ParentOf` and `Verify` are gone. The daemon
  holds **no** verb per journal operation;
- the **run engine**: the four re-exports are gone. The engine asks the host's
  journal for what it needs (its port carries the journal itself) and holds no
  verb of its own for the provenance walks;
- the **application**: the daemon-overview's gate binds the journal's own
  check now (`k.Journal().Verify`).

The control plane's guard learned the change: `Verify` left the **daemon**
family (it was named there as "the journal's hash-chain check"), and the daemon
family is now three names: `StartTime`, `SubAgentLimits`, `LifecycleService`.

## 4. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 125 | **121** |
| the daemon's production lines | 5,579 | **5,571** |
| the daemon's production files | 55 | **55** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the run engine's package | 922 | **908** |
| the daemon-overview application | 714 | **717** |
| the run engine's host port (`KernelAPI`) | 37 entries | **37 entries** |

**Four verbs off the daemon's surface.** The daemon's lines went down by eight:
the four copies and their account left together. The run engine's package went
down by fourteen — the four re-exports and their comments. The daemon-overview
grew by three: the binding now says where the check lives.

## 5. The ratchets — and the probe

**The daemon's side** (`kernel/runtime/journal_provenance_ratchet_test.go`,
new): the daemon holds **no** verb per journal operation, its production files
declare none, and its delegate file says where the walks live.

**The module's side** (`kernel/modules/runs/runner/journal_reexport_ratchet_test.go`,
new): the engine re-exports **none** of the journal's operations, and it does
not ask the host for a verb of its own either.

**The application's side** (`kernel/app/system/binding_ratchet_test.go`, new):
the daemon-overview's gate does not bind a verb of the daemon's, and it does
reach the journal. The check looks at the **shape** (`k.Verify(` against
`k.Journal().Verify`), because the verb names are the same on both sides.

The daemon's own surface ceiling followed the floor again: **125 → 121** (220 →
201 → 194 → 179 → 173 → 166 → 164 → 156 → 152 → 148 → 143 → 135 → 129 → 125 →
121), and the four retired names joined the retired-verb list.

**The probe** put the daemon's `Why` and `Verify` back, had the run engine
re-export `Why` again, and had the daemon-overview's gate bind the daemon's own
verb: the retired-verb ratchet, the surface ceiling (123 > 121), the daemon's
two journal ratchets, the module's re-export ratchet **and** the application's
binding ratchet all went red at once — **six** ratchets in one shot. The probe
was reverted and the suites went green again.

## 6. The coverage moved with the subject

The daemon's four verbs were the daemon's own tests' only callers, so the tests
moved with the subject: the causation test and the runtime test now ask the
journal through the daemon's own journal door (`k.Journal().Why(…)`), and the
daemon-overview's gate test still asks the gate — which now reaches the
journal.

## 7. Verification

- the compiler (`go build ./...`, `go vet ./...`) and `gofmt`;
- the daemon's tests, the run engine's tests, the daemon-overview's tests, the
  control plane's tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 8. What is left

The journal's provenance walks have **one** spelling, and it is the journal's.

The daemon holds **121** public verbs and **5,571** production lines; **forty**
application packages still name it; the run engine's host port still carries
**37** entries.

What is left in the daemon is the run's **own machinery** rather than its verbs:
`runtime_lifecycle.go`'s living verbs (`SetupRunState`, `CleanupRunState`,
`DeregisterRunSteer` — the host's own bookkeeping), the plan runner, the resume
handshake and the prompt doors — the surface a run touches on its way through
the host.

The **standing package** itself still lives at `kernel/standing`, and its
eventual home is the **triggers module** (`kernel/modules/triggers`) — the
architecture gate already places both `kernel/cadence` and `kernel/standing` in
that module, so the family's boundary is drawn; the module's directory does not
exist yet, and the move is a path move (four production importers), not a
lifecycle one.
