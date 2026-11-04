# W3.76x — the standing library's lifecycle moves into the standing package

**Slice:** W3.76x (the thirty-second of the runtime's dissolution)
**Base:** `315749b1` (the W3.76w measurement fix merge)
**Date:** 2026-11-04

---

## 1. The family

The **standing library** is the family's data *and* its meaning: the durable
**standing orders** (the recurring instructions an operator leaves for the
fleet), the **rules** that validate them, the **ids** the store assigns, and
the **transitions** the library journals.

W3.50 already moved the library's **file** out of the daemon: the composition
root opens `standing/standing.json` and hands the kernel the contract's store
port (`cfg.Standing`), and the kernel holds no library of its own. What stayed
in the daemon was the **lifecycle** — five verbs over the package's own
operations.

## 2. The port the contract already had — and the one it did not

The **contract** named the standing-orders tool's surface in
`standingapi.Host` — **three verbs**, in the names the users say them:

- `AddStanding`, `RemoveStanding`, `StandingList`.

The daemon held **five** verbs over the same ground: the contract's three plus
`SetStandingEnabled` and `UpdateStanding` — the pause and the edit that the
layers which **own** standing orders need (the standing application, the
guardians' orders, the roster's teardown).

So the family had **two** spellings, and the contract's was the narrower one.
The slice gives the contract the **wider** port, and the daemon spells none of
it.

## 3. What moved

- **the contract** now names the family once: `standingapi.Lifecycle` — the
  tool's `Host` **embedded**, plus `SetStandingEnabled` and `UpdateStanding`.
  The port is the CONTRACT's because more than one party binds it: the
  applications are L4 and the tool is a PLUGIN (L6), and a plugin may reach the
  contract and nothing else. The contract also gained the **refusal row**
  (`standingapi.ErrNoLibrary`): the missing library is one fact, and the
  library, the applications and the callers all have to name it.
- **the standing package** owns the lifecycle: a new `Service` over the store
  port and the host's publisher, answering in the **contract's** names
  (`var _ standingapi.Lifecycle = (*Service)(nil)`). The daemon's semantics
  moved with it **verbatim** — the same payloads, the same `standing.*`
  events, the same "an unknown id is not an error" rule on the edit path.
- **the daemon** lost **five** verbs and keeps two things:
  - `Standing` — the library's **FILE** (the contract's store port; the
    composition root opens it);
  - `StandingService` — the library's **FACE**.
- **the composition root** hands the kernel the library's **builder**
  (`cfg.StandingBuilder`) — the MCP registry's and the roster's shape — so the
  kernel holds a face without naming the package's implementation.
- **the application** (`kernel/app/standing`) binds the face: its own `Writer`
  port already spoke the **contract's** names, so **no adapter was written**.
  The application became **nil-safe**: a daemon with no library holds no face,
  and the application refuses with the **contract's row**
  (`standingapi.ErrNoLibrary`) through a refusing writer instead of calling
  into nothing.
- **the guardians' seeder** binds the same face — and this is where the slice
  closed a **real bug**: it used to plant its orders straight into the store,
  so their transitions never reached the journal. The write now goes through
  the library's lifecycle, and the transition is journalled with it.
- **the plugin** binds the face too (`standingPort(k)` answers
  `k.StandingService()`, nil-safe: a daemon with no library hands over a **nil**
  port and the tool answers "unavailable" itself).
- **two refusals went with the verbs**: the daemon's `errNoStandingLibrary`
  and the guardians application's own spelling of the same row. The missing
  library is one fact, and a second spelling of it is exactly what this
  program removes.

## 4. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 156 | **152** |
| the daemon's production lines | 5,704 | **5,657** |
| the daemon's production files | 54 | **54** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the standing package | 614 | **753** |
| the standing contract | 215 | **247** |
| the standing application | 567 | **612** |

**Four verbs off the daemon's surface in one slice** — five verbs gone, the
library's face added.

The lines that went **up** are the ones that carry the conditions: the
contract's port and its refusal row (+32), the package's service (+139 — the
service is new) and the application's nil-safe writer (+45).

The **contract grew** where W3.76w's did not: the MCP family's contract
already named the whole surface, but the standing family's contract named only
the tool's three verbs. The contract is the right home for the wider port: a
plugin may reach the contract and nothing else, and the applications are L4 —
so the port that both bind has to be in the contract layer.

## 5. The ratchets — and the probe

Three ratchets, **one probe firing all of them at once**:

- the daemon's new standing ratchet
  (`kernel/runtime/standing_ratchet_test.go`) pins that the five verbs do not
  come back, and that the daemon still holds the library's file and the
  library's face;
- the standing application's new binding ratchet
  (`kernel/app/standing/binding_ratchet_test.go`) pins that the application
  does not **write through the daemon's file port** — a write around the
  lifecycle skips the journal, which is the bug the slice closed — and does
  not bind the daemon handle as its writer;
- the guardian seeder's new binding ratchet
  (`kernel/app/guardians/binding_ratchet_test.go`) pins the same for the
  seeder.

The daemon's own surface ceiling (`kernel/runtime/dead_surface_ratchet_test.go`)
followed the floor again: **156 → 152** (220 → 201 → 194 → 179 → 173 → 166 →
164 → 156 → 152), and the five retired names joined the retired-verb list.

**The probe** put the daemon's `AddStanding` verb back and wrote through the
file port in both applications: the retired-verb ratchet, the surface ceiling
(153 > 152), the daemon's standing ratchet **and** the two applications'
binding ratchets all went red at once — **five** ratchets in one shot. The
probe was reverted and the suites went green again.

## 6. The coverage moved with the subject

- the **standing package** keeps its own tests — the lifecycle's refusal
  without a store (`standingapi.ErrNoLibrary`), the four writes and the
  transitions they journal, the unknown id, and the lifecycle that needs no
  publisher;
- the **daemon's** tests pin what the daemon still does: it holds the file,
  it holds the face, it holds **no** verb per operation, and a daemon with no
  library holds **no** face at all
  (`TestTheDaemonHoldsNoStandingFaceWithoutALibrary`);
- the **application's** tests pin the wiring and the refusal: the application's
  writer is the library's face, and a daemon with no library refuses with the
  contract's row;
- the **plugin's** and the **composition root's** tests pin the wiring: the
  tool receives the library's face, not the daemon.

## 7. Verification

- the compiler (`go build ./...`, `go vet ./...`);
- the standing package's own tests, the daemon's tests, the applications'
  tests, the control plane's tests, the plugins' tests, the composition root's
  tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 8. What is left

The standing library's lifecycle is the standing package's, the daemon holds
the library's face, and the applications and the plugin ask the library.

Two things are still open on this family:

- the **standing package itself** still lives at `kernel/standing` rather than
  `kernel/modules/standing`. The move is cheap — three production importers
  (`cmd/agezt/main.go`, `main_standing.go`, `main_toolinvoker.go`) — and it is
  its own slice, not a rider on this one;
- the **roster's** remaining write doors are the next shape: five forwarders
  (`AddProfile`, `SetProfileEnabled`, `SetProfileRetired`, `UpdateProfile`,
  `RemoveProfile`), and the **config plugin's port has to be split** before
  they can move.
