# W5.4bb — the runs family's resolve path, and the vision gate that belonged to nobody

## What the slice is

The **forty-first** slice of the port programme, and the **thirty-second** one
of its last phase: the handle itself.

The eighth slice of that phase whose subject is the **application's** handle,
and the **second** of the runs family's four files: `kernel/app/runs/run_resolve.go`.

## What the slice found

The resolve path — the part of a run that decides **who** runs, **on what**,
**under which correlation** and **looking at what** — reached for the handle
six times:

| read | the kernel's verb | the family that owns it |
|---|---|---|
| the agent a run runs as | `RosterGet` | the roster |
| the daemon's default model | `Model` | the identity |
| the correlation the run is journaled under | `NewCorrelation` | the correlation |
| the vision gate: what the run may look at | `AdmitImages` | **nobody** |
| the tool registry the execution profiles ask about | `Tools` | the tools |
| the warden's own ports | `Warden` | the execution profile |

Four of the six already belonged to a family, and the warden's was already
asked of the family that owns it since W5.4az. One belonged to **nobody**.

The measurement that moved:

| | before | after |
|---|---|---|
| application files that name the handle **outside** a port file | **3** | **1** |
| application-owned verbs the guard holds | **164** | **165** |
| the adapter's call sites on the handle | **0** | **0** |
| control-plane production files that name the runtime | **12** | **12** |

## What changed

`RunPorts` moved out of `run.go` into the family's **port file**
(`kernel/app/runs/ports.go`), and the port gained the six daemon-side
capabilities the resolve path used to reach for: the roster's lookup, the
daemon's default model, the correlation, the vision gate, the tool registry
and the execution profile family's own ports.

The capabilities are held **unexported** and held as **functions**, and the
resolve path asks the port for them:

```go
if !s.ports.Ready() {
    return nil, ErrNoKernel
}
...
p, found := s.ports.Profile(ref)
resolved.model = s.ports.DefaultModel()
resolved.corr = s.ports.Correlation()
admission, err := s.ports.AdmitVision(ctx, resolved.corr, modelOverride, intent, imageRefs)
```

The execution profile helper stopped taking the handle too: it asks the
**port** for the tools it needs and for the **execution profile family's**
ports for the warden.

### The refusal is written once

A host that holds no daemon holds no daemon to run on. The port says so
(`ErrNoKernel`), and the resolve path returns **that value** — it does not
spell the refusal again, and the test asks for it **by value**
(`errors.Is`), so a second spelling would not be the same refusal.

### The port may not carry the kernel's verb name

The vision gate is the kernel's `AdmitImages`, so the port's accessor is
`AdmitVision`. A port that carried the name of the verb it stands for would
let the adapter name the kernel's verb through the port's door — and a
reflection test holds that line: every method of the port is compared with
the kernel's own method set.

### The handle stays for the execute path

The port still holds the routed kernel in an **unexported** field, and only
the execute path reaches for it: the resolve path does not. That is the
**last** thing that holds it — a service that reaches for the handle reaches
for the whole kernel — and the slice does not pretend otherwise: the ratchet
says **1**, and that one file is named.

## The rule

The runs family's rule was a **word** rule (`Run`, `With`) plus the
exceptions it writes out.

The vision gate belonged to **nobody**: `AdmitImages` carries neither "Run"
nor "With", so the word rule could not find it, and the adapter was free to
name it. The **runs** family is the only family that reads it — the
admission is the run's own gate — so the runs family **owns** it, by
**exact name**, the way the daemon family's seven and the overseer's two are
owned.

The rule is held in **both** directions:

- **broad** — the guard names the verb, and the runs family's port names no
  verb the guard misses (read from the port's **own source**).
- **sharp** — the runs family's list claims no other family's verb.

Reading another family's verb is **not** a reason to write a rule for it:
the port names the roster's lookup, the identity's model and the
correlation's mint, and all three stay with the families that own them.

## The tests

**9 permanent tests**: eight in the runs family's own package, one in the
guard.

- `TestTheRunPortsReachTheDaemon` — the filled half: the port really reads
  the roster, really answers the daemon's default model, really mints a
  correlation and really asks the vision gate.
- `TestTheRunPortsRefuseForAHostWithNoDaemon` — the empty half: a host with
  no daemon holds no agent, no model, no correlation and no tool.
- `TestTheRunServicePortNamesAreNotTheKernelVerbs` — the port's methods are
  not the kernel's verb names.
- `TestTheRunPortsFindARegisteredTool` — the **positive** half of the tool
  registry: a tool the daemon registers is found.
- `TestTheRunPortsHoldTheWarden` — the execution profile family's ports are
  behind the run's ports, and not empty.
- `TestTheRunServiceRefusesAHostWithNoDaemon` — the refusal is the family's
  **own value**.
- `TestTheRunServiceAsksTheVisionGate` — the run asks the gate before it
  looks at anything: a model that cannot see is refused.
- `TestTheRunServiceRefusesTheRemoteProfileWithoutPeers` — the remote
  profile is refused when the daemon registers no `remote_run` tool.
- `TestTheRunsNamesNoKernelVerbTheGuardMisses` — the broad half, in the
  test's own words, read from the port's source.

The application's ratchet moved with the slice:
`applicationHandleNamingFiles` is **3 → 1**.

## The mutations

**26 mutations, 26 caught**, in two independent passes with **byte-identical**
logs (SHA-256 `4581114C6D7A0EB7FD10A2955E0B374C3A5FE339ADE9B13E159C366AE13AB0BE`):

- **M01** the port fills nothing — every run is refused while the port looks
  perfectly wired.
- **M02** a host with no daemon is given an agent.
- **M03** the port finds no agent.
- **M04** the port forgets the daemon's model.
- **M05** the port mints no correlation.
- **M06** the port finds no tool.
- **M07** the port admits every image without asking.
- **M08** the port holds no warden.
- **M09** the port claims a daemon it does not hold.
- **M10** the port claims no daemon at all.
- **M11** the ports answer for no agent.
- **M12** the ports answer with no model.
- **M13** the ports mint no correlation.
- **M14** the ports find every tool.
- **M15** the ports never ask the vision gate.
- **M16** the port carries the kernel's verb name.
- **M17** the resolve ignores the missing daemon.
- **M18** the refusal is spelled again.
- **M19** the resolve ignores the roster.
- **M20** the resolve fixes the model.
- **M21** the resolve mints no correlation.
- **M22** the resolve ignores the execution profile.
- **M23** the resolve ignores the tool registry.
- **M24** the handle walks back into the resolve — and the **application
  ratchet** catches it.
- **M25** the ratchet constant moves — the ratchet stops holding.
- **M26** the vision gate is unguarded — the guard's broad half catches it.

**Three mutations were rewritten**, and each rewrite was the mutation's
fault, not the tests':

- The first **M06** replaced the tool registry with a constant and left the
  `tools` binding unused, so it did not compile (**BUILDERR**). It was
  rewritten to keep the binding and answer "no" — the behaviour under test.
- The first **M16** renamed the port's accessor to the kernel's verb name
  and missed the **test file's** call sites, so it did not compile
  (**BUILDERR**). It was rewritten to **add** the colliding method instead of
  renaming one — the rule is about what the port carries, not about how it
  got there.
- The second **M16** inserted the colliding method **in place of** the next
  function's signature and left that function's body behind, so the file did
  not parse (**BUILDERR**). It was rewritten to insert the method **beside**
  it.

## What the slice does not do

- It does not move `kernel/app/runs/run_execute.go`: the bus, the run verbs,
  the remote mirror and `PublishRemoteRunEvent(k *runtime.Kernel, …)` still
  hold the handle. The ratchet is at **1**, and that one file is named.
- It does not move the **run record's** own handle field.
- It does not move the **daemon's** run verbs: a run is still started through
  the kernel's own `RunWith`/`RunWithRetry`, and the runs family's **own**
  launch port already carries them for the two callers that need a run
  started — the roster's operator wake and the workboard's dispatch.
