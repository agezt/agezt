# W5.4bc — the execute path asks its own ports, and the handle leaves the application

## What the slice is

The **forty-second** slice of the port programme, and the **thirty-third** one
of its last phase: the handle itself — and the **last** slice of that phase.

The ninth slice of that phase whose subject is the **application's** handle,
and the **third** of the runs family's four files: `kernel/app/runs/run_execute.go`
— plus the dry run beside it, `run_dryrun.go`, which reached for the same
handle.

## What the slice found

The execute path reached for the handle **seven** times:

| read | the kernel's verb | the family that owns it |
|---|---|---|
| the bus the run's events travel on | `Bus` | the event bus |
| the run's own subject on the bus | `SubjectForRun` | **nobody** |
| the run in flight, when the operator's client goes away | `CancelRun` | the overseer |
| the assured loop | `RunAssured` | **nobody** |
| the plain run | `RunWith` | the runs |
| the run with a retry policy | `RunWithRetry` | the runs |
| the tool the remote peer is reached through | `RunTool` | the tools |

And the dry run reached for it **five** more times: the daemon's default
system prompt, its wall-clock ceiling, the live provider, the catalog and the
tool registry.

The measurement that moved:

| | before | after |
|---|---|---|
| application files that name the handle **outside** a port file | **1** | **0** |
| application-owned verbs the guard holds | **165** | **168** |
| the adapter's call sites on the handle | **0** | **0** |
| control-plane production files that name the runtime | **12** | **12** |

**Zero.** The application layer names the kernel's handle in no file outside a
family's port file, and the port file is where the handle is *filled*.

## What changed

The execute path and the dry run now hold the ports and nothing else. The
port grew the daemon-side capabilities the two paths used to reach for, and
**every other family's capability is asked of the family that owns it**, so a
verb is never named twice:

```go
profile:      approster.ProfilePortsFromKernel(k).Get,
defaultModel: identity.DefaultModel,
correlation:  appcorrelation.PortsFromKernel(k).Mint,
execProfile:  appexecprofile.PortsFromKernel(k),
tools:        apptools.PortsFromKernel(k),
provider:     appprovider.PortsFromKernel(k),
catalog:      appcatalog.PortsFromKernel(k),
identity:     identity,
bus:          bus,
launcher:     PortsFromKernel(k),
```

The roster answers for the agent lookup, the persona family for the daemon's
default identity, the correlation family for the correlation, the catalog
family for the model's own row, the provider family for the live provider,
the tools family for the registry **and the runner**, and the event bus family
for the bus itself.

### The run's own end belongs to the family that owns runs

`CancelRun` was the **overseer's** by exact name since the W5.4ba slice: it
was unguarded then, and the overseer was the only family that read it.

The **runs** family reads it too — a run that outlives the operator's client
stops itself — and the run's own end belongs to the family that owns runs:
the family that starts a run also stops it. So the verb **moved**: the runs
family owns it now, and the overseer asks that family for it. The supervisor
stops a run by asking the family that owns runs.

### The runner belongs to the tools family

The remote path runs a tool through the kernel's own runner. The runner is
the **tools** family's business — the guard has said so by word since the
tools family was written ("the tool registry, the script-tool forge, the
**tool runner** and the MCP servers") — and the tools family's port now
carries it (`Invoke`). A family that runs a tool asks the family that owns
the registry.

### The remote event travels on the run's own subject

`PublishRemoteRunEvent` took the handle and published on the bus directly. It
is now the port's own `PublishRemoteEvent`: the subject convention — the
run's own stream — belongs to the runs family, and the mirror publishes
through the family's port.

## The rule

The runs family's rule was a **word** rule (`Run` **and** `With`) plus one
exact name. It now answers to **five** exact names:

| verb | what it is |
|---|---|
| `AdmitImages` | the vision gate a run asks before it looks at anything |
| `SubjectForRun` | the run's own subject on the bus |
| `RunAssured` | the loop that runs the intent until the answer is assured |
| `MaxDuration` | the daemon's own wall-clock ceiling |
| `CancelRun` | the run's own end — moved from the overseer |

Three of them were **unguarded** until this slice, and none of them carries
"Run" **and** "With" — a word rule cannot find them. Each was read by the
runs family alone, so the runs family owns them by exact name.

The guard now holds **168** application-owned verbs, up from **165**. The
overseer answers to **one** exact name now (`ActiveRunIDs`).

## The tests

**6 permanent tests** in the runs family's own package:

- `TestTheRunPortsReachTheDaemonForTheExecutePath` — the filled half: the
  run's own subject, the bus, the launcher, the daemon's ceiling, the
  daemon's default prompt, the live provider, the catalog, the registry and
  the runner.
- `TestTheRunPortsRefuseTheExecutePathWithoutADaemon` — the empty half: a
  host with no daemon holds no bus, no subject, no ceiling, no prompt, no
  provider, no registry — and every run verb refuses.
- `TestTheRunPortsPublishTheRemoteEventOntoTheRunStream` — the remote
  event lands on the **run's own subject** and carries the run's correlation.
- `TestTheRunPortsCarryTheDaemonsDefaultPrompt` — the daemon's default
  prompt is the identity family's own, and the run's port carries it.
- `TestTheRunEventsReachTheCaller` — the execute path listens on the run's
  own subject, so the run's events reach the operator.
- `TestTheRunPortsCarryTheAssuredLoop` — the assured loop is the daemon's
  own, and the run's port carries it.

The application's ratchet moved with the slice:
`applicationHandleNamingFiles` is **1 → 0**.

## The mutations

**24 mutations, 24 caught**, in two independent passes with **byte-identical**
logs (SHA-256 `B20EA480EC893E0B8DFB260BFC3D772F7199D36F47AC6EFF751675838EB4332F`):

- **M01** the port fills nothing.
- **M02** a host with no daemon is given a daemon.
- **M03** the port has no bus.
- **M04** the port has no subject.
- **M05** the port has no ceiling.
- **M06** the port has no prompt.
- **M07** the port has no provider.
- **M08** the port asks for no agent.
- **M09** the assured loop never runs.
- **M10** the launch port cancels everything.
- **M11** the port answers "cancelled" always.
- **M12** the ports answer with no subject.
- **M13** the ports answer with no ceiling.
- **M14** the ports answer with no prompt.
- **M15** the ports find no tool.
- **M16** the runner runs nothing.
- **M17** the remote event goes to another subject.
- **M18** the remote event is never published.
- **M19** the execute path listens on another subject.
- **M20** the dry run asks for no tools.
- **M21** the runs family loses the subject verb — the guard's broad half
  catches it.
- **M22** the cancel verb goes back to the overseer — the sharp halves
  catch it.
- **M23** the ratchet constant moves.
- **M24** the handle walks back into the execute path — and the
  **application ratchet** catches it.

**Three mutations were rewritten**, and each rewrite was the mutation's
fault, not the tests':

- The first **M05** and **M13** dropped the daemon's ceiling to zero and
  **survived**: the daemon under test had no ceiling of its own, so the
  mutation produced **no defect** — a zero ceiling and a missing ceiling look
  the same. They were given an observer: a daemon that holds a ceiling.
- The first **M08** answered "no agent" without naming the roster family and
  left the import unused, so it did not compile (**BUILDERR**). It was
  rewritten to keep the import used. The second **M08** assigned a
  two-value answer to one variable and did not compile (**BUILDERR**) — it
  was rewritten to discard both.

The harness itself had a bug, and it is worth writing down: the anchor
helper takes the "after" context as a **string**, so passing `0` for it
makes it **truthy** — the anchor then demanded that the line after the
anchor read "0", and no anchor ever matched. The context is `$null` now.

## What the slice does not do

- It does not change what a run **does**: the same bus, the same subject, the
  same cancel, the same assured loop, the same runner and the same dry-run
  plan. The port is a different door to the same daemon.
- It does not move the **run record's** own fields: the `Run` row is still
  the kernel's row, folded by the host.
- It does not end the programme: the **adapter** still names no kernel verb,
  but the control plane's own production files still name the runtime in
  **twelve** places, and the port programme's next phase is that number.
