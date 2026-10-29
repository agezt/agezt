# W5.4av — the config tool's family, and the second file off the handle

## What the slice is

The **thirty-fifth** slice of the port programme, and the **twenty-sixth** one
of its last phase: the handle itself.

W5.4au moved the biggest file off the application's handle — the daemon's own
**overview**. This slice takes the next one, and it is a different shape: the
config tool's adapter is the file that lets a **plugin** reach the live daemon
without naming the runtime, and it held the runtime **to do that**.

## What the slice found

`kernel/app/configtool/source.go` is the app-layer adapter behind the config
tool's contract port (`kernel/contract/configapi`). It held the handle and
named **two** kernel verbs with it:

| read | the family that owns it |
|---|---|
| `UpdateProfile()` | the roster (`../runtime/accessors_roster.go`) |
| `Reload()` | the catalog (the "Reload" word rule) |

**Both verbs were already claimed.** The config tool's family owns no kernel
verb of its own: it reads the roster's and the catalog's, and the guard says
so already. So this slice needed **no new family and no new rule** — the
sharp guard test, which fails when two families claim the same verb, would
have refused it.

The measurement that moved:

| | before | after |
|---|---|---|
| application files that name the handle **outside** a port file | **9** | **8** |
| the adapter's call sites on the handle | **0** | **0** |
| control-plane production files that name the runtime | **12** | **12** |
| application-owned verbs the guard holds | **161** | **161** |

## What changed

The config tool's ports moved into the application
(`kernel/app/configtool/ports.go`):

```go
type Ports struct {
	profile func(string, func(*agentapi.Profile)) (agentapi.Profile, bool, error)
	reload  func() error
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{
		profile: k.UpdateProfile,
		reload: func() error {
			_, _, err := k.Reload()
			return err
		},
	}
}
```

and the source now holds the **ports**:

```go
type kernelSource struct {
	ports Ports
}

func NewKernelSource(ports Ports) configapi.Kernel {
	if !ports.Ready() {
		return nil
	}
	return &kernelSource{ports: ports}
}
```

### The port's names are not the kernel's

The port answers `EditProfile` and `Rebuild` — **never** `UpdateProfile` and
`Reload`, which are the kernel's own verb names. This is the **sixth** time
this programme learned that lesson, and the test that holds it is a plain
reflection over the port's method set and the kernel's.

### The refusal is written once

The source used to check the handle for nil and hand the tool a **nil port**
— a rule the composition root also spelled, in `cmd/agezt/tooldeps.go`. Now
the ports say "no daemon" themselves (`ErrNoDaemon`), the source asks the
ports whether they are ready, and the composition root does not spell the
refusal again:

```go
func configSource(k *runtime.Kernel) configapi.Kernel {
	return configtool.NewKernelSource(configtool.PortsFromKernel(k))
}
```

## The rule

**No new rule.** The config tool's port names two kernel verbs, and both
already belong to families that declare their own ports:

- `UpdateProfile` — the roster family's file rule (`../runtime/accessors_roster.go`).
- `Reload` — the catalog family's word rule.

A family that claims a verb another family owns is exactly what the sharp
guard test refuses, and the slice would have failed it. What the slice adds
is the **broad** half for this family: the config tool's port names no verb
the guard misses, read from the port's **own source** (`k.<Verb>`) and not
through the helper the guard runs.

## The tests

**4 permanent tests** — three in the config tool's own package, one in the
guard:

- `TestTheConfigToolPortsReachTheDaemon` — the port really reaches the
  daemon: the edit is stored in the roster, and the rebuild runs.
- `TestTheConfigToolPortsRefuseForAHostWithNoDaemon` — the empty half: the
  port is not ready, the edit and the rebuild refuse with `ErrNoDaemon`, and
  the tool is handed a **nil** port.
- `TestTheConfigToolPortNamesAreNotTheKernelVerbs` — the port's method set and
  its field names are not the kernel's verb names.
- `TestTheConfigToolNamesNoKernelVerbTheGuardMisses` — the port names no verb
  the guard misses.

The application's ratchet moved with the slice:
`applicationHandleNamingFiles` is **9 → 8**.

## The mutations

**11 mutations, 11 caught**, in two independent passes with **byte-identical**
logs (SHA-256 `5C6EDEB75DD805ABBD6038F18FF8475D12CA05C60F122C3EC2327C371655D4EE`):

- **M01** the port fills nothing — the tool would refuse to do its work while
  looking perfectly wired.
- **M02** a host with no daemon asks the daemon anyway — the nil check is
  gone, and the port panics instead of answering.
- **M03** the refusal is swallowed — the edit answers "no error" for a daemon
  that is not there.
- **M04** the port carries a kernel verb's name — `EditProfile` becomes
  `UpdateProfile`.
- **M05** the handle walks back into the source — and the **application
  ratchet** catches it.
- **M06** the source hands the tool a port that calls into nothing — the nil
  port rule is gone.
- **M07** the port edits nothing — the answer looks right and the roster was
  never touched.
- **M08** the guard forgets the reload — the catalog family stops claiming
  `Reload`, and the port names a verb nobody guards.
- **M09** the readiness lies — a host with no daemon reports a daemon.
- **M10** the edit refuses nothing — the empty port calls into nothing.
- **M11** the rebuild refuses nothing.

**One** mutation did not compile on the first run, and a mutation that does
not compile measures nothing: it was **rewritten**, not excused — the rename
had to reach the source and both of the test's call sites.

## What the slice does not do

- It does not move the **other** eight files that still hold the handle: the
  runs family (four files), the overseer, the introspector, the execution
  profile and the guardians. The ratchet is at **8**, and every one of those
  is the next slice's business.
- It does not give the config tool a family of its own in the guard: the two
  verbs it reads belong to the roster and the catalog, and a second family
  claiming them is what the sharp test refuses.
