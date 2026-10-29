# W5.4ay — the introspector, and the daemon's own self-knowledge

## What the slice is

The **thirty-eighth** slice of the port programme, and the **twenty-ninth**
one of its last phase: the handle itself.

The fifth slice of that phase whose subject is the **application's** handle,
and the first one whose file read **thirteen** different kernel verbs at once:
`kernel/app/introspect/source.go` gathered the daemon's own self-knowledge —
the overview an agent reads when it asks what this daemon is.

## What the slice found

The introspector's reads belong to **nine** families:

| read | the family that owns it |
|---|---|
| the journal's head and tail | the journal |
| the schedules | the schedule |
| the standing orders | the standing |
| the approvals | the approval |
| the world model | the world |
| the skill library | the skill |
| the tools | the tools |
| the memory records | the memory |
| the default model | the persona |
| the start time, the delegation ceilings, the halt, the active runs | the daemon |

Every one of them already belongs to a family that declares its own ports, so
the introspector takes **no rule of its own**: a family that claims a verb
another family owns is exactly what the sharp guard test refuses.

The reaper's scan is asked of the **reaper family** and not of the kernel: the
scan is the reaper's business, and a family that names another family's verb
names it twice.

The measurement that moved:

| | before | after |
|---|---|---|
| application files that name the handle **outside** a port file | **6** | **5** |
| application-owned verbs the guard holds | **162** | **162** |
| the adapter's call sites on the handle | **0** | **0** |
| control-plane production files that name the runtime | **12** | **12** |

## What changed

The introspector's ports moved into the application
(`kernel/app/introspect/ports.go`) — fifteen capabilities, all held
**unexported**, and all held as **functions**: a question is asked when it is
asked, and a daemon that grows a capability does not grow the port.

```go
type Ports struct {
	journalHead       func() (int64, string)
	journalTail       func(int) ([]*event.Event, error)
	started           func() time.Time
	delegation        func() types.SubAgentLimits
	scheduleEntries   func() []cadenceapi.Entry
	pendingApprovals  func() int
	worldEntities     func() int
	activeSkills      func() int
	toolNames         func() []string
	defaultModel      func() string
	stopped           func() bool
	activeRunCount    func() int
	memoryRecordCount func() int
	fleetScan         func(int64) agentapi.ReaperReport
	standingOrders    func() []standingapi.Order
}
```

The source keeps the ports and nothing else
(`kernel/app/introspect/source.go`):

```go
type kernelSource struct {
	ports Ports
}

func NewKernelSource(ports Ports) introspectapi.Source { return &kernelSource{ports: ports} }
```

and the composition root asks for the ports
(`cmd/agezt/tooldeps.go`):

```go
Introspect: introspect.NewKernelSource(introspect.PortsFromKernel(k)),
```

### The names are the port's own

The port answers `JournalHead`, `JournalTail`, `Started`, `Delegation`,
`ScheduleEntries`, `PendingApprovals`, `WorldEntities`, `ActiveSkills`,
`ToolNames`, `DefaultModel`, `Stopped`, `ActiveRunCount`,
`MemoryRecordCount`, `FleetScan` and `StandingOrders` — **never** the kernel's
own verb names. Two of them would have collided: the kernel's own verbs are
`Schedules`, `Standing`, `Model`, `Tools`, `World`, `Forge`, `Journal`,
`Approvals`, `Memory`, `IsHalted` and `ActiveRuns`, and a port that carried
the name of the verb it stands for would let the adapter name the kernel's
verb through the port's door.

### The empty host is not a daemon with no state

A host with **no daemon** holds no daemon to report on, and the gather answers
with **nothing** rather than with a lie. The uptime is the sharp end of it:
measured from the zero time, a host with no daemon would claim to have been
running since the year one — so the gather asks whether the port holds a
daemon at all before it measures anything.

The daemon's own **libraries** may be absent while the daemon is there, and
that is a different answer: the world model, the skill library, the schedule
library and the standing library are the daemon's files, and a daemon that was
given none holds a nil port. The port answers zero for those, and the gather
reports a daemon that owns less than it does — which is the truth.

## The rule

The introspector has **no rule of its own**. It reads nine families' verbs,
and each of those belongs to a family that declares its own ports.

What the slice adds is the **broad** half, and it is held on the port's own
source:

```go
raw, err := os.ReadFile(filepath.Join("..", "app", "introspect", "ports.go"))
named := regexp.MustCompile(`\bk\.([A-Z][A-Za-z0-9_]*)`).FindAllStringSubmatch(string(raw), -1)
```

Every verb the port names is checked against the guard's list, and a verb the
guard misses is a verb the adapter may name. The port names **thirteen** verbs
today — `Journal`, `StartTime`, `SubAgentLimits`, `Schedules`, `Approvals`,
`ActiveRuns`, `MemoryCount`, `World`, `Forge`, `Tools`, `Model`, `IsHalted`
and `Standing` — and every one of them is already guarded.

## The tests

**4 permanent tests** in the introspector's own package, plus the guard:

- `TestTheIntrospectPortsReachTheDaemon` — the filled half: the port really
  reads the daemon's journal, tools, schedules, standing orders, world model,
  memory, model and delegation ceilings, and it answers **as the daemon
  answers**.
- `TestTheIntrospectPortsAnswerAHostWithNoDaemon` — the empty half: no
  daemon, and every capability answers with nothing — and the gather claims
  no uptime for a daemon that does not exist.
- `TestTheIntrospectPortsAnswerADaemonWithoutItsLibraries` — the **third**
  half: the daemon is there, its libraries are not — the reads answer zero
  and the gather does not call into nothing.
- `TestTheIntrospectPortNamesAreNotTheKernelVerbs` — the port's fields and
  methods are not the kernel's verb names.
- `TestTheIntrospectNamesNoKernelVerbTheGuardMisses` — the broad half, in the
  guard.

The application's ratchet moved with the slice:
`applicationHandleNamingFiles` is **6 → 5**.

The two tests that already existed were rewritten, not deleted: the
constructor's test now says that the gather answers with nothing instead of
panicking, and the reaper's projection tests speak the **contract's** row
(`agentapi.ReaperReport`) rather than the runtime's — the runtime's row is an
alias of it, and the application layer has no business naming the runtime's
types.

## The mutations

**13 mutations, 13 caught**, in two independent passes with **byte-identical**
logs (SHA-256 `06856F67D15F5199E5D9C3BD871AA769C4227AA782F6FF95F9AD6658BB7E2C73`):

- **M01** the port fills nothing — the gather reports an empty daemon while
  looking perfectly wired.
- **M02** a host with no daemon asks the daemon anyway — the nil check is
  gone, and the port panics instead of answering.
- **M03** the port does not read the journal.
- **M04** the port sees no tools.
- **M05** the port sees no schedules.
- **M06** the port sees no standing orders.
- **M07** the port does not read the world.
- **M08** the port does not read the memory.
- **M09** the port drops the delegation ceilings.
- **M10** the empty host answers a lie — the uptime is measured from the zero
  time, and the host claims to have been running since the year one.
- **M11** the handle walks back into the source — and the **application
  ratchet** catches it.
- **M12** the gather does not sort the tool names.
- **M13** the projection drops a field — the degraded agent's last reason.

All thirteen were caught on the first pass; none survived, and none failed to
compile.

The harness needed one fix before it could run: the source's struct anchor
was written without the space the file actually has, and an anchor that does
not match is not an anchor.

## What the slice does not do

- It does not move the **other five** files that still hold the handle: the
  runs family (four files) and the overseer. The ratchet is at **5**, and
  every one of those is the next slice's business.
- It does not move the **runs family's** handle: that family holds it for the
  roster, the model, the bus and the run verbs, and it is the biggest piece
  of what is left.
