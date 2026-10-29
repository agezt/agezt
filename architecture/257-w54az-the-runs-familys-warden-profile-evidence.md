# W5.4az — the runs family's warden profile, and the first of its four files

## What the slice is

The **thirty-ninth** slice of the port programme, and the **thirtieth** one of
its last phase: the handle itself.

The sixth slice of that phase whose subject is the **application's** handle,
and the first one that takes one of the **runs family's** four files off it.
The runs family is the biggest piece of what is left — it holds the handle for
the roster, the model, the bus, the provider, the catalog, the tools and the
run verbs — and this slice does not pretend to move it in one step. It takes
the **narrowest** of the four files: `kernel/app/runs/run_profile.go`, the
file that resolves a warden-family execution profile and layers the sandbox
override onto the context.

## What the slice found

The file reached for the handle for **two** things: the execution profile's
**inventory** (which profiles the run tools route) and the **warden's** own
answer (whether the container profile really runs isolated).

The inventory was already asked of the **execution profile family** — the
family owns the warden, and the guard says so by name (W5.4aw). The warden's
answer was not: it was reached for through the handle, one line below a call
that already asked the family.

So the family that owns the warden now owns the warden's **answer** too, and
the file asks the family instead of the handle.

The measurement that moved:

| | before | after |
|---|---|---|
| application files that name the handle **outside** a port file | **5** | **4** |
| application-owned verbs the guard holds | **162** | **162** |
| the adapter's call sites on the handle | **0** | **0** |
| control-plane production files that name the runtime | **12** | **12** |

## What changed

The execution profile family's port gained a third capability
(`kernel/app/execprofile/exprofile.go` — the port's own declaration):

```go
type Ports struct {
	Inventory func() executionprofile.Inventory
	Policy    func() executionprofile.ProfilePolicy

	// EffectiveProfile answers the profile the daemon's warden really runs
	// for a requested one.
	EffectiveProfile func(warden.Profile) warden.Profile
}
```

and the run profile file takes the family's **ports** and not the handle they
were filled from (`kernel/app/runs/run_profile.go`):

```go
func WardenProfile(ctx context.Context, ports appexecprofile.Ports, id string) (context.Context, string, error) {
	p, ok := executionprofile.WardenProfileForRun(id)
	if !ok {
		return ctx, "", fmt.Errorf("execution profile %q is not routable for run tools yet (supported: %s)",
			id, strings.Join(executionprofile.RoutableRunProfileIDsFor(ports.Inventory()), ", "))
	}
	if p == warden.ProfileContainer && ports.EffectiveProfile(p) != warden.ProfileContainer {
		return ctx, "", fmt.Errorf("execution profile %q requires an active container backend (...)", id)
	}
	return warden.WithProfileOverride(ctx, p), string(p), nil
}
```

The two callers hand over the ports:

- the runs family's own resolve path
  (`appexecprofile.PortsFromKernel(k)` — that file still holds the handle for
  the roster, the model, the correlation and the admission, and it is the next
  slice's business);
- the control plane's workboard dispatch
  (`appruns.WardenProfile(ctx, appexecprofile.PortsFromKernel(s.k), id)`) —
  the adapter now hands the application **ports**, and the handle the ports
  were filled from stops at the binding.

### The conservative answer

A host with **no daemon** holds no warden, and the port answers the
**conservative** one: no isolation. That is the ground of the container
refusal: a run that asked for the container profile is then **refused** rather
than silently unisolated.

## The rule

**No new family and no new rule**: the warden belongs to the execution profile
family, and that family's rule is by name — a kernel verb whose name mentions
the warden is the execution profile family's business. The port now names the
warden twice (the inventory and the answer), and both are guarded.

What the slice does **not** add is a rule of its own: the run profile file
names no kernel verb at all any more.

## The tests

**2 permanent tests** in the execution profile's own package, and the four
existing run profile tests were rewritten (they hand over the ports now):

- `TestTheExecutionProfilePortsAnswerTheWarden` — the filled half: the port
  really answers with the daemon's warden, profile by profile.
- `TestTheExecutionProfilePortsAnswerAnEmptyHostWithoutAWarden` — the empty
  half: a host with no daemon holds no warden, and the answer is no
  isolation.

The application's ratchet moved with the slice:
`applicationHandleNamingFiles` is **5 → 4**.

## The mutations

**7 mutations, 7 caught**, in two independent passes with **byte-identical**
logs (SHA-256 `352A0061F3648213904E0DCB8F012096A6B93D54007BD0A48481480F32BA4E0E`):

- **M01** the port answers the requested profile — the daemon's downgrade is
  ignored, and the container profile runs unisolated.
- **M02** a host with no daemon answers the requested profile.
- **M03** the run path compares against the wrong profile.
- **M04** the run path inverts the container check.
- **M05** the refusal loses the supported profiles.
- **M06** the handle walks back into the run profile — and the **application
  ratchet** catches it.
- **M07** the port has no warden for a host with no daemon.

**Two** mutations had to be rewritten, and both rewrites are the programme's own
lessons again:

- The first **M03** made the port answer "no isolation" for every profile, and
  it **survived**: on a host whose daemon downgrades every profile anyway, the
  mutation produced **no defect** and measured nothing. (The downgrade is the
  platform's: only Linux keeps the namespace profile, and the daemon here is
  not Linux.) It was rewritten as the run path comparing against the **wrong**
  profile, which is observable on every host — a mutation that does not
  produce a wrong behaviour is not a measurement.
- The first **M05** removed the only use of the `strings` import, so the
  mutation did not compile. It was rewritten to join an **empty** list: the
  message loses the supported profiles and the import stays used. A mutation
  that does not compile has not measured anything either.

## What the slice does not do

- It does not move the **other three** files of the runs family:
  `run.go` (the run record's own handle field), `run_execute.go` (the bus, the
  run verbs and the remote tool) and `run_resolve.go` (the roster, the model,
  the correlation and the admission). The ratchet is at **4**.
- It does not move the **overseer's** handle: the overseer supervises the live
  fleet and reads **eighteen** verbs across the roster, the daemon, the runs,
  the correlation, the provider and the reaper — and two of them, `CancelRun`
  and `ActiveRunIDs`, belong to **nobody** today. The family that reads an
  unowned verb owns it, so the overseer will claim those two **by name**, and
  the guard will say so in both directions.
