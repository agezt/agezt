# W5.4au — the daemon overview, and the handle on the application side

## What the slice is

The **thirty-fourth** slice of the port programme, and the **twenty-fifth** one
of its last phase: the handle itself.

It is the **first** slice of that phase whose subject is not the adapter's
handle but the **application's**. The adapter stopped naming kernel verbs on
the handle in the previous slice (W5.4at): after it, the adapter's call sites
on the handle were **zero**. What was left was the other side of the same
door — the application layer, which still held the handle in **twelve** files.

**This slice moves the biggest of them**: the daemon's own **overview** — the
`agt status` surface — held the whole kernel and named **thirteen** kernel
verbs across **eight** families. The overview now takes a **port**, and the
handle leaves the service.

## What the slice found

The overview's thirteen reads, and the families they came from:

| read | the family that owns it | the port's name |
|---|---|---|
| `Journal()` | the journal | `Events()` |
| `StartTime()` | the daemon's own | `Started()` |
| `SubAgentLimits()` | the daemon's own | `Delegation()` |
| `Schedules()` | the schedules | `Entries()` |
| `ScheduleEngine()` | the schedules | `Running()` |
| `Approvals()` | the approvals | `Pending()` |
| `World()` | the world | `Entities()` |
| `Forge()` | the skill library | `Skills()` |
| `Model()` | the persona | `DefaultModel()` |
| `IsHalted()` | the daemon's own | `Stopped()` |
| `ActiveRuns()` | the daemon's own | `ActiveRunCount()` |
| `Tools()` | the tools | `ToolCount()` |
| `MemoryCount()` | the memory | `MemoryRecordCount()` |

Nine of the thirteen already belonged to a family that declares its own ports.
**Four** belonged to nobody: the daemon's own state — when it started, the
ceilings its delegation runs under, whether it has halted and how many runs
are in flight — and they are the ones this slice's rule claims.

The measurement that moved:

| | before | after |
|---|---|---|
| application files that name the handle **outside** a port file | **10** | **9** |
| the adapter's call sites on the handle | **0** | **0** |
| control-plane production files that name the runtime | **12** | **12** |

The application layer's own number is the phase's new measurement, and it is
machine-checked (`kernel/app/handle_ratchet_test.go`): the port file is where
the handle is **filled** — that is the application's own business — and a
service outside a port file that names the handle is a service that reaches
for the kernel itself. The count only ever goes down.

## What changed

The daemon overview's ports moved into the application
(`kernel/app/system/ports.go`):

```go
type Ports struct {
	events     func() *journalstore.Journal
	started    func() time.Time
	delegation func() types.SubAgentLimits
	entries    func() []cadenceapi.Entry
	running    func() (int, bool)
	pending    func() int
	entities   func() int
	skills     func() int
	model      func() string
	stopped    func() bool
	active     func() int
	tools      func() int
	records    func() int
	gate       func() Gate
}
```

The service takes the port, and the adapter keeps the binding
(`kernel/controlplane/system_ports.go`), which does **not** name the runtime:

```go
func (s *Server) systemPorts() system.Ports {
	return system.PortsFromKernel(s.k)
}
```

### The port's names are not the kernel's

The port answers `DefaultModel`, not `Model`; `Stopped`, not `IsHalted`;
`ToolCount`, not `Tools`; `MemoryRecordCount`, not `MemoryCount`. **This is
the fifth time** this programme learned the same lesson, and the test that
holds it is a plain reflection over the port's own method set and the
kernel's: a port that carried the name of the verb it stands for would let
the adapter name the kernel's verb through the port's door.

The **fields** are held unexported, for the same reason the programme learned
in the journal slice: a port field that answers to a verb is a verb the
adapter may name. The fields are the port's own vocabulary; the **methods**
are what the overview asks.

### The refusal is a refusal, and the gate is a capability

A host with **no daemon** holds no daemon to report on, and the surface says
so: `ErrNoDaemon` is written **once**, in the port's own file, and the status
refuses rather than answering with zeros. A status full of zeros would read
as a claim about a daemon that does not exist ("the daemon has no tools and
no memory").

The daemon's own **gate** — the halt, the resume and the journal's hash-chain
check — is a **capability**, not the handle. The lifecycle service
(`kernel/app/system/lifecycle.go`) used to hold the whole kernel to reach
three verbs; it now holds a `Gate`, and the port fills it. A host with no
daemon holds no gate, and the lifecycle service refuses (`ErrNoGate`) rather
than halting nothing and answering "halted".

## The rule

The daemon family's rule is **by name**, and it answers to **seven exact
names**: `StartTime`, `SubAgentLimits`, `IsHalted`, `ActiveRuns`, `HaltWith`,
`ResumeWith`, `Verify`.

It cannot be a word rule, and the reason is worth writing down: **the daemon's
verbs share no word**. A "Halt" rule would take the drain from the
**lifecycle** family (the drain halts), and a "Verify" rule would take
`VerifyCompletion` — the workboard's proof. A rule that reaches into another
family's verbs makes two families claim the same verb, and the sharp guard
test exists to stop exactly that.

The rule is held in **both** directions:

- **broad** — the guard names every daemon verb, and the daemon overview's
  port names no verb the guard misses. The second half is checked by reading
  the port's **own source** (`k.<Verb>`), not through the helper the guard
  runs: a test that asks the helper what the helper owns cannot see the
  helper reach for something that is not its own.
- **sharp** — the daemon family's list claims no other family's verb, and it
  does not claim the whole kernel.

The guard now holds **161** application-owned verbs.

## The tests

**9 permanent tests** in the application's own packages
(`kernel/app/system/ports_test.go`, `kernel/controlplane/system_ports_test.go`
and `kernel/app/handle_ratchet_test.go`), plus **three** in the guard:

- `TestTheDaemonOverviewPortsReachTheDaemon` — the port really READS the
  daemon: its journal, its start time, its tools, its memory records, its
  model, its world entities, its skills, its delegation ceilings, its
  approvals, its halt flag and its runs in flight.
- `TestTheDaemonOverviewPortsRefuseForAHostWithNoDaemon` — the empty half:
  no journal, no gate, and the **surface refuses** with `ErrNoDaemon`.
- `TestTheDaemonOverviewGateRefusesForAHostWithNoDaemon` — the gate's empty
  half: halt, resume and verify all refuse with `ErrNoGate`.
- `TestTheDaemonOverviewGateReachesTheDaemon` — the gate really reaches the
  daemon: it halts it, resumes it, and checks its journal.
- `TestTheDaemonOverviewLifecycleHaltsTheDaemon` — the **surface** halts the
  daemon, and the daemon is halted afterwards. This test exists because of a
  mutation that survived: the first draft asked the **gate** and never the
  **service**, so a service that halted nothing was invisible.
- `TestTheDaemonOverviewPortAsksWhenItIsAsked` — the port asks **when it is
  asked**: the daemon is halted *after* the port was filled, and the port
  reports the daemon as it is now.
- `TestTheDaemonOverviewPortNamesAreNotTheKernelVerbs` — the port's method
  set and its field names are not the kernel's verb names.
- `TestTheApplicationLayerNamesTheHandleOnlyInPortFiles` — the phase's
  ratchet: the application layer names the handle in exactly **9** files
  outside a port file.
- `TestTheAdapterBindsTheDaemonOverviewPortsToTheDaemon` — the binding really
  reads the daemon, and the overview built on it reports that daemon.

## The mutations

**14 mutations, 14 caught**, in two independent passes with **byte-identical**
logs (SHA-256 `55B765DD892B3D09248C23576A6F6C58EC2AC979D96AA62571495829FFBC3D60`):

- **M01** the port fills nothing — the daemon is never read.
- **M02** a host with no daemon asks the daemon anyway — the nil check is
  gone, and the port panics instead of answering.
- **M03** the refusal is swallowed — the status answers zeros for a daemon
  that does not exist.
- **M04** the port carries a kernel verb's name — `DefaultModel` becomes
  `Model`.
- **M05** the gate is not filled — the daemon's halt and resume have nowhere
  to go.
- **M06** the gate halts nothing — the service answers "halted" without
  reaching the daemon.
- **M07** the delegation ceilings are dropped — the status reports no
  ceilings.
- **M08** the port answers from the moment it was filled — the port caches
  its answers instead of asking.
- **M09** the service takes the handle back — the application layer names
  the handle outside a port file again.
- **M10** the daemon binding answers for nobody.
- **M11** the daemon binding is filled with nothing.
- **M12** the guard forgets the daemon family — four verbs the application
  owns are unguarded.
- **M13** the family claims another family's verb — `Provider` joins the
  daemon's list.
- **M14** the port names a verb the guard misses — `IsHalted` becomes
  `Halted`, and nothing but the guard's own test notices.

Three of the mutations did not compile on the first run, and a mutation that
does not compile measures nothing: they were **rewritten**, not excused.
Two mutations **survived** the first run, and both were the programme's own
lesson coming back:

- **M06** survived because the test asked the **port** and not the
  **surface**. The port programme's rule — *ask the surface, do not ask the
  port* — is there for exactly this reason.
- **M08** survived because the mutation did not do what its name said: it
  captured the value on every **call**, not at **fill** time, so it was not a
  cache at all. The mutation was wrong, not the test.

## What the slice does not do

- It does not move the **other** application files that still hold the handle
  — the runs family (four files), the overseer, the introspector, the
  execution profile, the guardians and the config tool. The ratchet is at
  **9**, and every one of those is the next slice's business.
- It does not give the world, the approvals, the schedules, the skill
  library, the tools or the memory a port of their own for the overview's
  sake. The overview is a **reader**, and a reader declares its **own** narrow
  view over what it reads — the same rule the journal slice wrote down.
- It does not move the daemon's **halt** into the lifecycle family's port:
  the gate is the daemon's own, and the overview's package is where the
  daemon's own surfaces live.
