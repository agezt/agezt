# W3.39e — the schedule and skill rows become contract's ports

**Date:** 2026-10-24
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the tool registry's **schedule row** and **skill row** — the last
two families whose ports already existed.

## The last two clean families

Unlike W3.39b–W3.39d, nothing had to be *designed* here: the ports the two
tools need were already in the contract layer. The registry simply did not
carry them.

```go
// plugins/builtintools/kernelbound.go — before
// schedule
if d.K == nil {
	return nil
}
if sched := d.K.Schedules(); sched != nil {
	st.Bind(sched)
	st.BindAgentLookup(d.K.Roster().Get)
}

// skill
if d.K == nil {
	return nil
}
if fg := d.K.Forge(); fg != nil {
	var bundles skillapi.BundleStore
	if b := fg.Bundles(); b != nil {
		bundles = b
	}
	sk.Bind(fg, bundles)
}
```

The schedule tool takes the cadence store through the contract's port
(`cadenceapi.Store`) and the skill tool takes the forge and its bundle store
through the contract's ports (`skillapi.Forge`, `skillapi.BundleStore`) — but
the **spec** reached them through the live kernel, so the registry's row had
to be the live kernel, and the row's type was a module's.

## What moved

- `kernel/toolreg`: `KernelDeps` gains three rows, all of them **existing**
  contract ports:
  - `Schedules cadenceapi.Store`
  - `SkillForge skillapi.Forge`
  - `SkillBundles skillapi.BundleStore`
- `plugins/builtintools`: the two specs bind the new rows instead of `d.K`.
- `cmd/agezt`: the daemon supplies them nil-safely (`schedulesPort`,
  `skillForgePort`, `skillBundlesPort`).

The roster lookup the schedule tool needs is the **same closure** the
standing tool already takes (W3.39d) — one row, two readers.

## The ratchet did NOT move — for the fifth time

**307 packages, 78 import violations, 13 call sites** — unchanged.

The registry's **rows** are still in the wrong layer, and the last
`plugin-reach` edge falls only when every field of every row is a contract
row or a contract port. This slice removes the **fourth** of the five
families.

## The finding: a rule nobody measured

The mutation run found a **survivor**, and it was not a weak mutation — it
was a real rule with no measurement:

> *"the bundle store port stops being nil-safe"* — removing the
> `if b == nil { return nil }` check from `skillBundlesPort` changed
> nothing.

The reason is that the bundle store is **optional**: the forge's own doc
says it — *"nil leaves skills body-only"* — and `SetBundles` wires it when
the daemon wants bundles. So a daemon without bundles hands over a nil
`*skill.BundleStore`, and without the check that nil pointer becomes a
**non-nil interface**: the skill tool would bind a bundle store that is not
there and panic on the first resource read.

The rule was real and reachable — but **no test walked that path**, because
every test ran a daemon that had bundles. That is the measurement gap this
project keeps naming: *"a path nobody walks is not a path."*

So the fix was **a test, not a weaker mutation**: the daemon's test now
unwires the bundles (`k.Forge().SetBundles(nil)`) and asserts that the port
stays a **nil** port — the check on the concrete type, because a nil
`*skill.BundleStore` handed to an interface parameter is already a non-nil
interface.

## The proof

### Permanent tests

- **The plugin** (`plugins/builtintools/schedule_skill_ports_test.go`):
  without the ports the schedule and skill tools say they are unavailable —
  they do not call into nothing.
- **The daemon** (`cmd/agezt/tooldeps_test.go`): the composition root
  supplies all three ports, they **are the live ones** (the cadence store is
  the kernel's store, the forge is the kernel's forge, the bundle store is
  the forge's), and they stay **nil** ports when there is no kernel — and
  the bundle store stays a nil port when the forge has no bundles.
- **The re-pointed existing tests** (`skill_ports_test.go`,
  `trigger_ports_test.go`): they measured the live wiring and reached the
  live implementation through `d.K`; the ports now arrive in their own rows,
  so the tests supply them there. What they measure — the live store really
  carrying the live implementation — is unchanged, and their assertions are
  unchanged.

### Mutation run

**9 mutations: 9 caught.** No mutation survived (after the measurement gap
above was closed), none produced a compile error, and none was skipped on a
stale anchor.

| # | Mutation | Result |
|---|---|---|
| M01 | the schedule spec does not bind the store | caught |
| M02 | the schedule spec drops the agent lookup | caught |
| M03 | the skill spec does not bind the forge | caught |
| M04 | the skill spec binds a nil bundle store | caught |
| M05 | the daemon does not supply the cadence store port | caught |
| M06 | the daemon does not supply the forge port | caught |
| M07 | the daemon does not supply the bundle store port | caught |
| M08 | the cadence store port stops being nil-safe | caught |
| M09 | the bundle store port stops being nil-safe | caught — **after the gap was closed** |

M09 was the survivor that opened the gap; the test that closed it is the
bundle-port nil test above.

### Gates

The build, gofmt, vet, dead-code, archcheck (**307 packages, 78/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so is
the full test suite.

## What is deliberately left — one family and the final move

1. **the forge row and the platform rows** — `forgetool.Kernel` names the
   toolforge rows plus `approval.Decision`, which is a **platform** type
   (L2): the port can move only after that row moves with it, and moving it
   means moving the decision row to the contract layer with an alias. The
   **warden**, the **bus** and the **journal** are platform types too: the
   bus and the journal carry `event.Event`, which cannot move to the
   contract layer (it hashes with blake3), so those two need an **adapter**
   rather than an alias.

When the last of them is gone, every field of every row is a contract row or
a contract port — and the registry's rows can move to the contract layer,
which is where the last `plugin-reach` edge falls with them.
