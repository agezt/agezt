# W5.4ax — the guardian seeder's family, and the daemon's three libraries

## What the slice is

The **thirty-seventh** slice of the port programme, and the **twenty-eighth**
one of its last phase: the handle itself.

The fourth slice of that phase whose subject is the **application's** handle,
and the first one whose file reached for **three** of the daemon's libraries
at once: `kernel/app/guardians/host.go` answered the guardian seeder's port
from the live kernel, and the seeder plants guardians — agents, standing
orders and schedules — so the host needed the roster, the standing library
and the schedule library.

## What the slice found

The guardian seeder's reads are **three libraries**:

| library | the family that owns it |
|---|---|
| the roster (the agents) | the roster |
| the standing library (the orders) | the standing |
| the schedule library (the schedules) | the schedule |

Every one of them already belongs to a family that declares its own ports, so
the seeder takes **no rule of its own**: a family that claims a verb another
family owns is exactly what the sharp guard test refuses. The slice adds the
**broad** half for this family instead — the seeder's port names no verb the
guard misses — and reads the verbs from the port's **own source**, not through
the helper the guard runs.

The measurement that moved:

| | before | after |
|---|---|---|
| application files that name the handle **outside** a port file | **7** | **6** |
| application-owned verbs the guard holds | **162** | **162** |
| the adapter's call sites on the handle | **0** | **0** |
| control-plane production files that name the runtime | **12** | **12** |

## What changed

The guardian seeder's ports moved into the application
(`kernel/app/guardians/ports.go`) — eleven capabilities, all held
**unexported**, and all held as **functions**: a question is asked when it is
asked, and a daemon that grows a capability does not grow the port.

```go
type Ports struct {
	agents      func() []agentapi.Profile
	addAgent    func(agentapi.Profile) (agentapi.Profile, error)
	editAgent   func(string, func(*agentapi.Profile)) (agentapi.Profile, bool, error)
	orders      func() []standingapi.Order
	addOrder    func(standingapi.Order) (standingapi.Order, error)
	editOrder   func(string, func(*standingapi.Order)) (standingapi.Order, bool, error)
	setOrder    func(string, bool) (standingapi.Order, error)
	entries     func() []cadenceapi.Entry
	reschedule  func(string, string, time.Duration, int, int) (bool, error)
	addInterval func(string, time.Duration, string, string) (cadenceapi.Entry, error)
	addDaily    func(string, int, string, string) (cadenceapi.Entry, error)
}
```

The host keeps the ports and nothing else
(`kernel/app/guardians/host.go`):

```go
type kernelHost struct{ ports Ports }

func NewKernelHost(ports Ports) guardianapi.Host {
	if !ports.Ready() {
		return nil
	}
	return &kernelHost{ports: ports}
}
```

and the composition root asks for the ports
(`cmd/agezt/guardians.go`):

```go
return builtinguardians.SeedAll(guardians.NewKernelHost(guardians.PortsFromKernel(k)), "")
```

### Three refusals, written once

The daemon's **own files** may be absent — the standing library is a file the
kernel holds a port for, and the schedule library is another — and each
absence has its **own** refusal:

| the daemon | the answer |
|---|---|
| holds no daemon at all | `errNoDaemon` |
| holds no standing library | `errNoStandingLibrary` |
| holds no schedule library | `errNoScheduleLibrary` |

The refusals are written **once**, in the port, and the host does not spell
them again. The seeder can tell the three apart, because a daemon with no
standing library is **not** a daemon with no schedule library — and neither of
them is a daemon with no daemon.

The **reads** answer nothing when the library is absent, and the **writes**
refuse: "absent" is not "empty", and a refusal that answers with an empty
list reads like "nothing to do".

## The rule

The guardian seeder has **no rule of its own**. It reads the roster's verbs,
the standing family's verbs and the schedule family's verbs, and each of those
belongs to a family that already declares its own ports.

What the slice adds is the **broad** half, and it is held on the port's own
source:

```go
raw, err := os.ReadFile(filepath.Join("..", "app", "guardians", "ports.go"))
named := regexp.MustCompile(`\bk\.([A-Z][A-Za-z0-9_]*)`).FindAllStringSubmatch(string(raw), -1)
```

Every verb the port names is checked against the guard's list, and a verb the
guard misses is a verb the adapter may name.

The port's **own names** are not the kernel's: `Agents`, `AddAgent`,
`EditAgent`, `Orders`, `AddOrder`, `EditOrder`, `SetOrderEnabled`, `Entries`,
`Reschedule`, `AddInterval` and `AddDaily` — a port that carried the name of
the verb it stands for would let the adapter name the kernel's verb through the
port's door.

## The tests

**5 permanent tests** — four in the guardian family's own package, one in the
guard:

- `TestTheGuardianPortsReachTheDaemon` — the filled half: the port really
  reads and really writes all three of the daemon's libraries, and the edit is
  stored in the roster.
- `TestTheGuardianPortsRefuseForAHostWithNoDaemon` — the empty half: no
  daemon, and each write answers with its own refusal.
- `TestTheGuardianPortsRefuseForADaemonWithoutItsLibraries` — the **third**
  half: the daemon is there, its libraries are not — the roster half works and
  each library half refuses with its own refusal.
- `TestTheGuardianPortNamesAreNotTheKernelVerbs` — the port's fields and
  methods are not the kernel's verb names.
- `TestTheGuardianPortsNameNoKernelVerbTheGuardMisses` — the broad half, in
  the guard.

The application's ratchet moved with the slice:
`applicationHandleNamingFiles` is **7 → 6**.

## The mutations

**10 mutations, 10 caught**, in two independent passes with **byte-identical**
logs (SHA-256 `3BFBB7D29D589F5D62F52648AAD53B77ADB942EDBA26D636DFED63BF92871648`):

- **M01** the port fills nothing — the seeder is handed a port that answers
  "no daemon" for every question.
- **M02** a host with no daemon asks the daemon anyway — the nil check is
  gone, and the port panics instead of answering.
- **M03** the standing library's absence is not noticed.
- **M04** the schedule library's absence is not noticed.
- **M05** the handle walks back into the host — and the **application
  ratchet** catches it.
- **M06** the seeder is handed a host that calls into nothing.
- **M07** the port edits nothing — the roster's edit answers "found" and
  changes nothing.
- **M08** the port sees no schedules.
- **M09** the port sees no agents.
- **M10** the port sees no orders.

All ten were caught on the first pass; none survived, and none failed to
compile.

The harness needed one fix before it could run: the standing write's refusal
and the standing pause's refusal are the **same line of text**, so the anchor
alone matched twice and the harness refused to mutate. The anchor was given
the line that follows it as context, which is the same discipline the
programme applies to its own code — an anchor that matches twice is not an
anchor, and a mutation that cannot be placed exactly is not a measurement.

## What the slice does not do

- It does not move the **other six** files that still hold the handle: the
  runs family (four files), the overseer and the introspector. The ratchet is
  at **6**, and every one of those is the next slice's business.
- It does not move the **runs family's** handle: that family holds it for the
  roster, the model, the bus and the run verbs, and it is the biggest piece
  of what is left.
