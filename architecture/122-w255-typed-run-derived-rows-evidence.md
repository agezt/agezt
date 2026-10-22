# W2.55 typed run-derived rows

The schedule firing views and the tenant activity summary no longer keep a
second copy of a run row. They read the application's own folded run rows,
and the control plane's two re-projections are gone.

## What changed

W2.52 moved the run **fold** into the application layer
(`kernel/app/runs/collect.go`), and the fold produces the application's own
`Run` row. Two consumers still re-projected that row into a copy of their
own:

- `kernel/controlplane/schedule_helpers.go` copied nine fields of
  `appruns.Run` into `appschedule.FiringRun`;
- `kernel/controlplane/tenant.go` copied six fields into `apptenants.Run`.

Both copies were lossy on purpose — the firing view and the tenant summary
counted only what they needed — but they were also a second definition of
"a run", in the host, next to the one the application owns. The host knew
which fields of an application row existed and which ones mattered.

Now:

- `appschedule.FiringRun` and `apptenants.Run` are **gone**;
- `kernel/app/schedule` and `kernel/app/tenants` read `appruns.Run` directly
  — the same row the runs view shows, so the three views cannot disagree
  about a run;
- the control plane passes the fold straight through: the schedule's firing
  reads and the tenant's activity reader are the fold's own result.

`kernel/app/schedule` and `kernel/app/tenants` now import `kernel/app/runs`.
That direction is safe: `app/runs` imports `app/roster`, `app/execprofile`
and the approval package, none of which import the schedule or the tenant
packages.

## The semantics are unchanged

Nothing about what the operator sees moved:

- the firing view's outcome precedence (completed > failed > abandoned >
  running), its durations, its spend, its answer preview, its failure
  reasons and its success rate;
- the tenant summary's counts (the runs, the completed, the failed, the
  active), its accumulated spend and its last activity.

The one thing that is *not* the same is the tenant summary's reading of a
run: it counted `Completed` before `Failed` before everything else, and the
folded row carries `Abandoned` as well. A run that was abandoned is still
counted as active, exactly as before — the summary never looked at
`Abandoned` — and the order of the map the fold returns does not matter for
a count, a sum or a maximum.

## The wire shape is unchanged

The firing views' and the tenant summary's outputs are byte-for-byte the
same: the existing suites in `kernel/app/schedule` and `kernel/app/tenants`
and the control plane's schedule and tenant suites pass unchanged.

## Runnable evidence

The permanent tests are the existing ones, strengthened where the shared row
type exposed a gap:

- the schedule's firing filters now cover the `failed`, the `abandoned` and
  the `running` outcomes — the status filter's branches for the failed and
  the abandoned outcomes had never been exercised, because the filter was
  only ever used with a completed run;
- the schedule's firing row for a run that failed after it started now
  asserts its duration, its reason, its spend and its answer preview — the
  failed run's duration had never been asserted.

With those two gaps closed, the mutations fail tests: the lost precedence of
the completed outcome, the unreported failed and abandoned outcomes, the
uncounted completed, failed and abandoned firings, the forgotten failure
reason, the unnamed failure reason, the unaccumulated spend, the wrong
success rate, the unreported completed status, the unmeasured durations, the
dropped answer preview, the unreported failed status in the latest view, and
the tenant summary's uncounted completed and failed runs, its
unaccumulated spend, its unmeasured last activity and its uncounted runs.
20 mutations, 20 caught.

The run was honest about the order: the first pass caught 18 of 20 and left
two open — the status filter's failed and abandoned branches were never
exercised, so the mutations escaped. The tests were strengthened and the
run repeated; the second pass caught 19 of 20 and left the failed run's
duration open, because no test asserted it. The test was strengthened again
and the third pass caught all 20. No equivalent mutation is recorded for this
slice.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (270 packages, 129 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md` (no new package).

## Not done here

The projections that are left in the control plane convert **runtime** types
— the council's `runtime.CouncilMember`, the governor's snapshot, the
runtime's plugin registration — into the application's row types. The
application layer cannot import `kernel/runtime` (the runtime's own tests
import the app layer), so those conversions stay where they are for now;
they are listed as the next candidates in `architecture/NEXT.md`.

The cloned-kernel harness of the earlier slices — the pre-slice handler and
the registered operation on two kernels, compared step by step — is not part
of this slice, as in W2.44b through W2.54. The schedule and tenant suites
are the parity proof here.
