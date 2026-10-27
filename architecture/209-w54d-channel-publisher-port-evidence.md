# W5.4d — the channel guard reaches the bus through a port

## What the slice is

W5.4's second ratchet is the **import** one. W5.4b took the toolbox edge,
W5.4c took the two update edges; this slice takes the **channel** edge — the
last one that is a single function — and the ratchet goes **4 → 3**.

**The edge:** `upward kernel/channel -> kernel/bus`.

The channel package is a **contract** (layer 1: "Channel interface +
UnifiedMessage + manifest registry") and the bus is **platform** (layer 2).
A contract may not import the platform — the layer order alone catches it, and
the layer order is right here: the channel package holds the *shape* of a
channel, and the bus is the infrastructure that moves events.

The edge was one function:

```go
func Guard(b *bus.Bus, channelName string, fn func())
```

`Guard` runs a channel handler and recovers a panic, so one malformed
inbound message cannot take the daemon down. The panic is journalled as a
`channel.error` event — through the bus, which the contract package was not
allowed to name.

## What the measurement said, and where it was half right

The measured map had called this edge *"the nil-interface trap"* — and the
trap is real, but it is not where the map put it.

The trap is the **typed nil**: a plugin's config holds `*bus.Bus`, and a
plugin with no bus wired holds a **nil pointer**. Handed to an interface
parameter, a nil `*bus.Bus` becomes a **non-nil interface**. The old code
checked `b != nil` on the concrete pointer, so the check fired; move the
parameter to an interface and the same value sails past every nil check
downstream — the holder would hold a bus that is not there, and the publish
inside the recover would panic on a nil receiver.

That is the same trap the update port walked into two slices ago (W5.4c) and
the board port before that (W3.59).

## What changed

### The guard takes a port

```go
type Publisher interface {
	Publish(event.Spec) (*event.Event, error)
}

func Guard(p Publisher, channelName string, fn func()) (attempted bool)
```

The bus satisfies the port — the eleven call sites (`plugins/channels/*` and
`cmd/agezt`) pass their `*bus.Bus` unchanged, because the port is satisfied
structurally.

### The error path is panic-safe on its own

The publish happens inside the very `recover` that is supposed to contain the
damage. A publisher that is broken in its own right — a nil bus behind a
non-nil interface, or any publisher that panics — must not take the daemon
with it. The publish therefore has its own `recover`.

### The guard reports whether it tried

`Guard` returns whether it **attempted** to journal the recovered panic.

This is what makes the nil check load-bearing rather than decorative: with
no publisher configured there is nowhere to publish to, and the guard says so
(false) instead of raising a panic of its own on every error event. A
publisher that is broken is still an attempt — the guard tried, and the
publisher refused.

The eleven call sites ignore the return value, which is legal Go and keeps
the change to one file.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `TestGuard_RecoversPanicAndJournals` | the panic is recovered and a `channel.error` event is journalled |
| `TestGuard_RunsFnAndStaysQuietWhenNoPanic` | the handler runs, and the quiet path journals nothing |
| `TestGuard_NilBusStillRecovers` | **no publisher**: nothing is attempted and nothing crashes |
| `TestGuard_TypedNilPublisherStillRecovers` | **the trap**: a nil `*bus.Bus` behind a non-nil interface is contained |
| `TestGuard_ABrokenPublisherDoesNotTakeTheDaemonDown` | a publisher that panics cannot take the daemon with it |
| `TestGuard_JournalsTheChannelAndThePanicValue` | the journalled event stays diagnosable: subject, actor and payload |
| `TestGuard_ReportsTheJournalAttempt` | the guard reports the attempt — and reports none when there is no panic |

### The mutation run

11 mutations, 11 CAUGHT, two independent runs with byte-identical logs.

| # | Mutation | Caught by |
|---|---|---|
| 1 | the guard does not check for a publisher — a nil publisher is published on | `TestGuard_NilBusStillRecovers` |
| 2 | the error path is not panic-safe — a broken publisher takes the daemon down | `TestGuard_ABrokenPublisherDoesNotTakeTheDaemonDown` |
| 3 | the typed-nil publisher is not contained | `TestGuard_TypedNilPublisherStillRecovers` |
| 4 | the guard does not run the handler at all | `TestGuard_RunsFnAndStaysQuietWhenNoPanic` |
| 5 | the guard does not recover — the panic escapes | `TestGuard_RecoversPanicAndJournals` |
| 6 | the journalled event does not name the channel | `TestGuard_JournalsTheChannelAndThePanicValue` |
| 7 | the journalled event does not name the actor | `TestGuard_JournalsTheChannelAndThePanicValue` |
| 8 | the journalled payload does not carry the channel | `TestGuard_JournalsTheChannelAndThePanicValue` |
| 9 | the journalled payload does not carry the recovered value | `TestGuard_JournalsTheChannelAndThePanicValue` |
| 10 | the journalled event is not a channel error | `TestGuard_RecoversPanicAndJournals` |
| 11 | the port publishes nothing at all — the panic is swallowed | `TestGuard_RecoversPanicAndJournals` |

**The first attempt at mutation 1 survived, and the fix was in the code, not
in the test.** With the nil check and the panic-safe publish both in place,
removing the check changed nothing observable: the nil publisher's publish
panicked, and the nested recover contained it. The two protections overlapped,
and an overlapping protection is not observable from outside.

So the guard was made to **report** whether it attempted the journal. That
turned the nil check into a load-bearing part of the contract: a daemon with
no publisher now says so, and mutation 1 is caught.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 317 packages placed; 3 allowlisted import violations and 0 allowlisted call sites remain (ratchet)** — **import edges down from 4** |
| `go run ./tools/deadcodecheck` | OK — 31 public SDK findings and 1 cross-package test seams allowlisted |
| `go run ./tools/depscheck` | OK — 24 core dependencies, all justified |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | OK |
| `go run ./tools/docclaimscheck -base origin/main` | OK |
| `staticcheck ./...` | clean |
| `go test ./... -count=1 -timeout=40m` | green |

### A local flake that was measured, not assumed

The first full-suite run on this machine reported one failure outside the
slice: `plugins/channels/irc`'s `TestSessionSurvivesAPanickingHandler`, which
**timed out** — its 2-second deadline for a TCP round trip on a loaded box.
The slice's own package passed, and the failing assertion is a wall-clock
wait, not a behaviour.

That is not good enough to ship, so the failure was **attributed by
measurement** rather than argued away:

| Run | Code under test | Result |
|---|---|---|
| 1 | W5.4d | `irc` — `TestSessionSurvivesAPanickingHandler` timed out at 2.35 s |
| 2 | W5.4d | green |
| 3 | W5.4d, 20 iterations under an 8-worker CPU load | green |
| 4 | **pre-W5.4d** (the slice's three files stashed) | green |
| 5 | **pre-W5.4d** (the slice's three files stashed) | `irc` — `TestSessionHandlerErrorSendsApology` timed out at 2.00 s, and `kernel/warden` — two tests timed out |

The class of failure reproduces **on the previous code**, and the failing test
is **not the same test** in the two failing runs. So it is a load-dependent
wall-clock flake on this machine, not a W5.4d regression: the guard's publish
is the same publish the old code made, one interface dispatch and one extra
deferred `recover` cheaper than a disk fsync.

The deadlines were left alone. They are the plugin's own tests, they have
been green through every delivery so far, and widening a deadline to make a
local flake disappear is exactly the kind of change that hides a real
regression.


## What the ratchet says now

Three import edges remain, and none of them is a "one function reached a
platform" edge any more:

| Edge | What it needs |
|---|---|
| `adapter-bypass kernel/controlplane -> kernel/runtime` | the 37 verbs the control plane still takes from the kernel — a redesign, not a port |
| `cross-module kernel/cadence/systemtasks -> kernel/runtime` | nine kernel accessors, two of which are platform types the contract may not name |
| `cross-module kernel/modules/agents/selfrepair -> kernel/runtime` | a runs port that does not exist yet |

## The rules the slice is really about

**A contract that names the platform is a contract that cannot be reused.**
The channel package holds the shape of a channel; the bus is how events move.
Naming the bus in the shape made every holder of the shape a holder of the
platform.

**The error path deserves the same care as the happy path.** The guard exists
so that one bad message cannot end the process — and the publish inside the
recover was the one place where the process could still end. The publish has
its own recover now.

**A protection that cannot be observed is not a protection.** The nil check
and the panic-safe publish overlapped: removing either changed nothing. The
guard now reports whether it attempted the journal, and the check became
part of the contract instead of a decoration.

**A typed nil behind an interface is not nil.** The third time in this
migration (the board, the update engine, the channel guard): the value is
checked where it is still concrete.
