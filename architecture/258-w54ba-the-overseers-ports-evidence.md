# W5.4ba — the overseer, and the two verbs that belonged to nobody

## What the slice is

The **fortieth** slice of the port programme, and the **thirty-first** one of
its last phase: the handle itself.

The seventh slice of that phase whose subject is the **application's** handle,
and the first one whose file reached for **eighteen** verbs at once: the
overseer supervises the LIVE system — it lists the fleet, reads what a change
would affect, pauses and retires agents, cancels a run, wakes an agent, repairs
one, and halts or resumes the daemon itself.

## What the slice found

The overseer's reads belong to **six** families:

| read | the family that owns it |
|---|---|
| the fleet: list, get, edit, create, remove, pause, retire, impact | the roster |
| the daemon's gate: the halt and the resume | the daemon |
| the correlation a wake runs under | the correlation |
| the run a wake starts | the runs |
| the fleet scan the repair reads | the reaper |
| the live provider the routing chains live in | the provider |

And **two** verbs belonged to **nobody**:

| verb | before | after |
|---|---|---|
| `CancelRun` | unguarded | the overseer's |
| `ActiveRunIDs` | unguarded | the overseer's |

Those two were free for the adapter to name: no family claimed them, so
nothing in the tree said that the supervision of the live fleet was the
application's business. The **overseer** is the only family that reads them,
so the overseer owns them — and the rule is **by exact name**, the way the
daemon family's seven names are, because a word rule would sweep in the runs
family's own verbs.

The measurement that moved:

| | before | after |
|---|---|---|
| application files that name the handle **outside** a port file | **4** | **3** |
| application-owned verbs the guard holds | **162** | **164** |
| the adapter's call sites on the handle | **0** | **0** |
| control-plane production files that name the runtime | **12** | **12** |

## What changed

The overseer's ports moved into the application
(`kernel/app/overseer/ports.go`) — eighteen capabilities, all held
**unexported**, and all held as **functions**.

Four of them are other families' capabilities as a **whole**, and they are
asked of the families that own them, so a verb is never named twice:

```go
correlation := appcorrelation.PortsFromKernel(k)
provider := appprovider.PortsFromKernel(k)
reaper := appreaper.PortsFromKernel(k)
runs := appruns.PortsFromKernel(k)
```

The source, the operations and the chains now hold the ports and nothing else
(`kernel/app/overseer/source.go`, `ops.go`, `chains.go`), and the composition
root asks for the ports (`cmd/agezt/tooldeps.go`, `cmd/agezt/main.go`).

### The narrow view stays the consumer's

The task model chains are the **provider's** business, and the port carries the
live provider: the narrow view over it — the chains and nothing else — is the
**consumer's** business, and the consumer declares it.

The first cut of this slice asked the provider family's port for the chains
directly, and it **broke the routing**: the provider port narrows to the
provider's full chain view (the task chains AND the fallback chains), while the
overseer only ever needed the task chains — so a provider that carries the
task chains and no fallback chains stopped answering, and the routing update
answered "found no current chain". The consumer's own narrow view was put back.

### The refusal is written once

A host with **no daemon** holds no daemon to supervise. The **reads** answer
with nothing — no fleet, no impact, no runs — and the **writes** refuse with
the one refusal the port writes down (`ErrNoDaemon`): a supervisor that
answered "edited" about a daemon that does not exist would be lying.

## The rule

The overseer's rule is **by exact name**, and it answers to **two** names:
`CancelRun` and `ActiveRunIDs`.

It cannot be a word rule: a "Run" rule would take the **runs** family's own
verbs — the two that start a run — and a family that claims another family's
verb is exactly what the sharp test refuses.

The rule is held in **both** directions:

- **broad** — the guard names the two verbs, and the overseer's port names no
  verb the guard misses (read from the port's **own source**).
- **sharp** — the overseer family's list claims no other family's verb, and it
  does not claim the whole kernel.

## The tests

**3 permanent tests** in the overseer's own package, plus **three** in the
guard:

- `TestTheOverseerPortsReachTheDaemon` — the filled half: the port really
  reads and really writes the daemon's fleet (create, find, edit, pause,
  retire, delete), really reaches the daemon's gate (halt and resume), and
  really mints a correlation.
- `TestTheOverseerPortsRefuseForAHostWithNoDaemon` — the empty half: the
  reads answer with nothing and every write refuses with the one refusal.
- `TestTheOverseerPortNamesAreNotTheKernelVerbs` — the port's fields and
  methods are not the kernel's verb names.
- `TestTheOverseerFamilyClaimsNoOtherFamilysVerbs` — the sharp half.
- `TestTheGuardNamesEveryOverseerVerbTheKernelHas` — the broad half, in the
  test's own words.
- `TestTheOverseerNamesNoKernelVerbTheGuardMisses` — the port names no verb
  the guard misses.

The application's ratchet moved with the slice:
`applicationHandleNamingFiles` is **4 → 3**.

## The mutations

**13 mutations, 13 caught**, in two independent passes with **byte-identical**
logs (SHA-256 `3280102337742F651960A62CB7CE4258ABA0BD54DE4A6DEF0631F090109757A7`):

- **M01** the port fills nothing — the supervisor steers nothing while looking
  perfectly wired.
- **M02** a host with no daemon asks the daemon anyway.
- **M03** the port sees no fleet.
- **M04** the port finds no agent.
- **M05** the port does not pause the agent.
- **M06** the port does not retire the agent.
- **M07** the port does not delete the agent.
- **M08** the port does not halt the daemon.
- **M09** the port does not resume the daemon.
- **M10** the port cancels a run that does not exist.
- **M11** the port mints no correlation.
- **M12** the handle walks back into the source — and the **application
  ratchet** catches it.
- **M13** the port holds no live provider — the routing chains would be
  unreachable.

## What the slice does not do

- It does not move the **other three** files of the runs family:
  `run.go` (the run record's own handle field), `run_execute.go` (the bus, the
  run verbs and the remote tool) and `run_resolve.go` (the roster, the model,
  the correlation and the admission). The ratchet is at **3**.
- It does not move the **run record's** own handle field: the `Run` struct
  holds the kernel by name, and every capability that needs it has to be
  decided first.
