# W2.48 typed agent teardown orchestration

The agent teardown's **orchestration** now lives in the application layer, in
`kernel/app/roster/teardown.go` beside the remove, retire and set-enabled
services that use it. The control plane keeps only the host's data access.

Three native files are gone:

- `kernel/controlplane/roster_teardown.go` (the orchestration itself — the
  file stays, but only as the host's store bindings)
- `kernel/controlplane/roster_crud_internal.go` (12 lines — a one-line
  wrapper around the app's own error builder)
- `kernel/controlplane/roster_activity_text.go` (a header-only file left
  behind by an earlier split, with no code at all)

## What moved

The typed `agent_remove`, `agent_set_retired` and `agent_set_enabled`
operations were already in the application layer: they own the decision
(remove the agent's standing orders too? retire the sub-agents? count what is
paused?). What they did not own was the **orchestration** behind those ports —
which standing orders and schedules belong to the agent, what a teardown
retires, pauses or removes, and in which order.

`kernel/app/roster/teardown.go` (163 lines) owns it now:

```go
func RetireSubagents(ports TeardownPorts, parent string, children []core.Profile, on bool) (int, []string, error)
func RemoveStandingOrders(ports TeardownPorts, slug string, on bool) (int, error)
func PauseStandingOrders(ports TeardownPorts, slug string) (int, error)
func CountPausedStandingOrders(ports TeardownPorts, slug string) int
func RemoveSchedules(ports TeardownPorts, slug string, on bool) (int, error)
func PauseSchedules(ports TeardownPorts, slug string) (int, error)
func CountPausedSchedules(ports TeardownPorts, slug string) int
```

The host binds the stores:

```go
type TeardownPorts struct {
    RetireChild        func(slug, reason string) error
    Standing           func() []standing.Order
    RemoveStanding     func(id string) (bool, error)
    SetStandingEnabled func(id string, enabled bool) error
    Schedules          func() []cadence.Entry
    RemoveSchedule     func(id string) (bool, error)
    SetScheduleEnabled func(id string, enabled bool) (bool, error)
}
```

## The semantics are unchanged

The orchestration is moved verbatim:

- a teardown retires the sub-agents that are not already retired, with the
  reason `parent/owner <parent> removed`, pauses what they still wake (the
  standing orders and the schedules), and returns the retired slugs sorted;
- removing the agent's wake sources removes the standing orders and the
  schedules whose agent matches (case-insensitively), counting only the ones
  the store actually had;
- pausing touches only the **enabled** orders and schedules of that agent;
- counting the paused ones counts the **disabled** ones.

The switch semantics are preserved too: with the switch off, nothing is
retired, removed or paused — the operations answer zero.

## The wrapper the move left behind

`roster_crud_internal.go` existed only to wrap the application's own
`ManagedDirectCallError` in a native function. The three call sites (the
schedule admission, the standing service and the workboard dispatcher) now
call the application directly, and the file is gone.

`roster_activity_text.go` held a package clause and a provenance comment and
nothing else — a split that left the file behind. It is gone too.

## The wire shape is unchanged

The remove, retire and set-enabled operations keep their answers, their
counts and their journal arc: the application already owned the decision, and
the orchestration still answers the same questions with the same rules. The
existing end-to-end suite — which drives the real operations over a real
kernel — passes unchanged.

## Runnable evidence

Permanent tests cover, at the application layer:

- the sub-agent retirement: the retired children, the reason, the pause of
  their wake sources, the sorted slugs, the already-retired children left
  alone, the switch off, and the store's error;
- the standing orders: the removal of the agent's orders (case-insensitively,
  another agent's order left alone), the counting of only what existed, the
  switch off, the pause of only the enabled orders, and the count of the
  paused ones;
- the schedules: the same five cases;
- the store's error stops the pause and reports nothing paused.

At the control plane, the existing roster suite passes unchanged.

The mutations fail tests: the switch doing nothing, the already-retired
children retired again, the standing orders not paused, the schedules not
paused, the slugs not sorted, the reason losing the parent, the standing
removal's switch doing nothing, another agent's orders removed, missing
orders counted, disabled orders paused again, another agent's orders paused,
the enabled orders counted as paused, the schedule removal's switch doing
nothing, another agent's schedules removed, missing schedules counted,
disabled schedules paused again, another agent's schedules paused, a missing
schedule counted and the enabled schedules counted as paused. 19 mutations,
19 caught.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (268 packages, 129 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md`.

## Not done here

The teardown's **state cleanup** (forgetting the agent's memory, archiving
its skills, pruning the config-center entries and access references, deleting
its workspace) is still native, in `roster_teardown_state.go`, and so is the
impact cascade (`roster_cascade.go`) and the lifecycle helpers
(`roster_lifecycle_helpers.go`). They are the next slices of the same domain.

The cloned-kernel harness of the earlier slices — the pre-slice handler and the
registered operation on two kernels, compared step by step — is not part of
this slice, as in W2.44b through W2.47. The roster's own end-to-end suite is
the parity proof here: it drives the real operation over a real kernel and
reads the journal back.
