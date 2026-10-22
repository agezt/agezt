# W2.56 typed roster-profile and governor-snapshot projections

Two more of the control plane's re-projections are gone. What they mapped
was the host's own data into an application row — and the host knew the
application row's fields, which is the thing this slice removes.

## What moved

- `kernel/app/routing/agents.go` (new) gains `Agents(profiles []roster.Profile) []Agent`:
  the roster's profiles become the routing view's agents (the handle the
  operator addresses, the model the agent runs and its ordered fallback
  chain);
- `kernel/app/budget/snapshot.go` (new) gains `SnapshotOf(snap governor.BudgetSnapshot) Snapshot`:
  the governor's counters become the budget view's snapshot (the day's
  counters and each configured per-task-type cap with today's spend against
  it).

The control plane's two inline conversions — in `kernel/controlplane/app_operations.go`
— now call the application:

```go
Agents: func() []approuting.Agent {
    return approuting.Agents(s.k.Roster().List())
},
```

```go
func (b budgetGovernor) Snapshot() appbudget.Snapshot {
    return appbudget.SnapshotOf(b.g.Snapshot())
}
```

The host still reads the host's own data — the roster's profiles, the
governor's counters — and hands it over. The application's row stays the
application's business.

The two new imports are safe in this direction: `kernel/app/routing` now
imports `kernel/roster` (a module package that imports no app package) and
`kernel/app/budget` now imports `kernel/governor` (a platform package that
imports no app package). The archcheck ratchet is unchanged: 129 allowlisted
import violations and 13 allowlisted call sites, 270 packages.

## The semantics are unchanged

Both conversions move verbatim:

- the agent's slug, its model and its fallback chain, in the roster's
  order, with an empty roster mapping to an empty (not nil) agent list;
- the day's UTC date, the spent total, the ceiling, the strict-pricing flag
  and each per-task row's type, spend and cap, with no per-task budgets
  mapping to an empty (not nil) list.

## The wire shape is unchanged

The routing view's and the budget view's outputs are byte-for-byte the same:
the existing suites in `kernel/app/routing` and `kernel/app/budget` and the
control plane's routing, budget, agent and run suites pass unchanged.

## Runnable evidence

Permanent tests cover, at the application layer:

- the agent mapping: the slug, the model, the ordered fallback chain, the
  agent without a chain, and the empty roster;
- the snapshot mapping: the date, the spent total, the ceiling, the
  strict-pricing flag, the per-task rows (the type, the spend and the cap),
  and the governor without per-task budgets.

The mutations fail tests: the dropped slug, the name used as the slug, the
altered slug, the dropped model, the dropped fallback chain, the skipped
profile, the empty roster mapped to nothing, the dropped date, the dropped
spent total, the dropped ceiling, the dropped strict-pricing flag, the
dropped per-task type, the dropped per-task spend, the dropped per-task
cap, the skipped per-task row and the empty per-task list mapped to
nothing. 16 mutations, 16 caught. No equivalent mutation is recorded for
this slice.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (270 packages, 129 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md` (no new package).

## Not done here

The projections that are left in the control plane convert **runtime** types
— the council's `runtime.CouncilMember` and the runtime's plugin
registration — into the application's row types. The application layer
cannot import `kernel/runtime` (the runtime's own tests import the app
layer), so those conversions stay where they are for now; they are listed
as the next candidates in `architecture/NEXT.md`.

The cloned-kernel harness of the earlier slices — the pre-slice handler and
the registered operation on two kernels, compared step by step — is not part
of this slice, as in W2.44b through W2.55. The routing and budget suites are
the parity proof here.
