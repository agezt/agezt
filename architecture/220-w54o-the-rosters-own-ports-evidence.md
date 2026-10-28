# W5.4o — the roster's own ports, and a guard that reads the kernel

## What the slice is

W5.4n took the **first** family of the port programme: the council's port-filling
moved out of the adapter and into the application, and a verb-grained guard was
put in place to hold it there.

This slice takes the **second** family — the roster — and, while doing it, fixes
two things the first slice's guard could not see.

The roster is the busiest family the adapter has: **eight** kernel verbs
(`RosterGet`, `RosterList`, `AddProfile`, `UpdateProfile`, `RemoveProfile`,
`SetProfileEnabled`, `SetProfileRetired`, `AgentImpact`) named in **28** call
sites across **seven** adapter files. Like the council's, all of it was port
filling: the adapter was handing kernel methods to `kernel/app/roster`'s port
structs and asking the application what to do with them.

So the roster's ports moved into the application:

```go
// kernel/app/roster/ports.go
type ProfilePorts struct {
	Get        func(ref string) (agentapi.Profile, bool)
	List       func() []agentapi.Profile
	Add        func(p agentapi.Profile) (agentapi.Profile, error)
	Update     func(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error)
	Remove     func(ref string) (bool, error)
	SetEnabled func(ref string, enabled bool) (agentapi.Profile, error)
	SetRetired func(ref string, retired bool, reason ...string) (agentapi.Profile, error)
	Impact     func(slug string) []string
}

func ProfilePortsFromKernel(k *runtime.Kernel) ProfilePorts
```

The adapter keeps one binding — `kernel/controlplane/roster_ports.go`:

```go
func (s *Server) rosterPorts() approster.ProfilePorts {
	return approster.ProfilePortsFromKernel(s.k)
}
```

**Twenty-eight** kernel verb call sites left the adapter's production code.
**Zero** roster verb names are left in it (the remaining ones are all in
`_test.go` files, where a test sets the world up directly).

## What moved

| | Before | After |
|---|---|---|
| who an agent is | `s.k.RosterGet` in 6 files | the application's `Get` port |
| who all the agents are | `s.k.RosterList` in 3 files | the application's `List` port |
| adding one | `s.k.AddProfile` | the application's `Add` port |
| editing one | `s.k.UpdateProfile` in 3 files | the application's `Update` port |
| removing one | `s.k.RemoveProfile` | the application's `Remove` port |
| pausing one | `s.k.SetProfileEnabled` in 2 files | the application's `SetEnabled` port |
| retiring one | `s.k.SetProfileRetired` in 3 files | the application's `SetRetired` port |
| what depends on one | `s.k.AgentImpact` | the application's `Impact` port |

The rows the roster manages are **contract rows** (`agentapi.Profile` is the
agents module's row, which is the contract's), so the port hands them over
without a conversion — the same identity W5.4m found for the council's members.

One subtlety worth naming: the per-request kernel (`host.kernel`, the routed
tenant kernel) is bound too, at the one place the adapter asks a *request-scoped*
question (`app_operations.go`, the skill curation's agent check). The adapter
still says one sentence about the roster: *here is the handle*.

## Two holes the first guard had

W5.4n's guard was a hand-written list of four verbs and a matcher that looked
for `"." + verb + "("`. Two things it could not do:

1. **It could not see a verb that left as a value.** `s.k.Council` handed over
   as a function is not a call — the old matcher missed it. A verb could leave
   the adapter by becoming a method value.
2. **Its list could rot.** Four hand-written names, checked against nothing: a
   name the kernel does not have guards nothing, and a verb added to the kernel
   would not be guarded until somebody remembered.

Both are fixed, and both are now held by tests of their own:

- The matcher sees a call **and** a method value, and it keeps a **boundary** —
  `CouncilDefaultMembers` is not `Council`, and a rule that cannot tell them
  apart flags a name it should not.
- The roster's verbs are **derived** from the kernel's own roster accessor file
  (`kernel/runtime/accessors_roster.go`), not written out. A roster verb added
  to the kernel is guarded the day it lands; a verb dropped from the kernel
  leaves the guard on its own.
- Two tests hold the derivation itself: every guarded verb must be a **real
  method of `*runtime.Kernel`** (a verb nobody has guards nothing), and every
  roster verb the kernel declares must be **named by the guard**.

The derivation is deliberately **not** trusted by the test that checks it: the
test reads the accessor file itself, so a helper that quietly drops a verb
cannot hide the drop from the check meant to catch it. That separation is what
the calibration run forced — the first version of the test used the helper, and
the mutation that truncated the helper's list **survived**.

## The proof

### Permanent tests — 5 new, 1 extended

| Test | What it holds |
|---|---|
| `TestTheProfilePortsReachTheKernelProfiles` (`kernel/app/roster`) | every one of the eight ports answers with the kernel's own answer — the roster adds, reads, edits, pauses, retires (with its reason), reports what depends on the agent, and removes it |
| `TestTheAdapterBindsTheRosterPortsToItsOwnKernel` (`kernel/controlplane`) | the binding itself: the agents the kernel holds are the agents the adapter's ports read, and they are the agents the daemon's roster list serves |
| `TestTheGuardNamesOnlyVerbsTheKernelHas` (`kernel/controlplane`) | no guarded verb is a verb the kernel does not have |
| `TestTheGuardNamesEveryRosterVerbTheKernelHas` (`kernel/controlplane`) | every roster verb the kernel declares is guarded — read independently of the helper the guard uses |
| `TestTheVerbGuardSeesAMethodValueAndKeepsItsBoundary` (`kernel/controlplane`) | the matcher sees a value, sees a call, and does not confuse one verb's name for another's |
| `TestTheAdapterCallsNoApplicationOwnedVerbOnTheKernel` (extended) | **the slice's product at verb granularity**: the host may not call the application's verbs on the handle |

### The mutation run — 13 mutations, 13 caught

Two independent passes with **byte-identical** logs (SHA-256
`0EE5DAE0A7A192A5F88012A36F316828893880772D456C6D1BAE78DA9A4C808E`; the
elapsed times, ULIDs, epoch stamps, addresses and goroutine ids are normalized,
because they are not the mutation's fingerprint).

| Mutation | What it breaks | Caught by |
|---|---|---|
| M01 | the `Get` port answers empty | the ports test |
| M02 | the `List` port answers empty | the ports test |
| M03 | the `Add` port does not persist | the ports test |
| M04 | the `Update` port does not edit | the ports test |
| M05 | the `Remove` port removes nothing | the ports test |
| M06 | the `SetEnabled` port does not pause | the ports test |
| M07 | the `SetRetired` port drops the reason | the ports test |
| M08 | the `Impact` port reports nothing | the ports test |
| M09 | the matcher loses its boundary — `CouncilDefaultMembers` is flagged as `Council` | the matcher's own test |
| M10 | the matcher sees calls only — a verb can leave the adapter as a **value** | the matcher's own test |
| M11 | the derivation drops a roster verb | the derivation test |
| M12 | the derivation reads the **wrong file** | the derivation test |
| M13 | the adapter's binding is empty — the ports answer from nowhere | the binding test |

M11 is the one worth reading twice: it **survived** the first calibration run.
The test that was supposed to catch it called the very helper the mutation
truncated, so the drop was invisible to it. The fix was to make the test read
the kernel's accessor file itself — the expectation must not be derived by the
same code path it is checking.

M12 is worth naming too: its first form was a **no-op mutation**. It changed a
guard that could never fire (the file it guarded is always read successfully),
so nothing changed and nothing failed. A mutation that changes nothing is not
a mutation — it was rewritten to point the derivation at the wrong file, which
is a real failure mode.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | OK — **2 allowlisted import violations and 0 allowlisted call sites remain** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | OK |
| `go run ./tools/docclaimscheck -base origin/main` | OK |
| `go run ./tools/docsindex` | OK |
| `staticcheck ./...` | clean |
| `go test ./... -count=1 -timeout=40m` | green |

## What the slice does not do

The **ratchet does not move yet**: the control plane still imports the runtime
in **15** files, because it still holds the kernel handle for families this
slice did not touch (schedule, standing, workboard, tools, journal, …).

But the port programme now has its shape and its **guard** in place, and the
guard is no longer a hand-maintained list: it reads the kernel. The next family
does not need a new rule — it needs the same one, filled in.

The two families that remain hardest are worth writing down while the shape is
fresh:

- **The schedule/standing/workflow family** reads the daemon's *libraries*
  (`k.Schedules()`, `k.Standing()`, `k.Workflows()`), which are **nil-safe by
  design** (W3.49, W3.50, W3.55): a host without one holds a nil port and the
  application sees "no schedules" rather than calling into nothing. That
  nil-port discipline is the pattern to keep, not to remove.
- **The selfrepair family** is a **module→module** edge (the agents module
  reaching the runs module's runtime), which the gate holds at package
  granularity. It needs a port in the agents module's own api package, and it
  is currently blocked by another session's uncommitted work in the same files.
