# W2.63 the reaper's findings have one declaration

The reaper finds what has gone stale — dead agents, degraded agents,
misconfigured agents, retry and routing pressure, forced chains that are
failing, and unstable chains. Every one of those findings was described
**twice**: once where the runtime finds it and once where the application
publishes it — and the application's copy was filled in **field by field**.

| the finding | the runtime's row | the application's row | the copy |
|---|---|---|---|
| a dead agent | `runtime.ReaperAgent` | `app/reaper.DeadRow` | three fields |
| a degraded agent | `runtime.DegradedAgent` | `app/reaper.DegradedRow` | ten fields |
| a misconfigured agent | `runtime.MisconfiguredAgent` | `app/reaper.MisconfiguredRow` | six fields |
| retry pressure | `runtime.RetryPressureAgent` | `app/reaper.RetryRow` | twelve fields |
| routing pressure | `runtime.RoutingPressureAgent` | `app/reaper.RoutingRow` | thirteen fields |
| a forced chain | `runtime.RoutingForced{Probation,Failed,Exhausted}Agent` — **three identical declarations** | `app/reaper.ForcedRow` | fourteen fields, three times |
| an unstable chain | `runtime.RoutingUnstableAgent` | `app/reaper.UnstableRow` | thirteen fields |

With this slice the findings are declared **once**, in the runtime, and the
application publishes them as they are.

## What moved

- **The application's seven row types are aliases of the runtime's seven
  rows.** `DeadRow`, `DegradedRow`, `MisconfiguredRow`, `RetryRow`,
  `RoutingRow`, `ForcedRow` and `UnstableRow` are now the runtime's rows, and
  the seven copy loops (about seventy field copies) are gone: the scan hands
  the runtime's lists over with a slice copy.
- **The three forced phases are one row.** `RoutingForcedProbationAgent`,
  `RoutingForcedFailedAgent` and `RoutingForcedExhaustedAgent` were three
  byte-identical declarations of the same row. There is one declaration now —
  `RoutingForcedAgent` — and the three old names are aliases of it, so the
  runtime's public API is unchanged and the report still keeps one list per
  phase.
- **The runtime's finding rows carry every field.** The optional fields (the
  name, the doctor, the escalation, the last reason, the chains) lost their
  `omitempty`: the findings now say what they are, and the empty ones are
  explicit rather than missing.

## What did not change

**No wire changed.**

- The **operator's scan** (`reaper_scan`, the Web UI's route and `agt reaper`)
  publishes the same keys, in the same order, with the same values. Every row
  still carries all of its fields, the unset lists are still `null` and an
  empty finding is still an empty array (`[]`), never `null`.
- The **introspection tool's** reaper view is a different, deliberately
  compact wire: the model sees the empty optional fields omitted. It keeps its
  own rows and its own copy — it is the tool's own view, not the operator's
  scan.
- The **runtime's rows were never marshalled** — the daemon's scan reaches the
  operator through the application's typed operation, and the roster's status
  snapshot and the overseer tool read the fields. Dropping `omitempty` from
  them therefore changes nothing that anyone can observe; it only makes the
  runtime's rows the same rows the application publishes.

## Runnable evidence

Permanent tests cover:

- the **identity** of every row that now shares a declaration: the
  application's seven rows are the runtime's seven rows, and the three forced
  phases are one row (`reflect.TypeOf` comparisons, so a copy that drifts is
  caught, not only a renamed field);
- the **wire** of every finding row in the runtime: each field is present,
  the empty ones included (`"name":""`, `"issues":null`, `"forced_chain":null`,
  `"routing_force_generation":0`, …);
- the **values that flow unchanged**: the scan's rows are the runtime's rows,
  with their counts, and the wire keeps the zero fields and the null lists
  (the pre-existing scan test, unchanged).

The mutations fail tests: **12 mutations, 12 caught** — eight renamed or
omitted wire keys across the finding rows (the dead agent's name, the
degraded agent's last reason, the misconfigured agent's issues, the retry
pressure's next attempt, the routing pressure's failed model, the forced
chain's chain and generation, the unstable chain's current chain), the copy
that copies nothing, the copy that adds a row, and the two scans that drop a
list.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (271 packages, 129 allowlisted import violations and 13
allowlisted call sites — the ratchet is unchanged), `tools/structure-md` (no
new package), `tools/changelog-lint`, `tools/docclaimscheck` and staticcheck.
The affected suites — the reaper operation, the roster's status snapshot, the
introspection tool, the overseer tool and the runtime's reaper tests — pass at
count=20.

## Not done here

The **introspection tool's reaper rows** keep their own declarations: the
tool's view is a compact wire for the model (the empty optional fields are
omitted), not the operator's scan. Unifying them would widen the model's view
with explicit zeros.

The **application's own artifact row** (`kernel/app/artifacts.Entry`) and the
**data-lake's records** (`kernel/datalake`, `kernel/app/data`) still have
their own declarations and their own spellings — they are their own wires.

The **execution profile's rows** (`kernel/app/execprofile` against
`kernel/executionprofile`) still have two declarations: the module's rows omit
the empty optional fields, the application's always carry every field.
