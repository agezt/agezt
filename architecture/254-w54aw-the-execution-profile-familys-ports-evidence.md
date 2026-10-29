# W5.4aw — the execution profile family, and the warden the profiles run through

## What the slice is

The **thirty-sixth** slice of the port programme, and the **twenty-seventh**
one of its last phase: the handle itself.

The third slice of that phase whose subject is the **application's** handle,
and the first one where the file that held the handle was not an adapter at
all but a **helper**: `kernel/app/execprofile/inventory.go` read the daemon's
tools and the daemon's warden to build the picture of where work can run.

## What the slice found

The execution profile family's reads are **two**:

| read | the family that owns it |
|---|---|
| `Tools()` | the tools |
| `Warden()` | **nobody** — until this slice |

The warden's verb was **unguarded**: no family claimed it, so the adapter was
free to name it, and nothing in the tree said that the execution profile
family's business was the application's.

The measurement that moved:

| | before | after |
|---|---|---|
| application files that name the handle **outside** a port file | **8** | **7** |
| application-owned verbs the guard holds | **161** | **162** |
| the adapter's call sites on the handle | **0** | **0** |
| control-plane production files that name the runtime | **12** | **12** |

## What changed

The execution profile family's ports moved into the application
(`kernel/app/execprofile/ports.go`):

```go
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{
			Inventory: func() executionprofile.Inventory { return executionprofile.Build(executionprofile.Options{}) },
			Policy:    executionprofile.PolicyFromEnv,
		}
	}
	return Ports{
		Inventory: func() executionprofile.Inventory { return buildInventory(k) },
		Policy:    executionprofile.PolicyFromEnv,
	}
}
```

and `inventory.go` keeps the one thing that was never the kernel's: the
tool-name list, which is a pure function of a map.

The adapter asks for the ports instead of filling them by hand
(`kernel/controlplane/app_operations.go`):

```go
return appexecprofile.New(appexecprofile.PortsFromKernel(k))
```

The runs family — which holds the handle for its own reasons — asks the
execution profile family for the same picture
(`kernel/app/runs/run_profile.go`):

```go
appexecprofile.PortsFromKernel(k).Inventory()
```

### The empty host is not the daemon with no tools

A host with **no daemon** holds no tools and no warden, and the port answers
the **empty picture**: the profiles that need a tool are unsupported, and the
ones that do not are still there. That is the truth about a host with no
daemon — and it is **not** the same answer as a daemon with no tools, which
is a different picture entirely (the host's operating system is still
reported, and the profile that needs no tool is still there).

The remote backends — SSH, Kubernetes, Modal, Daytona — come from the
**host's environment**, not from the kernel: they are not the daemon's
business, and the port says so.

## The rule

The execution profile family's rule is **by name**: a kernel verb whose name
mentions the warden is the execution profile family's business. It catches
**one** verb today: `Warden`.

The rule does **not** reach for the tools, even though the family reads them:
the tools are the **tools** family's business, and a rule that reached for
them would make two families claim the same verb — which is exactly what the
sharp test refuses.

The rule is held in **both** directions:

- **broad** — the guard names the warden's verb, and the execution profile's
  port names no verb the guard misses (read from the port's **own source**).
- **sharp** — the execution profile family's list claims no other family's
  verb, and it does not claim the whole kernel.

## The tests

**6 permanent tests** — three in the execution profile's own package, three
in the guard:

- `TestTheExecutionProfilePortsReachTheDaemon` — the inventory really reads
  the daemon: the local profile sees the daemon's tools, and it is routable.
- `TestTheExecutionProfilePortsAnswerAnEmptyHostWithoutADaemon` — the empty
  half: no tools, no routing, and the host still knows its own operating
  system.
- `TestTheExecutionProfilePortNamesAreNotTheKernelVerbs` — the port's fields
  and methods are not the kernel's verb names.
- `TestTheExecutionProfileFamilyClaimsNoOtherFamilysVerbs` — the sharp half.
- `TestTheGuardNamesEveryExecutionProfileVerbTheKernelHas` — the broad half.
- `TestTheExecutionProfileNamesNoKernelVerbTheGuardMisses` — the port names no
  verb the guard misses.

The application's ratchet moved with the slice:
`applicationHandleNamingFiles` is **8 → 7**.

## The mutations

**9 mutations, 9 caught**, in two independent passes with **byte-identical**
logs (SHA-256 `E59E66642FE5342686E07C041720925627B093C5CCF8642439A6901670AF7B2C`):

- **M01** the port fills nothing — the inventory reports a host with no tools.
- **M02** a host with no daemon asks the daemon anyway — the nil check is
  gone, and the port panics instead of answering.
- **M03** the port does not see the daemon's tools — the inventory is built
  with no tools at all.
- **M04** the guard forgets the warden — the family claims the tools'
  verbs instead, and the sharp test refuses it.
- **M05** the handle walks back into the service — and the **application
  ratchet** catches it.
- **M06** the adapter fills the port with nothing — and the control plane's
  own profile surface reports a host with no routable profile.
- **M07** the SSH backend is dropped — the host's own SSH configuration
  stops reaching the inventory.
- **M08** the empty host answers a lie — the inventory claims there is no
  host at all, not even an operating system.
- **M09** the Kubernetes backend is dropped.

**One** mutation did not compile on the first run (the adapter's kernel
variable became unused) and was **rewritten**, not excused.

**One** mutation **survived** the first run, and it was the programme's own
lesson again: it dropped the **warden**, and the inventory's builder falls
back to a default warden when there is none — the default answers the same,
so the mutation produced **no defect** and measured nothing. It was rewritten
as the Kubernetes backend, which the host's environment really decides. A
mutation that does not produce a wrong behaviour is not a measurement.

## What the slice does not do

- It does not move the **other** seven files that still hold the handle: the
  runs family (four files), the overseer, the introspector and the guardians.
  The ratchet is at **7**, and every one of those is the next slice's
  business.
- It does not move the **runs family's** handle: that family holds it for
  the roster, the model, the bus and the run verbs, and it is the biggest
  piece of what is left.
