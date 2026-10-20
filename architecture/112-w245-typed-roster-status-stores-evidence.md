# W2.45b typed roster status stores

The roster status **collection** now lives entirely in the application layer,
in `kernel/app/roster` beside the status projection it feeds. The control
plane keeps only the host's data access.

W2.45a moved the journal-derived half of the collection; this slice moves the
store-backed half — the wake views, the escalation views, the repair summaries,
the reaper mapping — and the assembly of the whole snapshot.

Three native files are gone:

- `kernel/controlplane/roster_status_wake.go` (107 lines)
- `kernel/controlplane/roster_status_escalation.go` (73 lines)
- `kernel/controlplane/roster_repair_summary.go` (98 lines)

## What moved

`kernel/app/roster/status_stores.go` (329 lines) is new. It owns:

- **the wake views** — the schedules and the standing orders that can wake an
  agent, the nearest scheduled wake and its label, and the event subjects the
  agent listens to;
- **the escalation views** — the board's open help requests, per agent, as open
  and acked counts;
- **the repair summaries** — the doctor's own repair events, the latest repair
  per agent and the in-flight fingerprints;
- **the reaper mapping** — the degraded, misconfigured, routing-pressured,
  forced, forced-failed, forced-exhausted, unstable and dead agents;
- **the assembly** — `CollectStatus`, which fills the snapshot from every
  source.

The host keeps the data access: the control plane binds the journal, the folded
run rows, the schedule store, the standing store, the board reader and the
reaper scan into the application's source set, and the application decides what
each of them MEANS.

## The ports

`StatusSources` grew the store-backed sources:

```go
type StatusSources struct {
    Journal        func(func(*event.Event) error) error
    Runs           func() ([]RunRow, error)
    Schedules      func() []cadence.Entry
    Standing       func() []standing.Order
    Board          func() []board.Message
    Reaper         func(reaperCut int64) runtime.ReaperReport
    RepairCooldown func() time.Duration
}
```

The host's stores stay in the host: the application reads the cadence, the
standing and the board packages only for their row types, and the board's
`OpenHelp` read (with its limit and its error handling) stays in the control
plane — a board that cannot be read is a board with no escalations, not a
failure.

## The repair summaries share the single journal walk

The repair summaries used to be their own journal walk: the native file ran a
second `Range` over the journal, beside the walk that filled the live statuses
and the pressures. Now the doctor's `doctor.auto_repair` events are dispatched
into the repair accumulators inside the **one** walk that W2.45a established.

The semantics are preserved exactly: the latest repair per agent wins by
sequence number, the latest repair per fingerprint wins by sequence number, an
in-flight repair is a fingerprint whose phase is `queued` or
`routing_rollback_queued`, and the next eligible time is the event's timestamp
plus the cooldown. Two journal walks became one, which matters for the same
reason the first eleven mattered: a busy journal was reliably tripping the
control-plane connection's read deadline.

The one behavior that had to be preserved is the empty-roster case: the
repair summaries do not depend on the profiles, so they are collected even when
the roster is empty — the way the native code did, where the repair walk ran
regardless of the profile list.

## The cooldown moved to the application

The repair cooldown's environment reading (`AGEZT_AUTO_REPAIR_COOLDOWN`, the
30-minute default, a malformed or non-positive value falling back to the
default) moved to the application layer as `approster.AutoRepairCooldown`. The
control plane's `agentAutoRepairCooldown` is gone, and the typed
`roster repair status` operation now binds the application's own reader, so the
per-agent repair status command and the roster status row cannot disagree about
when a repair becomes eligible again.

The host may still override the cooldown through the `RepairCooldown` port;
the application falls back to the environment when the port is absent or
returns a non-positive duration.

## The dead code the move left behind

The move left four native type aliases over the app status rows without
readers — `agentRepairRow`, `agentRepairSummary`, `agentEscalationLoad` and
`agentWakeStatus` — and one payload helper (`plInt`) that only the deleted
repair file read. All five are gone. As in W2.45a, the dead-code gate does not
see unused type aliases: `tools/deadcodecheck` reports unreachable functions
only, so staticcheck's U1000 is the gate that catches them.

## The wire shape is unchanged

The status fields, their names, their types and their presence rules are
untouched: the application already owned the projection, and the collection
still fills the same snapshot. The existing end-to-end suite — the wake status,
the live run status, the schedule and standing wake runbooks, the doctor's wake
runbook, the mailbox wake causality, the repair history and the in-flight
count, the dynamic health, the escalations — now exercises the application's
collection and passes unchanged.

## Runnable evidence

Permanent tests cover, at the application layer:

- the wake views: a schedule by its agent field, the legacy `--agent` flag in
  the intent (both the `--agent=slug` and the `--agent slug` form), the nearest
  schedule winning, a disabled schedule that never wakes, a standing order's
  event subjects (deduplicated, blanks and non-event triggers dropped), an
  agent that nothing wakes having no wake status at all, and an empty roster;
- the wake label: the workflow, the system task, the tool, the intent and the
  schedule id;
- the escalation views: a direct message, an acknowledged message, a broadcast
  that reaches everyone but the sender, a message for an unknown agent, and a
  board that cannot be read;
- the reaper mapping: every health family (degraded, misconfigured, routing
  pressure, forced, forced failed, forced exhausted, unstable, dead) with its
  fields, and the cut that reaches the host's scan;
- the repair summaries: the latest repair winning by sequence number, the
  in-flight fingerprints (both queued phases), the next eligible time, the
  host's cooldown port and the environment default (including a malformed and
  a non-positive value);
- the assembly: a snapshot that carries the live status, the repairs, the
  escalations, the wakes and the health maps at once.

At the control plane, the existing roster suite — which publishes the events to
a real kernel's bus and reads the status back over the wire — passes
unchanged, twenty times over.

The mutations fail tests: the nearest schedule becoming the farthest, a
disabled schedule that wakes, the legacy `--agent` flag ignored, the standing
event trigger read as a cron trigger, the agent that nothing wakes kept, the
empty roster returning an empty map, the event subjects not deduplicated, the
broadcast not expanded, the broadcast reaching its own sender, the unknown
recipient counted, the acked message counted as open, the unreadable board
returning escalations, the degraded threshold and window swapped, the forced
generation dropped, the reaper scan never running, the repair latest-wins
inverted, the routing rollback queue not counted as in flight, the next
eligible time ignoring the cooldown, the host's cooldown port ignored, the
default cooldown halved, a non-positive cooldown accepted, the doctor's subject
not matched, the target agent keying the summary and the fingerprint not
tracked. One mutation is a recorded equivalent: splitting the malformed-value
guard into two guards is the same guard.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (268 packages, 129 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md`.

## Not done here

The roster status collection is now wholly in the application layer. What
remains native on this domain is the request handling itself: the list
service binding and the HTTP route that reads the status.

The cloned-kernel harness of the earlier slices — the pre-slice handler and the
registered operation on two kernels, compared step by step — is not part of
this slice, as in W2.44b through W2.45a. The roster's own end-to-end suite is
the parity proof here: it publishes to a real journal and reads the status back
over the wire.
